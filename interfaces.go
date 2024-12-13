package resilientbridge

// ProviderAdapter defines the interface all adapters must implement.
type ProviderAdapter interface {
	ExecuteRequest(req *NormalizedRequest) (*NormalizedResponse, error)
	ParseRateLimitInfo(resp *NormalizedResponse) (*NormalizedRateLimitInfo, error)
	IsRateLimitError(resp *NormalizedResponse) bool

	// SetRateLimitDefaults sets the max requests and window (in seconds) that this adapter should enforce by default.
	SetRateLimitDefaults(maxRequests int, windowSecs int64)
}
