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

const (
	CloudflareDefaultMaxRequests = 1200
	CloudflareDefaultWindowSecs  = 300
)

type CloudflareAdapter struct {
	APIKey string

	mu                sync.Mutex
	requestTimestamps []int64
	MaxRequests       int
	WindowSecs        int64
}

func NewCloudflareAdapter(apiKey string) *CloudflareAdapter {
	return &CloudflareAdapter{
		APIKey:            apiKey,
		MaxRequests:       CloudflareDefaultMaxRequests,
		WindowSecs:        CloudflareDefaultWindowSecs,
		requestTimestamps: []int64{},
	}
}

func (c *CloudflareAdapter) SetRateLimitDefaults(maxRequests int, windowSecs int64) {
	if maxRequests == 0 {
		maxRequests = CloudflareDefaultMaxRequests
	}
	if windowSecs == 0 {
		windowSecs = CloudflareDefaultWindowSecs
	}
	c.mu.Lock()
	c.MaxRequests = maxRequests
	c.WindowSecs = windowSecs
	c.mu.Unlock()
}

func (c *CloudflareAdapter) ExecuteRequest(req *resilientbridge.NormalizedRequest) (*resilientbridge.NormalizedResponse, error) {
	if c.isRateLimited() {
		return &resilientbridge.NormalizedResponse{
			StatusCode: 429,
			Headers:    map[string]string{},
			Data:       []byte(`{"error":"Rate limit reached"}`),
		}, nil
	}

	client := &http.Client{}
	fullURL := "https://api.cloudflare.com/client/v4" + req.Endpoint

	httpReq, err := http.NewRequest(req.Method, fullURL, bytes.NewReader(req.Body))
	if err != nil {
		return nil, err
	}

	for k, v := range req.Headers {
		httpReq.Header.Set(k, v)
	}
	if httpReq.Header.Get("Authorization") == "" && c.APIKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+c.APIKey)
	}
	if httpReq.Header.Get("Content-Type") == "" {
		httpReq.Header.Set("Content-Type", "application/json")
	}

	resp, err := client.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	c.recordRequest()

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
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now().Unix()
	windowStart := now - c.WindowSecs

	var newTimestamps []int64
	for _, ts := range c.requestTimestamps {
		if ts >= windowStart {
			newTimestamps = append(newTimestamps, ts)
		}
	}
	c.requestTimestamps = newTimestamps

	remaining := c.MaxRequests - len(c.requestTimestamps)
	var resetAt *int64
	if remaining <= 0 {
		resetTime := (windowStart + c.WindowSecs) * 1000
		resetAt = &resetTime
	}

	info := &resilientbridge.NormalizedRateLimitInfo{
		MaxRequests:       intPtr(c.MaxRequests),
		RemainingRequests: intPtr(remaining),
		ResetRequestsAt:   resetAt,
	}
	return info, nil
}

func (c *CloudflareAdapter) IsRateLimitError(resp *resilientbridge.NormalizedResponse) bool {
	return resp.StatusCode == 429
}

func (c *CloudflareAdapter) isRateLimited() bool {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now().Unix()
	windowStart := now - c.WindowSecs
	var newTimestamps []int64
	for _, ts := range c.requestTimestamps {
		if ts >= windowStart {
			newTimestamps = append(newTimestamps, ts)
		}
	}
	c.requestTimestamps = newTimestamps

	return len(c.requestTimestamps) >= c.MaxRequests
}

func (c *CloudflareAdapter) recordRequest() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.requestTimestamps = append(c.requestTimestamps, time.Now().Unix())
}

func intPtr(i int) *int {
	return &i
}
