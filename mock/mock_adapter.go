package mock

import (
	"unifiedsdk"
)

// MockAdapter simulates provider responses for testing.
type MockAdapter struct {
	RequestsUntilRateLimit int // How many requests succeed before hitting 429
	currentRequestCount    int
	ShouldReturn429Always  bool // If true, always return 429
}

// ExecuteRequest simulates provider responses. The first N requests succeed, subsequent requests return 429.
func (m *MockAdapter) ExecuteRequest(req *unifiedsdk.NormalizedRequest) (*unifiedsdk.NormalizedResponse, error) {
	m.currentRequestCount++

	if m.ShouldReturn429Always || (m.RequestsUntilRateLimit > 0 && m.currentRequestCount > m.RequestsUntilRateLimit) {
		// Simulate rate limit error
		return &unifiedsdk.NormalizedResponse{
			StatusCode: 429,
			Headers:    map[string]string{},
			Data:       []byte(`{"error":"Rate limited"}`),
		}, nil
	}

	// Simulate a success response
	return &unifiedsdk.NormalizedResponse{
		StatusCode: 200,
		Headers:    map[string]string{},
		Data:       []byte(`{"success":true}`),
	}, nil
}

// ParseRateLimitInfo can return a simulated rate limit info if needed.
func (m *MockAdapter) ParseRateLimitInfo(resp *unifiedsdk.NormalizedResponse) (*unifiedsdk.NormalizedRateLimitInfo, error) {
	// You can return a mock rate limit info if needed for testing.
	return nil, nil
}

func (m *MockAdapter) IsRateLimitError(resp *unifiedsdk.NormalizedResponse) bool {
	return resp.StatusCode == 429
}
