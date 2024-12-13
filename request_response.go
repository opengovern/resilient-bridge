// request_response.go
// --------------------
// This file defines the NormalizedRequest and NormalizedResponse types, which represent
// the SDK's provider-agnostic request and response structures. Providers convert their
// native request/response formats into these normalized forms.
//
// NormalizedRateLimitInfo holds parsed rate limit details (like max requests, remaining,
// and reset time) that adapters can extract from response headers.
//
// Together, these types ensure a consistent interface for requests and responses across
// all providers.
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

// IntPtr is a helper to quickly create an *int from an int.
func IntPtr(i int) *int {
	return &i
}
