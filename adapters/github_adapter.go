package adapters

import (
	"bytes"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	resilientbridge "github.com/opengovern/resilient-bridge"
)

// Default GitHub limits for authenticated PAT:
// REST: 5000 requests/hour (5000 per 3600 seconds)
// GraphQL: Also 5000/hour by default.
const (
	GitHubDefaultRestMaxRequests    = 5000
	GitHubDefaultRestWindowSecs     = 3600
	GitHubDefaultGraphQLMaxRequests = 5000
	GitHubDefaultGraphQLWindowSecs  = 3600
)

type GitHubAdapter struct {
	APIToken string

	mu sync.Mutex

	restMaxRequests  int
	restWindowSecs   int64
	restRequestTimes []int64

	graphqlMaxRequests  int
	graphqlWindowSecs   int64
	graphqlRequestTimes []int64
}

func NewGitHubAdapter(apiToken string) *GitHubAdapter {
	return &GitHubAdapter{
		APIToken:           apiToken,
		restMaxRequests:    GitHubDefaultRestMaxRequests,
		restWindowSecs:     GitHubDefaultRestWindowSecs,
		graphqlMaxRequests: GitHubDefaultGraphQLMaxRequests,
		graphqlWindowSecs:  GitHubDefaultGraphQLWindowSecs,
	}
}

func (g *GitHubAdapter) SetRateLimitDefaultsForType(requestType string, maxRequests int, windowSecs int64) {
	g.mu.Lock()
	defer g.mu.Unlock()

	if requestType == "rest" {
		if maxRequests == 0 {
			maxRequests = GitHubDefaultRestMaxRequests
		}
		if windowSecs == 0 {
			windowSecs = GitHubDefaultRestWindowSecs
		}
		g.restMaxRequests = maxRequests
		g.restWindowSecs = windowSecs
	} else if requestType == "graphql" {
		if maxRequests == 0 {
			maxRequests = GitHubDefaultGraphQLMaxRequests
		}
		if windowSecs == 0 {
			windowSecs = GitHubDefaultGraphQLWindowSecs
		}
		g.graphqlMaxRequests = maxRequests
		g.graphqlWindowSecs = windowSecs
	}
}

func (g *GitHubAdapter) ExecuteRequest(req *resilientbridge.NormalizedRequest) (*resilientbridge.NormalizedResponse, error) {
	isGraphQL := g.isGraphQLRequest(req)
	if g.isRateLimited(isGraphQL) {
		return &resilientbridge.NormalizedResponse{
			StatusCode: 429,
			Headers:    map[string]string{},
			Data:       []byte(`{"error":"GitHub rate limit reached"}`),
		}, nil
	}

	client := &http.Client{}
	baseURL := "https://api.github.com"
	fullURL := baseURL + req.Endpoint

	httpReq, err := http.NewRequest(req.Method, fullURL, bytes.NewReader(req.Body))
	if err != nil {
		return nil, err
	}

	for k, v := range req.Headers {
		httpReq.Header.Set(k, v)
	}
	if httpReq.Header.Get("Authorization") == "" && g.APIToken != "" {
		httpReq.Header.Set("Authorization", "Bearer "+g.APIToken)
	}
	if httpReq.Header.Get("Content-Type") == "" {
		httpReq.Header.Set("Content-Type", "application/json")
	}

	resp, err := client.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	g.recordRequest(isGraphQL)

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

func (g *GitHubAdapter) ParseRateLimitInfo(resp *resilientbridge.NormalizedResponse) (*resilientbridge.NormalizedRateLimitInfo, error) {
	// GitHub provides x-ratelimit-limit, x-ratelimit-remaining, x-ratelimit-reset headers
	// We'll trust these headers directly. If user wants overrides, they must match these known limits.

	h := resp.Headers
	parseInt := func(key string) *int {
		if val, ok := h[key]; ok {
			if i, err := strconv.Atoi(val); err == nil {
				return &i
			}
		}
		return nil
	}

	parseReset := func(key string) *int64 {
		if val, ok := h[key]; ok {
			if ts, err := strconv.ParseInt(val, 10, 64); err == nil {
				// Convert seconds to ms
				ms := ts * 1000
				return &ms
			}
		}
		return nil
	}

	info := &resilientbridge.NormalizedRateLimitInfo{
		MaxRequests:       parseInt("x-ratelimit-limit"),
		RemainingRequests: parseInt("x-ratelimit-remaining"),
		ResetRequestsAt:   parseReset("x-ratelimit-reset"),
	}

	return info, nil
}

func (g *GitHubAdapter) IsRateLimitError(resp *resilientbridge.NormalizedResponse) bool {
	return resp.StatusCode == 429 || resp.StatusCode == 403
}

// Additional helper methods

func (g *GitHubAdapter) isGraphQLRequest(req *resilientbridge.NormalizedRequest) bool {
	// GitHub GraphQL endpoint is typically POST /graphql
	// If Endpoint == "/graphql", consider it GraphQL
	return req.Endpoint == "/graphql"
}

func (g *GitHubAdapter) isRateLimited(isGraphQL bool) bool {
	g.mu.Lock()
	defer g.mu.Unlock()

	var maxReq int
	var windowSecs int64
	var timestamps []int64

	if isGraphQL {
		maxReq = g.graphqlMaxRequests
		windowSecs = g.graphqlWindowSecs
		timestamps = g.graphqlRequestTimes
	} else {
		maxReq = g.restMaxRequests
		windowSecs = g.restWindowSecs
		timestamps = g.restRequestTimes
	}

	now := time.Now().Unix()
	windowStart := now - windowSecs
	var newTimestamps []int64
	for _, ts := range timestamps {
		if ts >= windowStart {
			newTimestamps = append(newTimestamps, ts)
		}
	}

	if isGraphQL {
		g.graphqlRequestTimes = newTimestamps
	} else {
		g.restRequestTimes = newTimestamps
	}

	return len(newTimestamps) >= maxReq
}

func (g *GitHubAdapter) recordRequest(isGraphQL bool) {
	g.mu.Lock()
	defer g.mu.Unlock()
	now := time.Now().Unix()
	if isGraphQL {
		g.graphqlRequestTimes = append(g.graphqlRequestTimes, now)
	} else {
		g.restRequestTimes = append(g.restRequestTimes, now)
	}
}
