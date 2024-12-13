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

type GitGuardianAdapter struct {
	APIToken   string
	APIKeyType string // "personal" or "service"
	IsPaidPlan bool

	mu             sync.Mutex
	requestHistory []int64 // timestamps of requests in seconds
}

func NewGitGuardianAdapter(apiToken string, apiKeyType string, isPaidPlan bool) *GitGuardianAdapter {
	return &GitGuardianAdapter{
		APIToken:   apiToken,
		APIKeyType: apiKeyType,
		IsPaidPlan: isPaidPlan,
	}
}

func (g *GitGuardianAdapter) SetRateLimitDefaultsForType(requestType string, maxRequests int, windowSecs int64) {
	// GitGuardian has fixed global limits, ignoring overrides.
}

// IdentifyRequestType: GitGuardian API does not mention GraphQL, assume all are REST.
func (g *GitGuardianAdapter) IdentifyRequestType(req *resilientbridge.NormalizedRequest) string {
	return "rest"
}

func (g *GitGuardianAdapter) ExecuteRequest(req *resilientbridge.NormalizedRequest) (*resilientbridge.NormalizedResponse, error) {
	limit := g.getRateLimit()
	if limit > 0 && g.isRateLimited(limit, 60) {
		return &resilientbridge.NormalizedResponse{
			StatusCode: 429,
			Headers:    map[string]string{},
			Data:       []byte(`{"error":"GitGuardian rate limit reached"}`),
		}, nil
	}

	client := &http.Client{}
	baseURL := "https://api.gitguardian.com"
	fullURL := baseURL + req.Endpoint

	httpReq, err := http.NewRequest(req.Method, fullURL, bytes.NewReader(req.Body))
	if err != nil {
		return nil, err
	}

	for k, v := range req.Headers {
		httpReq.Header.Set(k, v)
	}
	if httpReq.Header.Get("Authorization") == "" && g.APIToken != "" {
		httpReq.Header.Set("Authorization", "Token "+g.APIToken)
	}
	if httpReq.Header.Get("Content-Type") == "" {
		httpReq.Header.Set("Content-Type", "application/json")
	}

	resp, err := client.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	g.recordRequest()

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

func (g *GitGuardianAdapter) ParseRateLimitInfo(resp *resilientbridge.NormalizedResponse) (*resilientbridge.NormalizedRateLimitInfo, error) {
	// No documented rate limit headers to parse.
	return nil, nil
}

func (g *GitGuardianAdapter) IsRateLimitError(resp *resilientbridge.NormalizedResponse) bool {
	return resp.StatusCode == 429
}

func (g *GitGuardianAdapter) getRateLimit() int {
	if g.APIKeyType == "service" {
		// Service account key
		if !g.IsPaidPlan {
			// Not available under free plan, assume no limit or handle error differently
			return 0
		}
		return 1000 // requests/min for service account on paid plan
	} else {
		// Personal access token
		if g.IsPaidPlan {
			return 200 // paid plan PAT
		}
		return 50 // free plan PAT
	}
}

func (g *GitGuardianAdapter) isRateLimited(limit int, windowSecs int64) bool {
	if limit <= 0 {
		return false
	}

	g.mu.Lock()
	defer g.mu.Unlock()

	now := time.Now().Unix()
	windowStart := now - windowSecs

	var newTimestamps []int64
	for _, ts := range g.requestHistory {
		if ts >= windowStart {
			newTimestamps = append(newTimestamps, ts)
		}
	}
	if len(newTimestamps) >= limit {
		return true
	}

	g.requestHistory = newTimestamps
	return false
}

func (g *GitGuardianAdapter) recordRequest() {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.requestHistory = append(g.requestHistory, time.Now().Unix())
}
