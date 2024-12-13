// github_adapter.go
// -----------------
// This adapter integrates with the GitHub API, handling both REST and GraphQL endpoints with distinct rate limits.
// By default, GitHub provides rate limit headers with each response. We also have the option to proactively
// check rate limits ahead of making other calls by querying the /rate_limit endpoint once if desired.
//
// Key Points:
// - Rate limits:
//   * REST: 5000 requests/hour by default
//   * GraphQL: 5000 requests/hour by default
// - We differentiate between "rest" and "graphql" requests.
// - On the first request (if CHECK_REQUEST_RATE_LIMIT_AHEAD = true), we call GET /rate_limit once to
//   proactively fetch current rate limits without counting against primary rate limit.
// - If 429 or 403 is encountered, consider it a rate limit error.
// - We'll parse rate limit headers from each response to keep track of the current state.
//
// Note: Secondary rate limits and request "points" are not explicitly tracked in this example,
// but could be added if GitHub documents them more specifically. Here we rely on standard headers.
//
// If at any point headers are missing or can't be parsed, we fallback to known defaults.

package adapters

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	resilientbridge "github.com/opengovern/resilient-bridge"
)

const (
	GitHubDefaultRestMaxRequests    = 5000
	GitHubDefaultRestWindowSecs     = 3600 // 1 hour
	GitHubDefaultGraphQLMaxRequests = 5000
	GitHubDefaultGraphQLWindowSecs  = 3600

	// Set this to true if you want to proactively check the rate limit before the first request
	CHECK_REQUEST_RATE_LIMIT_AHEAD = false
)

type GitHubAdapter struct {
	APIToken string

	mu sync.Mutex

	// Configured max and windows
	restMaxRequests    int
	restWindowSecs     int64
	graphqlMaxRequests int
	graphqlWindowSecs  int64

	restRequestTimes    []int64
	graphqlRequestTimes []int64

	// Indicates if we've performed the initial rate limit check
	didInitialRateCheck bool
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

func (g *GitHubAdapter) IdentifyRequestType(req *resilientbridge.NormalizedRequest) string {
	if g.isGraphQLRequest(req) {
		return "graphql"
	}
	return "rest"
}

func (g *GitHubAdapter) ExecuteRequest(req *resilientbridge.NormalizedRequest) (*resilientbridge.NormalizedResponse, error) {
	isGraphQL := g.isGraphQLRequest(req)

	// If CHECK_REQUEST_RATE_LIMIT_AHEAD is true and we haven't done the initial check, do it now.
	if CHECK_REQUEST_RATE_LIMIT_AHEAD {
		g.mu.Lock()
		shouldCheck := !g.didInitialRateCheck
		g.didInitialRateCheck = true
		g.mu.Unlock()

		if shouldCheck {
			if err := g.checkInitialRateLimit(); err != nil {
				// Not a fatal error if we fail, just log and continue
				fmt.Printf("Warning: failed initial rate limit check: %v\n", err)
			}
		}
	}

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
	// 429 or 403 can indicate rate limits
	return resp.StatusCode == 429 || resp.StatusCode == 403
}

func (g *GitHubAdapter) isGraphQLRequest(req *resilientbridge.NormalizedRequest) bool {
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

// checkInitialRateLimit calls the /rate_limit endpoint once to proactively fetch the current
// rate limit info. This call does not count against the primary rate limit, but can affect secondary limits.
// If successful, we use the returned data to update our known limits if needed.
func (g *GitHubAdapter) checkInitialRateLimit() error {
	client := &http.Client{}
	req, err := http.NewRequest("GET", "https://api.github.com/rate_limit", nil)
	if err != nil {
		return err
	}
	if g.APIToken != "" {
		req.Header.Set("Authorization", "Bearer "+g.APIToken)
	}

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("GET /rate_limit returned %d: %s", resp.StatusCode, resp.Status)
	}

	data, _ := io.ReadAll(resp.Body)
	// JSON structure for rate_limit:
	// {
	//   "resources": {
	//     "core": {
	//       "limit": 5000,
	//       "remaining": 4999,
	//       "reset": 1372700873,
	//       "used": 1,
	//       "resource": "core"
	//     },
	//     "graphql": {
	//       "limit": 5000,
	//       "remaining": 5000,
	//       "reset": 1372700873,
	//       "used": 0,
	//       "resource": "graphql"
	//     }
	//   },
	//   "rate": {
	//       "limit": 5000,
	//       "remaining": 4999,
	//       "reset": 1372700873,
	//       "used": 1
	//   }
	// }

	type ResourceInfo struct {
		Limit     int `json:"limit"`
		Remaining int `json:"remaining"`
		Reset     int `json:"reset"`
		Used      int `json:"used"`
	}

	type RateLimitResp struct {
		Resources struct {
			Core    ResourceInfo `json:"core"`
			Graphql ResourceInfo `json:"graphql"`
		} `json:"resources"`
		Rate ResourceInfo `json:"rate"`
	}

	var r RateLimitResp
	if err := json.Unmarshal(data, &r); err != nil {
		return err
	}

	// Update internal maxRequests/window if needed
	g.mu.Lock()
	defer g.mu.Unlock()

	// The returned limit is always the same (5000/hour) for normal use, but if GitHub changes it or we have a special token,
	// we could reflect it here. For now, we just trust the returned limits.
	// Convert reset from epoch seconds to window in seconds:
	// If we want to adapt the windows: The reset is a timestamp, not a window. We rely on relative logic from headers normally.
	// For now, we won't override our known windows/time. The main purpose is to confirm we can call this endpoint and get info.
	// If we wanted to, we could compute a window based on the difference between now and reset.
	// For simplicity, do nothing more than logging:
	fmt.Printf("Initial rate limit (core): Limit=%d, Remaining=%d, Reset=%d\n", r.Resources.Core.Limit, r.Resources.Core.Remaining, r.Resources.Core.Reset)
	fmt.Printf("Initial rate limit (graphql): Limit=%d, Remaining=%d, Reset=%d\n", r.Resources.Graphql.Limit, r.Resources.Graphql.Remaining, r.Resources.Graphql.Reset)

	return nil
}
