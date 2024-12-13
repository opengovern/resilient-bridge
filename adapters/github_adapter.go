// github_adapter.go
// -----------------
// This adapter integrates with the GitHub API, handling both REST and GraphQL requests.
// Besides the primary rate limit (e.g., 5000 requests/hour), GitHub uses a secondary rate limit
// that counts "points" rather than raw requests. The points vary based on request type:
//
//   Request type                         | Points
//   -------------------------------------|-------
//   GraphQL without mutations            | 1
//   GraphQL with mutations               | 5
//   Most REST GET, HEAD, OPTIONS         | 1
//   Most REST POST, PATCH, PUT, DELETE   | 5
//
// Some endpoints may cost different points, but these values are not publicly disclosed.
// We use the known defaults. If needed, you can add logic to detect special endpoints.
//
// This adapter tracks and applies both primary (request count) and secondary (points)
// limits internally. For now, the secondary limit logic can be embedded similarly to the primary
// logic, but if there's no known fixed second limit, we can still track it for possible
// future expansions.
//
// This code updates rate limit parsing from GitHub's response headers, which provide primary
// rate limit information. Secondary rate limit tracking would be internal, based on request points.
//
// Methods:
// - SetRateLimitDefaultsForType: Allows overriding the primary rate limit and window.
// - IdentifyRequestType: Distinguishes "graphql" vs. "rest" calls.
// - ExecuteRequest: Makes the actual request, updates internal counters for requests and points.
// - ParseRateLimitInfo: Extracts primary rate limit details (x-ratelimit-*) from GitHub headers.
// - IsRateLimitError: Checks if the response code (429 or 403) indicates a rate limit error.
//
// Note: The secondary rate limit rules are not fully integrated into an enforced limit
// here, as GitHub does not provide a direct secondary limit threshold in the prompt.
// However, we track and log points so that logic could be added if a known secondary limit is defined.

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

// Default GitHub limits for authenticated PAT (primary rate limit):
// REST: 5000 requests/hour (5000 per 3600 seconds)
// GraphQL: Also 5000/hour by default.
const (
	GitHubDefaultRestMaxRequests    = 5000
	GitHubDefaultRestWindowSecs     = 3600
	GitHubDefaultGraphQLMaxRequests = 5000
	GitHubDefaultGraphQLWindowSecs  = 3600
)

// Points per request type for secondary limit tracking (if needed):
const (
	GraphQLNoMutationsPoints   = 1
	GraphQLWithMutationsPoints = 5
	RestGetHeadOptionsPoints   = 1
	RestOtherMethodsPoints     = 5
)

type GitHubAdapter struct {
	APIToken string

	mu sync.Mutex

	// Primary limit tracking for REST:
	restMaxRequests  int
	restWindowSecs   int64
	restRequestTimes []int64

	// Primary limit tracking for GraphQL:
	graphqlMaxRequests  int
	graphqlWindowSecs   int64
	graphqlRequestTimes []int64

	// Secondary limit (points) tracking:
	// We'll track points similarly to requests. Without a known limit, we just store them.
	restPointsTimes    []int64 // timestamps of REST requests (for potential secondary limit)
	graphqlPointsTimes []int64 // timestamps of GraphQL requests (for potential secondary limit)
	// If a known secondary window and limit were known, we'd track similarly.
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
		// If primary limit would be exceeded, return synthetic 429.
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

	// Record the request timestamp
	g.recordRequest(isGraphQL)

	// Also calculate points and record them if needed
	points := g.calculateRequestPoints(req, isGraphQL)
	g.recordPoints(isGraphQL, points)

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
	// GitHub returns 429 when rate limit exceeded, and sometimes 403 for secondary rate limit.
	return resp.StatusCode == 429 || resp.StatusCode == 403
}

func (g *GitHubAdapter) IdentifyRequestType(req *resilientbridge.NormalizedRequest) string {
	if g.isGraphQLRequest(req) {
		return "graphql"
	}
	return "rest"
}

func (g *GitHubAdapter) isGraphQLRequest(req *resilientbridge.NormalizedRequest) bool {
	return req.Endpoint == "/graphql"
}

// isRateLimited checks if making another request would exceed primary limits.
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

	// Update timestamps after filtering
	if isGraphQL {
		g.graphqlRequestTimes = newTimestamps
	} else {
		g.restRequestTimes = newTimestamps
	}

	return len(newTimestamps) >= maxReq
}

// recordRequest logs the timestamp for a request (REST or GraphQL) for primary limit tracking.
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

// calculateRequestPoints determines the point cost of the request based on method and type.
func (g *GitHubAdapter) calculateRequestPoints(req *resilientbridge.NormalizedRequest, isGraphQL bool) int {
	if isGraphQL {
		// GraphQL: 1 point for queries, 5 points for mutations
		// Simple heuristic: If method is POST and body contains "mutation" keyword
		// This is a simplistic approach, real logic might need to parse the GraphQL query.
		if strings.ToUpper(req.Method) == "POST" && strings.Contains(string(req.Body), "mutation") {
			return GraphQLWithMutationsPoints
		}
		return GraphQLNoMutationsPoints
	}

	// REST:
	method := strings.ToUpper(req.Method)
	switch method {
	case "GET", "HEAD", "OPTIONS":
		return RestGetHeadOptionsPoints
	case "POST", "PATCH", "PUT", "DELETE":
		return RestOtherMethodsPoints
	default:
		// Default to 1 point for unknown methods
		return 1
	}
}

// recordPoints tracks the timestamps of requests for secondary limit calculations (if needed).
func (g *GitHubAdapter) recordPoints(isGraphQL bool, points int) {
	g.mu.Lock()
	defer g.mu.Unlock()

	// Here we just store one timestamp per point. In reality,
	// you could store cumulative sums or separate arrays.
	// For simplicity, if a request costs 5 points, we record 5 entries.
	now := time.Now().Unix()
	if isGraphQL {
		for i := 0; i < points; i++ {
			g.graphqlPointsTimes = append(g.graphqlPointsTimes, now)
		}
	} else {
		for i := 0; i < points; i++ {
			g.restPointsTimes = append(g.restPointsTimes, now)
		}
	}

	// Without a known secondary limit and window, we are not enforcing anything here.
	// If a secondary limit is introduced, you would implement similar logic to isRateLimited()
	// to filter timestamps and compare against a max points/window.
}
