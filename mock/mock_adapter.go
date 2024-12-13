package mock

import (
	"time"

	resilientbridge "github.com/opengovern/resilient-bridge"
)

const (
	MockDefaultMaxRequests = 100
	MockDefaultWindowSecs  = 60
)

type MockAdapter struct {
	RequestsUntilRateLimit int  // How many requests until we hit a limit
	ShouldReturn429Always  bool // If true, always return 429

	MaxRequestsRest         int
	WindowSecsRest          int64
	currentRequestCountRest int

	MaxRequestsGraphQL         int
	WindowSecsGraphQL          int64
	currentRequestCountGraphQL int
}

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

func (m *MockAdapter) ExecuteRequest(req *resilientbridge.NormalizedRequest) (*resilientbridge.NormalizedResponse, error) {
	isGraphQL := req.Endpoint == "/graphql"
	if m.ShouldReturn429Always || (m.RequestsUntilRateLimit > 0 && ((isGraphQL && m.currentRequestCountGraphQL > m.RequestsUntilRateLimit) || (!isGraphQL && m.currentRequestCountRest > m.RequestsUntilRateLimit))) {
		return &resilientbridge.NormalizedResponse{
			StatusCode: 429,
			Headers:    map[string]string{},
			Data:       []byte(`{"error":"Rate limited"}`),
		}, nil
	}

	// Simulate a success response
	if isGraphQL {
		m.currentRequestCountGraphQL++
	} else {
		m.currentRequestCountRest++
	}

	return &resilientbridge.NormalizedResponse{
		StatusCode: 200,
		Headers:    map[string]string{},
		Data:       []byte(`{"success":true}`),
	}, nil
}

func (m *MockAdapter) ParseRateLimitInfo(resp *resilientbridge.NormalizedResponse) (*resilientbridge.NormalizedRateLimitInfo, error) {
	// Just return some info based on currentRequestCountRest as if it's REST calls
	// This mock is simplistic.
	remaining := m.MaxRequestsRest - m.currentRequestCountRest
	if remaining < 0 {
		remaining = 0
	}
	var resetAt *int64
	if remaining == 0 {
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

func (m *MockAdapter) IsRateLimitError(resp *resilientbridge.NormalizedResponse) bool {
	return resp.StatusCode == 429
}

func intPtr(i int) *int {
	return &i
}
