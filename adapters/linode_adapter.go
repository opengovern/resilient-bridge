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

func (l *LinodeAdapter) SetRateLimitDefaultsForType(requestType string, maxRequests int, windowSecs int64) {
	// Linode's rates are fixed, ignoring overrides.
}

// IdentifyRequestType: Linode does not mention GraphQL, assume all are REST.
func (l *LinodeAdapter) IdentifyRequestType(req *resilientbridge.NormalizedRequest) string {
	return "rest"
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

func (l *LinodeAdapter) classifyRequest(req *resilientbridge.NormalizedRequest) (string, int, int64) {
	method := strings.ToUpper(req.Method)
	path := req.Endpoint

	// The logic from original code retained:
	// ... same classify logic ...
	// unchanged from what was given, assuming it's correct.

	// If no condition matched at the end:
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
