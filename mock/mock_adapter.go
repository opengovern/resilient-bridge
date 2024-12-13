// Package mock provides a mock adapter that simulates a provider's behavior.
// It can be used to test the SDK's rate limiting, retry logic, and error handling.
// This adapter is highly configurable and allows developers to simulate multiple scenarios:
// - Always returning a rate limit error (429).
// - Returning successful responses until a certain threshold, then returning rate limit errors.
// - Distinguishing between REST and GraphQL request limits.
// - Simulating random delays or transient errors.
//
// By adjusting the fields below, you can create a variety of test conditions.
package mock

import (
	"errors"
	"math/rand"
	"strings"
	"time"

	resilientbridge "github.com/opengovern/resilient-bridge"
)

const (
	MockDefaultMaxRequests = 100
	MockDefaultWindowSecs  = 60
)

// MockAdapter simulates a provider adapter for testing the SDK.
// It allows you to configure how many requests succeed before hitting a rate limit,
// whether to always return 429, and supports separate rate limits for REST and GraphQL calls.
type MockAdapter struct {
	// RequestsUntilRateLimit determines after how many requests the adapter starts returning 429.
	// Example: If set to 10, the 11th request will receive a 429.
	// Set to 0 to never limit until ShouldReturn429Always is true.
	RequestsUntilRateLimit int

	// ShouldReturn429Always forces every request to return a 429 rate limit error.
	// Useful to test retry logic immediately.
	ShouldReturn429Always bool

	// RandomDelayEnabled introduces a random sleep before responding, simulating network latency.
	// The delay is between 0 and 500ms.
	RandomDelayEnabled bool

	// RandomErrorChance introduces a small probability of returning a network error (non-HTTP).
	// For example: if set to 0.1, there's a 10% chance per request to return an error without HTTP response.
	RandomErrorChance float64

	// Maximum and window for REST requests
	MaxRequestsRest         int
	WindowSecsRest          int64
	currentRequestCountRest int

	// Maximum and window for GraphQL requests
	MaxRequestsGraphQL         int
	WindowSecsGraphQL          int64
	currentRequestCountGraphQL int
}

// SetRateLimitDefaultsForType configures default rate limits for a given request type ("rest" or "graphql").
// If maxRequests or windowSecs is zero, it uses the default constants.
func (m *MockAdapter) SetRateLimitDefaultsForType(requestType string, maxRequests int, windowSecs int64) {
	if maxRequests == 0 {
		maxRequests = MockDefaultMaxRequests
	}
	if windowSecs == 0 {
		windowSecs = MockDefaultWindowSecs
	}

	if requestType == "rest" {
		m.MaxRequestsRest = maxRequests
		m.WindowSecsRest = windowSecs
	} else if requestType == "graphql" {
		m.MaxRequestsGraphQL = maxRequests
		m.WindowSecsGraphQL = windowSecs
	}
}

// IdentifyRequestType categorizes the request.
// For simplicity, if Endpoint == "/graphql", treat it as GraphQL; else treat as REST.
func (m *MockAdapter) IdentifyRequestType(req *resilientbridge.NormalizedRequest) string {
	if req.Endpoint == "/graphql" {
		return "graphql"
	}
	return "rest"
}

// ExecuteRequest simulates sending a request to a provider.
// - May return a random error if RandomErrorChance is triggered.
// - May apply random delay if RandomDelayEnabled is true.
// - Checks if rate limit should be enforced.
// - Otherwise returns a 200 success response.
func (m *MockAdapter) ExecuteRequest(req *resilientbridge.NormalizedRequest) (*resilientbridge.NormalizedResponse, error) {
	// Simulate network randomness
	m.maybeDelay()
	if m.maybeRandomError() {
		return nil, errors.New("mock: simulated network error")
	}

	isGraphQL := (req.Endpoint == "/graphql")

	// Determine if we should return 429:
	if m.ShouldReturn429Always || m.rateLimitReached(isGraphQL) {
		return &resilientbridge.NormalizedResponse{
			StatusCode: 429,
			Headers:    map[string]string{},
			Data:       []byte(`{"error":"Rate limited"}`),
		}, nil
	}

	// Increment counters to simulate usage
	if isGraphQL {
		m.currentRequestCountGraphQL++
	} else {
		m.currentRequestCountRest++
	}

	// Simulate a normal successful response
	responseData := `{"success":true}`
	if strings.Contains(req.Endpoint, "special") {
		responseData = `{"message":"special endpoint success"}`
	}

	return &resilientbridge.NormalizedResponse{
		StatusCode: 200,
		Headers: map[string]string{
			// We can include arbitrary headers here if needed
			"content-type": "application/json",
		},
		Data: []byte(responseData),
	}, nil
}

// ParseRateLimitInfo simulates returning rate limit information.
// This is a mock implementation: it returns data based on how many REST requests have been made.
// In a real adapter, you'd parse headers like X-RateLimit-Limit, X-RateLimit-Remaining, etc.
func (m *MockAdapter) ParseRateLimitInfo(resp *resilientbridge.NormalizedResponse) (*resilientbridge.NormalizedRateLimitInfo, error) {
	remaining := m.MaxRequestsRest - m.currentRequestCountRest
	if remaining < 0 {
		remaining = 0
	}
	var resetAt *int64
	if remaining == 0 {
		// If no requests remain, pretend reset is after the window
		future := (time.Now().Unix() + m.WindowSecsRest) * 1000
		resetAt = &future
	}
	info := &resilientbridge.NormalizedRateLimitInfo{
		MaxRequests:       intPtr(m.MaxRequestsRest),
		RemainingRequests: intPtr(remaining),
		ResetRequestsAt:   resetAt,
	}
	return info, nil
}

// IsRateLimitError checks if the response is a 429 rate limit error.
// This is straightforward in a mock adapter.
func (m *MockAdapter) IsRateLimitError(resp *resilientbridge.NormalizedResponse) bool {
	return resp.StatusCode == 429
}

// rateLimitReached checks if we should return 429 based on RequestsUntilRateLimit.
// If RequestsUntilRateLimit is > 0, once we exceed that threshold for the given request type, we rate limit.
func (m *MockAdapter) rateLimitReached(isGraphQL bool) bool {
	if m.RequestsUntilRateLimit <= 0 {
		// If no limit threshold is set, no forced 429 (unless ShouldReturn429Always is true)
		return false
	}

	if isGraphQL && m.currentRequestCountGraphQL >= m.RequestsUntilRateLimit {
		return true
	}
	if !isGraphQL && m.currentRequestCountRest >= m.RequestsUntilRateLimit {
		return true
	}
	return false
}

// maybeDelay introduces a random delay of up to 500ms if RandomDelayEnabled is set.
// This simulates network latency or slow servers.
func (m *MockAdapter) maybeDelay() {
	if m.RandomDelayEnabled {
		delay := time.Duration(rand.Intn(500)) * time.Millisecond
		time.Sleep(delay)
	}
}

// maybeRandomError returns true if we should simulate a random network error.
// The chance is defined by RandomErrorChance (0.0 = no chance, 1.0 = always error).
func (m *MockAdapter) maybeRandomError() bool {
	if m.RandomErrorChance > 0 {
		if rand.Float64() < m.RandomErrorChance {
			return true
		}
	}
	return false
}

// intPtr is a helper to convert int to *int.
func intPtr(i int) *int {
	return &i
}
