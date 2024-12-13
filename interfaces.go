package resilientbridge

// ProviderAdapter defines the interface all adapters must implement.
type ProviderAdapter interface {
	ExecuteRequest(req *NormalizedRequest) (*NormalizedResponse, error)
	ParseRateLimitInfo(resp *NormalizedResponse) (*NormalizedRateLimitInfo, error)
	IsRateLimitError(resp *NormalizedResponse) bool

	// SetRateLimitDefaultsForType allows setting default rate limits for a specific call type.
	SetRateLimitDefaultsForType(requestType string, maxRequests int, windowSecs int64)

	// IdentifyRequestType inspects the request and returns a call type string
	// This could be "rest", "graphql", or any other type the adapter supports.
	IdentifyRequestType(req *NormalizedRequest) string
}
