package resilientbridge

// ProviderAdapter defines the interface all adapters must implement.
type ProviderAdapter interface {
	ExecuteRequest(req *NormalizedRequest) (*NormalizedResponse, error)
	ParseRateLimitInfo(resp *NormalizedResponse) (*NormalizedRateLimitInfo, error)
	IsRateLimitError(resp *NormalizedResponse) bool

	// SetRateLimitDefaultsForType sets the max requests and window for a given request type (e.g., "rest", "graphql").
	// If maxRequests or windowSecs are zero, the adapter uses its internal defaults.
	SetRateLimitDefaultsForType(requestType string, maxRequests int, windowSecs int64)
}
