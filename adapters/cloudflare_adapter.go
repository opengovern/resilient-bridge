// cloudflare_adapter.go
// -----------------------
// This adapter integrates with the Cloudflare API. Cloudflare has both a general API limit and a separate GraphQL limit.
//
// Limits:
// - General API (rest): 1200 requests per 5 minutes per user.
// - GraphQL API: 300 queries per 5 minutes per user.
//
// If a request is GraphQL: It must not exceed either the general limit or the GraphQL-specific limit.
// If a request is non-GraphQL (rest): It must not exceed the general limit only.
//
// We track request timestamps locally to determine if we are rate-limited. Cloudflare does not return explicit rate-limit
// headers for general usage in all cases, so we rely on our internal counters.
//
// Key Points:
// - isGraphQLRequest checks if the endpoint contains "/graphql" to categorize request types.
// - isRateLimited compares the counts of recent requests against known limits.
// - recordRequest stores timestamps of successful requests, and filterTimestamps prunes old timestamps beyond the window.
// - ParseRateLimitInfo returns nil since no specific headers are used.
// - If we hit the limit, ExecuteRequest returns a synthetic 429 before even sending the request.

package adapters

import (
	"bytes"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	resilientbridge "github.com/opengovern/resilient-bridge"
)

// Cloudflare limits (fixed, ignoring user overrides for now):
// General API (REST): 1200 requests/5 min
// GraphQL: 300 queries/5 min
// Window = 300 seconds (5 minutes)
const (
	cloudflareGeneralLimit = 1200
	cloudflareGraphQLLimit = 300
	cloudflareWindowSecs   = 300 // 5 minutes = 300 seconds
)

type CloudflareAdapter struct {
	APIToken string

	mu             sync.Mutex
	generalHistory []int64 // timestamps (in Unix seconds) of all requests
	graphqlHistory []int64 // timestamps (in Unix seconds) of GraphQL requests
}

// NewCloudflareAdapter creates a new instance of CloudflareAdapter with the given API token.
func NewCloudflareAdapter(apiToken string) *CloudflareAdapter {
	return &CloudflareAdapter{
		APIToken: apiToken,
	}
}

// SetRateLimitDefaultsForType currently ignores overrides since Cloudflare rates are fixed.
func (c *CloudflareAdapter) SetRateLimitDefaultsForType(requestType string, maxRequests int, windowSecs int64) {
	// Cloudflare has fixed rules. Ignore overrides for now.
}

// IdentifyRequestType determines if a given request should be categorized as "graphql" or "rest".
// If endpoint contains "/graphql", we consider it a GraphQL request.
func (c *CloudflareAdapter) IdentifyRequestType(req *resilientbridge.NormalizedRequest) string {
	if c.isGraphQLRequest(req) {
		return "graphql"
	}
	return "rest"
}

// ExecuteRequest sends the request to Cloudflare if not rate-limited.
// If rate-limited, returns a synthetic 429 directly, without hitting the API.
func (c *CloudflareAdapter) ExecuteRequest(req *resilientbridge.NormalizedRequest) (*resilientbridge.NormalizedResponse, error) {
	isGraphQL := c.isGraphQLRequest(req)
	if c.isRateLimited(isGraphQL) {
		// Return synthetic 429 if we're locally rate-limited.
		return &resilientbridge.NormalizedResponse{
			StatusCode: 429,
			Headers:    map[string]string{},
			Data:       []byte(`{"error":"Cloudflare rate limit reached"}`),
		}, nil
	}

	client := &http.Client{}
	baseURL := "https://api.cloudflare.com/client/v4"
	fullURL := baseURL + req.Endpoint

	httpReq, err := http.NewRequest(req.Method, fullURL, bytes.NewReader(req.Body))
	if err != nil {
		return nil, err
	}

	// Set headers from NormalizedRequest
	for k, v := range req.Headers {
		httpReq.Header.Set(k, v)
	}
	// If APIToken is provided, use it in Authorization header if not already present.
	if c.APIToken != "" && httpReq.Header.Get("Authorization") == "" {
		httpReq.Header.Set("Authorization", "Bearer "+c.APIToken)
	}
	if httpReq.Header.Get("Content-Type") == "" {
		httpReq.Header.Set("Content-Type", "application/json")
	}

	resp, err := client.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	// Record request timestamp after successful completion
	c.recordRequest(isGraphQL)

	data, _ := io.ReadAll(resp.Body)
	headers := make(map[string]string)
	for k, vals := range resp.Header {
		if len(vals) > 0 {
			headers[strings.ToLower(k)] = vals[0]
		}
	}

	return &resilientbridge.NormalizedResponse{
		StatusCode: resp.StatusCode,
		Headers:    headers,
		Data:       data,
	}, nil
}

// ParseRateLimitInfo returns nil because Cloudflare general APIs do not consistently provide rate-limit headers.
// We rely on internal tracking instead.
func (c *CloudflareAdapter) ParseRateLimitInfo(resp *resilientbridge.NormalizedResponse) (*resilientbridge.NormalizedRateLimitInfo, error) {
	return nil, nil
}

// IsRateLimitError checks if Cloudflare actually returned a 429 status code.
func (c *CloudflareAdapter) IsRateLimitError(resp *resilientbridge.NormalizedResponse) bool {
	return resp.StatusCode == 429
}

// isGraphQLRequest checks if the request endpoint includes "/graphql".
func (c *CloudflareAdapter) isGraphQLRequest(req *resilientbridge.NormalizedRequest) bool {
	return strings.Contains(req.Endpoint, "/graphql")
}

// isRateLimited checks if making another request would exceed local rate limits.
// It ensures that recent requests (within last 5 minutes) do not exceed either general or GraphQL counts.
func (c *CloudflareAdapter) isRateLimited(isGraphQL bool) bool {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now().Unix()
	windowStart := now - cloudflareWindowSecs

	// General limit check
	c.generalHistory = filterTimestamps(c.generalHistory, windowStart)
	if len(c.generalHistory) >= cloudflareGeneralLimit {
		return true
	}

	// GraphQL limit check (only if this request is GraphQL)
	if isGraphQL {
		c.graphqlHistory = filterTimestamps(c.graphqlHistory, windowStart)
		if len(c.graphqlHistory) >= cloudflareGraphQLLimit {
			return true
		}
	}

	return false
}

// recordRequest appends the current timestamp to generalHistory, and graphqlHistory if it's a GraphQL request.
func (c *CloudflareAdapter) recordRequest(isGraphQL bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now().Unix()
	c.generalHistory = append(c.generalHistory, now)
	if isGraphQL {
		c.graphqlHistory = append(c.graphqlHistory, now)
	}
}

// filterTimestamps returns only timestamps within the allowed window.
func filterTimestamps(timestamps []int64, windowStart int64) []int64 {
	var newT []int64
	for _, ts := range timestamps {
		if ts >= windowStart {
			newT = append(newT, ts)
		}
	}
	return newT
}
