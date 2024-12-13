// doppler_adapter.go
// -------------------
// This file implements a ProviderAdapter for the Doppler API. It uses REST-based rate limiting.
// The DopplerAdapter:
// - Tracks the number of requests made in a given time window to determine remaining capacity.
// - Integrates with the Resilient-Bridge SDK to handle retries, rate limit parsing, and request execution.
//
// Key Points:
// - Doppler uses a REST interface only, so IdentifyRequestType always returns "rest".
// - The adapter sets default rate limits (480 requests per 60 seconds by default) unless overridden.
// - After each request, it updates an internal list of timestamps to determine remaining requests.
// - ParseRateLimitInfo uses this timestamp list to simulate known rate limit state without explicit headers from Doppler.
// - IsRateLimitError checks the HTTP status code for 429 responses from the Doppler API itself.
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
	DopplerDefaultMaxRequests = 480
	DopplerDefaultWindowSecs  = 60
)

type DopplerAdapter struct {
	APIToken string

	mu                sync.Mutex
	requestTimestamps []int64

	restMaxRequests int
	restWindowSecs  int64
}

// SetRateLimitDefaultsForType sets default rate limit values for Doppler requests.
// Since Doppler does not have GraphQL, we only adjust "rest" request types.
func (d *DopplerAdapter) SetRateLimitDefaultsForType(requestType string, maxRequests int, windowSecs int64) {
	d.mu.Lock()
	defer d.mu.Unlock()

	// Doppler only supports REST calls, no separate logic for GraphQL needed.
	if requestType == "rest" {
		if maxRequests == 0 {
			maxRequests = DopplerDefaultMaxRequests
		}
		if windowSecs == 0 {
			windowSecs = DopplerDefaultWindowSecs
		}
		d.restMaxRequests = maxRequests
		d.restWindowSecs = windowSecs
	}
}

// IdentifyRequestType returns the type of request.
// For Doppler, all requests are considered "rest" since there's no GraphQL endpoint.
func (d *DopplerAdapter) IdentifyRequestType(req *resilientbridge.NormalizedRequest) string {
	return "rest"
}

// ExecuteRequest sends the HTTP request to the Doppler API.
// It sets the Authorization header with the Doppler API token and content type if not specified.
// After the response is received, it records the request timestamp for rate limiting calculations.
func (d *DopplerAdapter) ExecuteRequest(req *resilientbridge.NormalizedRequest) (*resilientbridge.NormalizedResponse, error) {
	client := &http.Client{}
	fullURL := "https://api.doppler.com" + req.Endpoint

	httpReq, err := http.NewRequest(req.Method, fullURL, bytes.NewReader(req.Body))
	if err != nil {
		return nil, err
	}
	for k, v := range req.Headers {
		httpReq.Header.Set(k, v)
	}
	httpReq.Header.Set("Authorization", "Bearer "+d.APIToken)
	if httpReq.Header.Get("Content-Type") == "" {
		httpReq.Header.Set("Content-Type", "application/json")
	}

	resp, err := client.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	// Record the request after it completes to update internal rate limit tracking.
	d.recordRequest()

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
// Doppler doesn't provide explicit rate limit headers, so we rely on local request timestamps.
// This simulates a sliding window: removing old requests outside the current window and determining remaining capacity.
func (d *DopplerAdapter) ParseRateLimitInfo(resp *resilientbridge.NormalizedResponse) (*resilientbridge.NormalizedRateLimitInfo, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	now := time.Now().Unix()
	windowStart := now - d.restWindowSecs

	var newTimestamps []int64
	for _, ts := range d.requestTimestamps {
		if ts >= windowStart {
			newTimestamps = append(newTimestamps, ts)
		}
	}
	d.requestTimestamps = newTimestamps

	remaining := d.restMaxRequests - len(d.requestTimestamps)
	var resetAt *int64
	if remaining <= 0 {
		// If we've used up all requests, set a reset time in the future.
		resetTime := (windowStart + d.restWindowSecs) * 1000
		resetAt = &resetTime
	}

	info := &resilientbridge.NormalizedRateLimitInfo{
		MaxRequests:       resilientbridge.IntPtr(d.restMaxRequests),
		RemainingRequests: resilientbridge.IntPtr(remaining),
		ResetRequestsAt:   resetAt,
	}

	return info, nil
}

// IsRateLimitError checks if the HTTP response indicates a rate limit error (status code 429).
// If Doppler returns 429, we know we've hit the rate limit server-side.
func (d *DopplerAdapter) IsRateLimitError(resp *resilientbridge.NormalizedResponse) bool {
	return resp.StatusCode == 429
}

// recordRequest adds the current timestamp to the requestTimestamps slice.
// This helps track how many requests have been made in the current window.
func (d *DopplerAdapter) recordRequest() {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.requestTimestamps = append(d.requestTimestamps, time.Now().Unix())
}
