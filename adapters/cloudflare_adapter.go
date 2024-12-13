package adapters

import (
	"bytes"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/opengovern/resilient-bridge"
)

// Cloudflare limits
// General API: 1200 requests/5 min/user
// GraphQL API: 300 GraphQL queries/5 min/user
//
// For simplicity, we implement two rate limit checks:
// 1. General limit (applies to all requests): 1200 requests per 5 minutes (300 seconds)
// 2. GraphQL limit (applies only to requests hitting GraphQL endpoint): 300 queries per 5 minutes (300 seconds)
//
// If a request is GraphQL: must not exceed either the general limit or the GraphQL limit.
// If a request is non-GraphQL: must not exceed the general limit only.

const (
	cloudflareGeneralLimit = 1200
	cloudflareGraphQLLimit = 300
	cloudflareWindowSecs   = 300 // 5 minutes = 300 seconds
)

type CloudflareAdapter struct {
	APIToken string

	mu             sync.Mutex
	generalHistory []int64 // timestamps of all requests in seconds
	graphqlHistory []int64 // timestamps of graphql requests in seconds
}

func NewCloudflareAdapter(apiToken string) *CloudflareAdapter {
	return &CloudflareAdapter{
		APIToken: apiToken,
	}
}

func (c *CloudflareAdapter) SetRateLimitDefaultsForType(requestType string, maxRequests int, windowSecs int64) {
	// Cloudflare has fixed rules. Ignore overrides for now.
}

func (c *CloudflareAdapter) ExecuteRequest(req *resilientbridge.NormalizedRequest) (*resilientbridge.NormalizedResponse, error) {
	isGraphQL := c.isGraphQLRequest(req)
	if c.isRateLimited(isGraphQL) {
		return &resilientbridge.NormalizedResponse{
			StatusCode: 429,
			Headers:    map[string]string{},
			Data:       []byte(`{"error":"Cloudflare rate limit reached"}`),
		}, nil
	}

	client := &http.Client{}
	baseURL := "https://api.cloudflare.com/client/v4"
	// GraphQL endpoint might differ: "https://api.cloudflare.com/client/v4/graphql"
	// Assume requests define their endpoint fully (if needed).
	fullURL := baseURL + req.Endpoint

	httpReq, err := http.NewRequest(req.Method, fullURL, bytes.NewReader(req.Body))
	if err != nil {
		return nil, err
	}

	for k, v := range req.Headers {
		httpReq.Header.Set(k, v)
	}
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

	// Record successful request
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

func (c *CloudflareAdapter) ParseRateLimitInfo(resp *resilientbridge.NormalizedResponse) (*resilientbridge.NormalizedRateLimitInfo, error) {
	// Cloudflare docs on general rate limiting: no special headers mentioned.
	return nil, nil
}

func (c *CloudflareAdapter) IsRateLimitError(resp *resilientbridge.NormalizedResponse) bool {
	return resp.StatusCode == 429
}

func (c *CloudflareAdapter) isGraphQLRequest(req *resilientbridge.NormalizedRequest) bool {
	// GraphQL endpoint: "/graphql" at Cloudflare typically
	// If the endpoint contains "/graphql" at the end?
	// Cloudflare GraphQL endpoint: "https://api.cloudflare.com/client/v4/graphql"
	return strings.Contains(req.Endpoint, "/graphql")
}

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

	// If GraphQL, also check GraphQL limit
	if isGraphQL {
		c.graphqlHistory = filterTimestamps(c.graphqlHistory, windowStart)
		if len(c.graphqlHistory) >= cloudflareGraphQLLimit {
			return true
		}
	}

	return false
}

func (c *CloudflareAdapter) recordRequest(isGraphQL bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now().Unix()
	c.generalHistory = append(c.generalHistory, now)
	if isGraphQL {
		c.graphqlHistory = append(c.graphqlHistory, now)
	}
}

func filterTimestamps(timestamps []int64, windowStart int64) []int64 {
	var newT []int64
	for _, ts := range timestamps {
		if ts >= windowStart {
			newT = append(newT, ts)
		}
	}
	return newT
}
