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
	OpenAIDefaultMaxRequests = 60
	OpenAIDefaultWindowSecs  = 60
)

type OpenAIAdapter struct {
	APIKey string

	mu               sync.Mutex
	restMaxRequests  int
	restWindowSecs   int64
	restRequestTimes []int64
}

func (o *OpenAIAdapter) SetRateLimitDefaultsForType(requestType string, maxRequests int, windowSecs int64) {
	o.mu.Lock()
	defer o.mu.Unlock()

	// Since OpenAI doesn't support GraphQL, we only implement "rest"
	if requestType == "rest" {
		if maxRequests == 0 {
			maxRequests = OpenAIDefaultMaxRequests
		}
		if windowSecs == 0 {
			windowSecs = OpenAIDefaultWindowSecs
		}
		o.restMaxRequests = maxRequests
		o.restWindowSecs = windowSecs
	} else if requestType == "graphql" {
		// OpenAI does not support GraphQL. Ignore or set same defaults without usage.
		// We'll just silently ignore this since no GraphQL requests should be made to OpenAI.
	}
}

func (o *OpenAIAdapter) ExecuteRequest(req *resilientbridge.NormalizedRequest) (*resilientbridge.NormalizedResponse, error) {
	if o.isRateLimited() {
		return &resilientbridge.NormalizedResponse{
			StatusCode: 429,
			Headers:    map[string]string{},
			Data:       []byte(`{"error":"OpenAI rate limit reached"}`),
		}, nil
	}

	client := &http.Client{}
	fullURL := "https://api.openai.com" + req.Endpoint

	httpReq, err := http.NewRequest(req.Method, fullURL, bytes.NewReader(req.Body))
	if err != nil {
		return nil, err
	}

	for k, v := range req.Headers {
		httpReq.Header.Set(k, v)
	}
	httpReq.Header.Set("Authorization", "Bearer "+o.APIKey)
	if httpReq.Header.Get("Content-Type") == "" {
		httpReq.Header.Set("Content-Type", "application/json")
	}

	resp, err := client.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	o.recordRequest()

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

func (o *OpenAIAdapter) ParseRateLimitInfo(resp *resilientbridge.NormalizedResponse) (*resilientbridge.NormalizedRateLimitInfo, error) {
	o.mu.Lock()
	defer o.mu.Unlock()

	now := time.Now().Unix()
	windowStart := now - o.restWindowSecs
	var newTimestamps []int64
	for _, ts := range o.restRequestTimes {
		if ts >= windowStart {
			newTimestamps = append(newTimestamps, ts)
		}
	}
	o.restRequestTimes = newTimestamps

	remaining := o.restMaxRequests - len(o.restRequestTimes)
	var resetAt *int64
	if remaining <= 0 {
		resetTime := (windowStart + o.restWindowSecs) * 1000
		resetAt = &resetTime
	}

	info := &resilientbridge.NormalizedRateLimitInfo{
		MaxRequests:       intPtr(o.restMaxRequests),
		RemainingRequests: intPtr(remaining),
		ResetRequestsAt:   resetAt,
	}
	return info, nil
}

func (o *OpenAIAdapter) IsRateLimitError(resp *resilientbridge.NormalizedResponse) bool {
	return resp.StatusCode == 429
}

func (o *OpenAIAdapter) isRateLimited() bool {
	o.mu.Lock()
	defer o.mu.Unlock()

	now := time.Now().Unix()
	windowStart := now - o.restWindowSecs
	var newTimestamps []int64
	for _, ts := range o.restRequestTimes {
		if ts >= windowStart {
			newTimestamps = append(newTimestamps, ts)
		}
	}
	o.restRequestTimes = newTimestamps

	return len(o.restRequestTimes) >= o.restMaxRequests
}

func (o *OpenAIAdapter) recordRequest() {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.restRequestTimes = append(o.restRequestTimes, time.Now().Unix())
}

func intPtr(i int) *int {
	return &i
}
