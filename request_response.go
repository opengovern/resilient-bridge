package resilientbridge

// NormalizedRequest represents a standardized request structure the SDK uses.
type NormalizedRequest struct {
	Method   string
	Endpoint string
	Headers  map[string]string
	Body     []byte
}

// NormalizedResponse represents a standardized response structure from providers.
type NormalizedResponse struct {
	StatusCode int
	Headers    map[string]string
	Data       []byte
}

// NormalizedRateLimitInfo holds extracted rate limit details.
type NormalizedRateLimitInfo struct {
	MaxRequests       *int
	RemainingRequests *int
	ResetRequestsAt   *int64

	MaxTokens       *int
	RemainingTokens *int
	ResetTokensAt   *int64

	GlobalResetAt *int64
}