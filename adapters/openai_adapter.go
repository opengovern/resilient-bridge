package adapters

import (
	"bytes"
	"errors"
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

	mu                sync.Mutex
	requestTimestamps []int64

	restMaxRequests int
	restWindowSecs  int64
}

func NewOpenAIAdapter(apiKey string) *OpenAIAdapter {
	return &OpenAIAdapter{
		APIKey:          apiKey,
		restMaxRequests: OpenAIDefaultMaxRequests,
		restWindowSecs:  OpenAIDefaultWindowSecs,
	}
}

func (o *OpenAIAdapter) SetRateLimitDefaultsForType(requestType string, maxRequests int, windowSecs int64) {
	o.mu.Lock()
	defer o.mu.Unlock()

	if requestType == "rest" {
		if maxRequests == 0 {
			maxRequests = OpenAIDefaultMaxRequests
		}
		if windowSecs == 0 {
			windowSecs = OpenAIDefaultWindowSecs
		}
		o.restMaxRequests = maxRequests
		o.restWindowSecs = windowSecs
	}
	// Ignore "graphql" since OpenAI doesn't use it.
}

func (o *OpenAIAdapter) IdentifyRequestType(req *resilientbridge.NormalizedRequest) string {
	// OpenAI is REST-only.
	return "rest"
}

func (o *OpenAIAdapter) ExecuteRequest(req *resilientbridge.NormalizedRequest) (*resilientbridge.NormalizedResponse, error) {
	if o.isRateLimited() {
		// We can return a synthetic 429 if desired, but let's not do that.
		// Just proceed and let the actual request fail if it does.
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

	if httpReq.Header.Get("Authorization") == "" && o.APIKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+o.APIKey)
	}
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

	response := &resilientbridge.NormalizedResponse{
		StatusCode: resp.StatusCode,
		Headers:    headers,
		Data:       data,
	}

	// If the actual response from the provider is 429, return an error
	// The SDK's request executor will handle retries.
	if resp.StatusCode == 429 {
		return response, errors.New("openai: rate limit exceeded (429)")
	}

	return response, nil
}

func (o *OpenAIAdapter) ParseRateLimitInfo(resp *resilientbridge.NormalizedResponse) (*resilientbridge.NormalizedRateLimitInfo, error) {
	o.mu.Lock()
	defer o.mu.Unlock()

	now := time.Now().Unix()
	windowStart := now - o.restWindowSecs
	var newTimestamps []int64
	for _, ts := range o.requestTimestamps {
		if ts >= windowStart {
			newTimestamps = append(newTimestamps, ts)
		}
	}
	o.requestTimestamps = newTimestamps

	remaining := o.restMaxRequests - len(o.requestTimestamps)
	var resetAt *int64
	if remaining <= 0 {
		resetTime := (windowStart + o.restWindowSecs) * 1000
		resetAt = &resetTime
	}

	info := &resilientbridge.NormalizedRateLimitInfo{
		MaxRequests:       resilientbridge.IntPtr(o.restMaxRequests),
		RemainingRequests: resilientbridge.IntPtr(remaining),
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
	for _, ts := range o.requestTimestamps {
		if ts >= windowStart {
			newTimestamps = append(newTimestamps, ts)
		}
	}
	o.requestTimestamps = newTimestamps

	return len(o.requestTimestamps) >= o.restMaxRequests
}

func (o *OpenAIAdapter) recordRequest() {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.requestTimestamps = append(o.requestTimestamps, time.Now().Unix())
}
