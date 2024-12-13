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

	MaxRequests         int
	WindowSecs          int64
	currentRequestCount int
}

func (m *MockAdapter) SetRateLimitDefaults(maxRequests int, windowSecs int64) {
	if maxRequests == 0 {
		maxRequests = MockDefaultMaxRequests
	}
	if windowSecs == 0 {
		windowSecs = MockDefaultWindowSecs
	}
	m.MaxRequests = maxRequests
	m.WindowSecs = windowSecs
}

func (m *MockAdapter) ExecuteRequest(req *resilientbridge.NormalizedRequest) (*resilientbridge.NormalizedResponse, error) {
	// If always returning 429, or if request count exceeds a certain threshold:
	m.currentRequestCount++

	if m.ShouldReturn429Always || (m.RequestsUntilRateLimit > 0 && m.currentRequestCount > m.RequestsUntilRateLimit) {
		return &resilientbridge.NormalizedResponse{
			StatusCode: 429,
			Headers:    map[string]string{},
			Data:       []byte(`{"error":"Rate limited"}`),
		}, nil
	}

	return &resilientbridge.NormalizedResponse{
		StatusCode: 200,
		Headers:    map[string]string{},
		Data:       []byte(`{"success":true}`),
	}, nil
}

func (m *MockAdapter) ParseRateLimitInfo(resp *resilientbridge.NormalizedResponse) (*resilientbridge.NormalizedRateLimitInfo, error) {
	// For the mock, just return a static info or something simple
	remaining := m.MaxRequests - m.currentRequestCount
	if remaining < 0 {
		remaining = 0
	}
	var resetAt *int64
	if remaining == 0 {
		future := (time.Now().Unix() + m.WindowSecs) * 1000
		resetAt = &future
	}
	info := &resilientbridge.NormalizedRateLimitInfo{
		MaxRequests:       intPtr(m.MaxRequests),
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
