// openai_adapter.go
// -----------------
// This adapter integrates with the OpenAI API. We rely on the rate limit headers returned by the API:
//
// Headers:
// - x-ratelimit-limit-requests: Maximum requests allowed before rate limit is hit.
// - x-ratelimit-remaining-requests: Requests remaining in the current window.
// - x-ratelimit-reset-requests: The time until the rate limit resets to its initial state, in a string like "1s".
//   We must parse this duration and convert it to a future timestamp.
//
// Tokens are not applicable, so we ignore any token-related headers.
//
// We do not preemptively block requests before hitting the limit. Instead, if we receive a 429 response, we return
// an error so the SDK can handle retries. ParseRateLimitInfo returns the rate limit info derived from the headers.

package adapters

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	resilientbridge "github.com/opengovern/resilient-bridge"
)

const (
	OpenAIDefaultMaxRequests = 60
	OpenAIDefaultWindowSecs  = 60
)

type OpenAIAdapter struct {
	APIKey string

	mu sync.Mutex

	restMaxRequests int
	restWindowSecs  int64
}

// NewOpenAIAdapter creates a new adapter with default limits.
func NewOpenAIAdapter(apiKey string) *OpenAIAdapter {
	return &OpenAIAdapter{
		APIKey:          apiKey,
		restMaxRequests: OpenAIDefaultMaxRequests,
		restWindowSecs:  OpenAIDefaultWindowSecs,
	}
}

// SetRateLimitDefaultsForType sets defaults for "rest" requests. OpenAI doesn't use GraphQL.
func (o *OpenAIAdapter) SetRateLimitDefaultsForType(requestType string, maxRequests int, windowSecs int64) {
	o.mu.Lock()
	defer o.mu.Unlock()

	if requestType == "rest" {
		if maxRequests == 0 {
			maxRequests = OpenAIDefaultMaxRequests
		}
		if windowSecs == 0 {
			windowSecs = OpenAIDefaultWindowSecs
		}
		o.restMaxRequests = maxRequests
		o.restWindowSecs = windowSecs
	}
}

// IdentifyRequestType returns "rest" as OpenAI only supports REST endpoints.
func (o *OpenAIAdapter) IdentifyRequestType(req *resilientbridge.NormalizedRequest) string {
	return "rest"
}

// ExecuteRequest sends the request to OpenAI. If OpenAI returns 429, we return an error
// so that the SDK can handle retries. We do not do synthetic 429 before sending.
func (o *OpenAIAdapter) ExecuteRequest(req *resilientbridge.NormalizedRequest) (*resilientbridge.NormalizedResponse, error) {
	client := &http.Client{}
	fullURL := "https://api.openai.com" + req.Endpoint

	httpReq, err := http.NewRequest(req.Method, fullURL, bytes.NewReader(req.Body))
	if err != nil {
		return nil, err
	}

	// Set headers
	for k, v := range req.Headers {
		httpReq.Header.Set(k, v)
	}
	if httpReq.Header.Get("Authorization") == "" && o.APIKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+o.APIKey)
	}
	if httpReq.Header.Get("Content-Type") == "" {
		httpReq.Header.Set("Content-Type", "application/json")
	}

	resp, err := client.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	data, _ := io.ReadAll(resp.Body)
	headers := make(map[string]string)
	for k, vals := range resp.Header {
		if len(vals) > 0 {
			headers[strings.ToLower(k)] = vals[0]
		}
	}

	response := &resilientbridge.NormalizedResponse{
		StatusCode: resp.StatusCode,
		Headers:    headers,
		Data:       data,
	}

	// If actual 429 from OpenAI, return error.
	if resp.StatusCode == 429 {
		return response, errors.New("openai: rate limit exceeded (429)")
	}

	return response, nil
}

// ParseRateLimitInfo uses the x-ratelimit-* headers to determine the current rate limit status.
// We rely solely on these headers instead of internal timestamps.
//
// Example headers:
// x-ratelimit-limit-requests: "60"
// x-ratelimit-remaining-requests: "59"
// x-ratelimit-reset-requests: "1s"
//
// We'll parse the integer values and the duration.
// If reset is "1s", we'll convert it into a future Unix timestamp in milliseconds.
func (o *OpenAIAdapter) ParseRateLimitInfo(resp *resilientbridge.NormalizedResponse) (*resilientbridge.NormalizedRateLimitInfo, error) {
	h := resp.Headers
	limitStr, limitOK := h["x-ratelimit-limit-requests"]
	remainingStr, remainingOK := h["x-ratelimit-remaining-requests"]
	resetStr, resetOK := h["x-ratelimit-reset-requests"]

	if !limitOK || !remainingOK || !resetOK {
		// If headers are not present, return nil. We can't derive info without them.
		return nil, nil
	}

	limit, err := strconv.Atoi(limitStr)
	if err != nil {
		return nil, nil
	}
	remaining, err := strconv.Atoi(remainingStr)
	if err != nil {
		return nil, nil
	}

	// Parse the reset duration string, e.g. "1s"
	dur, err := time.ParseDuration(resetStr)
	if err != nil {
		// If we can't parse, just return nil. Better to have no info than wrong info.
		return nil, nil
	}
	// Convert to a future reset timestamp in ms
	resetMs := time.Now().Add(dur).UnixMilli()

	info := &resilientbridge.NormalizedRateLimitInfo{
		MaxRequests:       resilientbridge.IntPtr(limit),
		RemainingRequests: resilientbridge.IntPtr(remaining),
		ResetRequestsAt:   &resetMs,
	}

	return info, nil
}

// IsRateLimitError returns true if the response status code is 429.
func (o *OpenAIAdapter) IsRateLimitError(resp *resilientbridge.NormalizedResponse) bool {
	return resp.StatusCode == 429
}
