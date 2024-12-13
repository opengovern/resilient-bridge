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
	requestHistory map[string][]int64 // key: action
}

func NewLinodeAdapter(apiToken string) *LinodeAdapter {
	return &LinodeAdapter{
		APIToken:       apiToken,
		requestHistory: make(map[string][]int64),
	}
}

// We won't implement SetRateLimitDefaultsForType here because Linode's rates are fixed.
func (l *LinodeAdapter) SetRateLimitDefaultsForType(requestType string, maxRequests int, windowSecs int64) {
	// Linode has fixed limits, ignoring overrides.
}

func (l *LinodeAdapter) ExecuteRequest(req *resilientbridge.NormalizedRequest) (*resilientbridge.NormalizedResponse, error) {
	action, limit, window := l.classifyRequest(req)
	if l.isRateLimited(action, limit, window) {
		return &resilientbridge.NormalizedResponse{
			StatusCode: 429,
			Headers:    map[string]string{},
			Data:       []byte(`{"error":"Linode rate limit reached"}`),
		}, nil
	}

	client := &http.Client{}
	baseURL := "https://api.linode.com/v4" // assumed base
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

// classifyRequest determines action and returns (action, limit, window_seconds)
func (l *LinodeAdapter) classifyRequest(req *resilientbridge.NormalizedRequest) (string, int, int64) {
	method := strings.ToUpper(req.Method)
	path := req.Endpoint

	// Special actions:
	// Create Linode: POST /linode/instances
	if method == "POST" && strings.HasPrefix(path, "/linode/instances") && !strings.Contains(path, "/") {
		// Actually, /linode/instances might also be followed by ID. If no ID, it's create.
		// Check if path exactly "/linode/instances" or "/linode/instances?"
		parts := strings.Split(strings.TrimPrefix(path, "/"), "/")
		if len(parts) == 2 && parts[0] == "linode" && parts[1] == "instances" {
			return "create_linode", 5, 15 // 5 req/15s
		}
	}

	// Create volume: POST /volumes
	if method == "POST" && strings.HasPrefix(path, "/volumes") && (path == "/volumes" || path == "/volumes?") {
		return "create_volume", 25, 60 // 25/min
	}

	// List images: GET /images
	if method == "GET" && strings.HasPrefix(path, "/images") && (path == "/images" || strings.Contains(path, "/images?")) {
		return "list_images", 20, 60 // 20/min
	}

	// Stats endpoints contain /stats:
	// For example: GET /linode/instances/{id}/stats
	// We'll just check if '/stats' appears:
	if method == "GET" && strings.Contains(path, "/stats") {
		return "stats_operation", 50, 60 // 50/min
	}

	// Object Storage operations:
	// Any endpoint containing /object-storage:
	if strings.Contains(path, "/object-storage") {
		return "object_storage", 750, 1 // 750/s
	}

	// Open support ticket: POST /support/tickets
	if method == "POST" && strings.HasPrefix(path, "/support/tickets") {
		return "open_ticket", 2, 60 // 2/min
	}

	// Accept a service transfer:
	// POST /account/service-transfers/{transferId}/accept
	if method == "POST" && strings.Contains(path, "/account/service-transfers/") && strings.HasSuffix(path, "/accept") {
		return "accept_service_transfer", 2, 60 // 2/min
	}

	// Now fallback:
	// If GET and likely returns a collection (paginated):
	// We guess if the endpoint is plural and no id at the end, treat as get_paginated
	// A simplistic approach: If method=GET and does not contain a numeric ID or additional segment after resource name:
	if method == "GET" {
		// Check if path likely ends at a collection endpoint:
		// We'll assume if the path ends right after a resource name or has query params but no trailing id
		// If there's a numeric ID at the end or a known pattern (like "/linode/instances/123"), skip.
		// For simplicity: If no second level ID segment at the end:
		parts := strings.Split(path, "/")
		if len(parts) > 2 {
			// Check last part if numeric
			last := parts[len(parts)-1]
			if isNumeric(last) {
				// probably a single resource GET => default to 800/min
				return "get_single_resource", 800, 60
			}
		}
		// assume paginated collection
		return "get_paginated", 200, 60
	}

	// Default (non-GET operations): 800 req/min
	return "default_action", 800, 60
}

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

func (l *LinodeAdapter) recordRequest(action string) {
	l.mu.Lock()
	defer l.mu.Unlock()

	timestamps := l.requestHistory[action]
	timestamps = append(timestamps, time.Now().Unix())
	l.requestHistory[action] = timestamps
}

func isNumeric(s string) bool {
	_, err := strconv.Atoi(s)
	return err == nil
}
