package resilientbridge

type NormalizedRequest struct {
	Method   string
	Endpoint string
	Headers  map[string]string
	Body     []byte
}

type NormalizedResponse struct {
	StatusCode int
	Headers    map[string]string
	Data       []byte
}

type NormalizedRateLimitInfo struct {
	MaxRequests       *int
	RemainingRequests *int
	ResetRequestsAt   *int64

	MaxTokens       *int
	RemainingTokens *int
	ResetTokensAt   *int64

	GlobalResetAt *int64
}
