// linode_adapter.go
// -----------------
// This adapter integrates with the Linode API, assigning different rate limits based on the type of request.
// It classifies requests into categories (e.g., create_linode, create_volume, list_images, stats_operation, etc.)
// and applies corresponding rate limits and windows.
// If a request would exceed the assigned rate limit for that action, it returns a synthetic 429 before sending the request.
//
// The code parses Linode's rate limits from response headers for informational purposes, but uses predefined logic
// to limit requests by action type.
//
// Example categories and their limits:
// - create_linode: 5 requests per 15 seconds
// - create_volume: 25 requests per minute
// - list_images: 20 requests per minute
// - stats_operation: 50 requests per minute
// - object_storage: 750 requests per second
// - open_ticket: 2 requests per minute
// - accept_service_transfer: 2 requests per minute
// - get_paginated: 200 requests per minute (for listing resources)
// - get_single_resource: 800 requests per minute
// - default_action (other non-GET calls): 800 requests per minute

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

type LinodeAdapter struct {
	APIToken string

	mu             sync.Mutex
	requestHistory map[string][]int64 // key: action, value: timestamps of recent requests
}

// NewLinodeAdapter creates a LinodeAdapter with an API token.
func NewLinodeAdapter(apiToken string) *LinodeAdapter {
	return &LinodeAdapter{
		APIToken:       apiToken,
		requestHistory: make(map[string][]int64),
	}
}

// SetRateLimitDefaultsForType: Linode rates are considered fixed, ignoring overrides.
func (l *LinodeAdapter) SetRateLimitDefaultsForType(requestType string, maxRequests int, windowSecs int64) {
	// No custom logic since Linode rates are pre-defined per action category.
}

// IdentifyRequestType returns "rest" since Linode does not use GraphQL in this integration.
func (l *LinodeAdapter) IdentifyRequestType(req *resilientbridge.NormalizedRequest) string {
	return "rest"
}

func (l *LinodeAdapter) ExecuteRequest(req *resilientbridge.NormalizedRequest) (*resilientbridge.NormalizedResponse, error) {
	action, limit, window := l.classifyRequest(req)
	if l.isRateLimited(action, limit, window) {
		// If rate-limited, return a synthetic 429 before making the request.
		return &resilientbridge.NormalizedResponse{
			StatusCode: 429,
			Headers:    map[string]string{},
			Data:       []byte(`{"error":"Linode rate limit reached"}`),
		}, nil
	}

	client := &http.Client{}
	baseURL := "https://api.linode.com/v4"
	fullURL := baseURL + req.Endpoint

	httpReq, err := http.NewRequest(req.Method, fullURL, bytes.NewReader(req.Body))
	if err != nil {
		return nil, err
	}

	for k, v := range req.Headers {
		httpReq.Header.Set(k, v)
	}
	if httpReq.Header.Get("Authorization") == "" && l.APIToken != "" {
		httpReq.Header.Set("Authorization", "Bearer "+l.APIToken)
	}
	if httpReq.Header.Get("Content-Type") == "" {
		httpReq.Header.Set("Content-Type", "application/json")
	}

	resp, err := client.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	// Record the request timestamp after a successful execution
	l.recordRequest(action)

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

func (l *LinodeAdapter) ParseRateLimitInfo(resp *resilientbridge.NormalizedResponse) (*resilientbridge.NormalizedRateLimitInfo, error) {
	// Linode returns X-RateLimit-Limit, X-RateLimit-Remaining, X-RateLimit-Reset
	// Parse these for informational use.
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
				// convert seconds to ms
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

func (l *LinodeAdapter) IsRateLimitError(resp *resilientbridge.NormalizedResponse) bool {
	return resp.StatusCode == 429
}

// classifyRequest determines the action category and returns (action, limit, window_seconds).
// Different endpoints and methods map to different rate limits, as documented above.
func (l *LinodeAdapter) classifyRequest(req *resilientbridge.NormalizedRequest) (string, int, int64) {
	method := strings.ToUpper(req.Method)
	path := req.Endpoint

	// Create a Linode: 5 req/15s
	if method == "POST" && strings.HasPrefix(path, "/linode/instances") && !strings.Contains(strings.TrimPrefix(path, "/linode/instances"), "/") {
		return "create_linode", 5, 15
	}

	// Create a volume: POST /volumes = 25 req/min
	if method == "POST" && strings.HasPrefix(path, "/volumes") && (path == "/volumes" || path == "/volumes?") {
		return "create_volume", 25, 60
	}

	// List images: GET /images = 20 req/min
	if method == "GET" && strings.HasPrefix(path, "/images") && (path == "/images" || strings.Contains(path, "/images?")) {
		return "list_images", 20, 60
	}

	// Stats operation: GET something/stats = 50 req/min
	if method == "GET" && strings.Contains(path, "/stats") {
		return "stats_operation", 50, 60
	}

	// Object storage: any endpoint containing /object-storage = 750 req/s
	if strings.Contains(path, "/object-storage") {
		return "object_storage", 750, 1
	}

	// Open a support ticket: POST /support/tickets = 2 req/min
	if method == "POST" && strings.HasPrefix(path, "/support/tickets") {
		return "open_ticket", 2, 60
	}

	// Accept a service transfer: POST /account/service-transfers/xxx/accept = 2 req/min
	if method == "POST" && strings.Contains(path, "/account/service-transfers/") && strings.HasSuffix(path, "/accept") {
		return "accept_service_transfer", 2, 60
	}

	// For GET requests, distinguish between fetching a single resource (with ID) or paginated lists:
	if method == "GET" {
		parts := strings.Split(path, "/")
		if len(parts) > 2 {
			// Check last part if numeric
			last := parts[len(parts)-1]
			if isNumeric(last) {
				// single resource
				return "get_single_resource", 800, 60
			}
		}
		// otherwise, assume it's a collection
		return "get_paginated", 200, 60
	}

	// Default non-GET actions: 800 req/min
	return "default_action", 800, 60
}

// isRateLimited checks if the given action has hit its rate limit.
func (l *LinodeAdapter) isRateLimited(action string, limit int, windowSecs int64) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := time.Now().Unix()
	windowStart := now - windowSecs
	timestamps := l.requestHistory[action]
	var newTimestamps []int64
	for _, ts := range timestamps {
		if ts >= windowStart {
			newTimestamps = append(newTimestamps, ts)
		}
	}
	l.requestHistory[action] = newTimestamps

	return len(newTimestamps) >= limit
}

// recordRequest stores the current timestamp for the given action's request.
func (l *LinodeAdapter) recordRequest(action string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now().Unix()
	l.requestHistory[action] = append(l.requestHistory[action], now)
}

// isNumeric checks if a string consists only of digits.
func isNumeric(s string) bool {
	_, err := strconv.Atoi(s)
	return err == nil
}
