// interfaces.go
// --------------
// This file defines the ProviderAdapter interface. Every adapter must implement these methods
// to integrate with the SDK.
//
// Methods:
// - ExecuteRequest: How to actually send the request to the provider.
// - ParseRateLimitInfo: How to extract rate limit details from the response.
// - IsRateLimitError: How to identify a rate limit error (e.g., HTTP 429).
// - SetRateLimitDefaultsForType: Initialize default rate limits for different request types (rest, graphql, etc.).
// - IdentifyRequestType: Determine the type of request (rest, graphql, read, write, etc.) based on the request.
package resilientbridge

// ProviderAdapter defines the interface all adapters must implement.
type ProviderAdapter interface {
	ExecuteRequest(req *NormalizedRequest) (*NormalizedResponse, error)
	ParseRateLimitInfo(resp *NormalizedResponse) (*NormalizedRateLimitInfo, error)
	IsRateLimitError(resp *NormalizedResponse) bool

	SetRateLimitDefaultsForType(requestType string, maxRequests int, windowSecs int64)
	IdentifyRequestType(req *NormalizedRequest) string
}
