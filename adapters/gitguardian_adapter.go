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
	// GitGuardian has fixed global limits per key type and plan, ignoring overrides.
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

	// Record the request after it's done
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
	// GitGuardian docs don't mention rate limit headers besides error code 429.
	// If they exist, parse them here. For now, return nil.
	return nil, nil
}

func (g *GitGuardianAdapter) IsRateLimitError(resp *resilientbridge.NormalizedResponse) bool {
	return resp.StatusCode == 429
}

// Determine the rate limit based on API key type and plan
func (g *GitGuardianAdapter) getRateLimit() int {
	// If self-hosted or no limit applies, return 0 or implement a toggle
	// For now, we assume SaaS as per instructions
	if g.APIKeyType == "service" {
		// Service account key
		if !g.IsPaidPlan {
			// Service accounts are not available under Free plan
			// If somehow used, fallback or just assume paid plan
			return 0
		}
		return 1000 // requests/min
	} else {
		// Personal access token
		if g.IsPaidPlan {
			return 200
		}
		return 50
	}
}

// Checks if the next request would exceed the rate limit within the given window
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
	// At this point, newTimestamps are the requests in the last window
	if len(newTimestamps) >= limit {
		// Already at the limit
		return true
	}

	// Otherwise not rate-limited
	g.requestHistory = newTimestamps
	return false
}

func (g *GitGuardianAdapter) recordRequest() {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.requestHistory = append(g.requestHistory, time.Now().Unix())
}
