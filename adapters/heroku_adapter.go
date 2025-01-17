// heroku_adapter.go
// -------------------
// This file implements a ProviderAdapter for the Heroku API. It uses REST-based rate limiting.
// The HerokuAdapter:
// - Tracks the number of requests made in a given time window to determine remaining capacity.
// - Integrates with the Resilient-Bridge SDK to handle retries, rate limit parsing, and request execution.
//
// Key Points:
// - Heroku uses a REST interface only, so IdentifyRequestType always returns "rest".
// - The adapter sets default rate limits (480 requests per 60 seconds by default) unless overridden.
// - After each request, it updates an internal list of timestamps to determine remaining requests.
// - ParseRateLimitInfo uses this timestamp list to simulate known rate limit state without explicit headers from Heroku.
// - IsRateLimitError checks the HTTP status code for 429 responses from the Heroku API itself.
// - ExecuteRequest handles constructing and sending the HTTP request, including setting the Authorization header.

package adapters

import (
	"bytes"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	resilientbridge "github.com/opengovern/resilient-bridge"
)

const (
	HerokuDefaultMaxRequests = 480
	HerokuDefaultWindowSecs  = 60
)

type HerokuAdapter struct {
	APIToken string

	mu                sync.Mutex
	requestTimestamps []int64

	restMaxRequests int
	restWindowSecs  int64
}

// SetRateLimitDefaultsForType sets default rate limit values for Heroku requests.
// Since Heroku does not have GraphQL, we only adjust "rest" request types.
func (h *HerokuAdapter) SetRateLimitDefaultsForType(requestType string, maxRequests int, windowSecs int64) {
	h.mu.Lock()
	defer h.mu.Unlock()

	// Heroku only supports REST calls, no separate logic for GraphQL needed.
	if requestType == "rest" {
		if maxRequests == 0 {
			maxRequests = HerokuDefaultMaxRequests
		}
		if windowSecs == 0 {
			windowSecs = HerokuDefaultWindowSecs
		}
		h.restMaxRequests = maxRequests
		h.restWindowSecs = windowSecs
	}
}

// IdentifyRequestType returns the type of request.
// For Heroku, all requests are considered "rest" since there's no GraphQL endpoint.
func (h *HerokuAdapter) IdentifyRequestType(req *resilientbridge.NormalizedRequest) string {
	return "rest"
}

// ExecuteRequest sends the HTTP request to the Heroku API.
// It sets the Authorization header with the Heroku API token and content type if not specified.
// After the response is received, it records the request timestamp for rate limiting calculations.
func (h *HerokuAdapter) ExecuteRequest(req *resilientbridge.NormalizedRequest) (*resilientbridge.NormalizedResponse, error) {
	client := &http.Client{}
	fullURL := "https://api.heroku.com" + req.Endpoint

	httpReq, err := http.NewRequest(req.Method, fullURL, bytes.NewReader(req.Body))
	if err != nil {
		return nil, err
	}
	for k, v := range req.Headers {
		httpReq.Header.Set(k, v)
	}
	httpReq.Header.Set("Authorization", "Bearer "+h.APIToken)
	if httpReq.Header.Get("Content-Type") == "" {
		httpReq.Header.Set("Content-Type", "application/json")
	}

	resp, err := client.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	// Record the request after it completes to update internal rate limit tracking.
	h.recordRequest()

	data, _ := io.ReadAll(resp.Body)
	headers := make(map[string]string)
	for k, vals := range resp.Header {
		if len(vals) > 0 {
			headers[strings.ToLower(k)] = vals[0]
		}
	}

	return &resilientbridge.NormalizedResponse{
		StatusCode: resp.StatusCode,
		Headers:    headers,
		Data:       data,
	}, nil
}

// ParseRateLimitInfo calculates the current rate limit state based on the timestamps of recent requests.
// Heroku doesn't provide explicit rate limit headers, so we rely on local request timestamps.
// This simulates a sliding window: removing old requests outside the current window and determining remaining capacity.
func (h *HerokuAdapter) ParseRateLimitInfo(resp *resilientbridge.NormalizedResponse) (*resilientbridge.NormalizedRateLimitInfo, error) {
	h.mu.Lock()
	defer h.mu.Unlock()

	now := time.Now().Unix()
	windowStart := now - h.restWindowSecs

	var newTimestamps []int64
	for _, ts := range h.requestTimestamps {
		if ts >= windowStart {
			newTimestamps = append(newTimestamps, ts)
		}
	}
	h.requestTimestamps = newTimestamps

	remaining := h.restMaxRequests - len(h.requestTimestamps)
	var resetAt *int64
	if remaining <= 0 {
		// If we've used up all requests, set a reset time in the future.
		resetTime := (windowStart + h.restWindowSecs) * 1000
		resetAt = &resetTime
	}

	info := &resilientbridge.NormalizedRateLimitInfo{
		MaxRequests:       resilientbridge.IntPtr(h.restMaxRequests),
		RemainingRequests: resilientbridge.IntPtr(remaining),
		ResetRequestsAt:   resetAt,
	}

	return info, nil
}

// IsRateLimitError checks if the HTTP response indicates a rate limit error (status code 429).
// If Heroku returns 429, we know we've hit the rate limit server-side.
func (h *HerokuAdapter) IsRateLimitError(resp *resilientbridge.NormalizedResponse) bool {
	return resp.StatusCode == 429
}

// recordRequest adds the current timestamp to the requestTimestamps slice.
// This helps track how many requests have been made in the current window.
func (h *HerokuAdapter) recordRequest() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.requestTimestamps = append(h.requestTimestamps, time.Now().Unix())
}
