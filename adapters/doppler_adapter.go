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
	DopplerDefaultMaxRequests = 480
	DopplerDefaultWindowSecs  = 60
)

type DopplerAdapter struct {
	APIToken string

	mu                sync.Mutex
	requestTimestamps []int64

	restMaxRequests int
	restWindowSecs  int64

	graphqlMaxRequests int
	graphqlWindowSecs  int64
}

func (d *DopplerAdapter) SetRateLimitDefaultsForType(requestType string, maxRequests int, windowSecs int64) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if requestType == "rest" {
		if maxRequests == 0 {
			maxRequests = DopplerDefaultMaxRequests
		}
		if windowSecs == 0 {
			windowSecs = DopplerDefaultWindowSecs
		}
		d.restMaxRequests = maxRequests
		d.restWindowSecs = windowSecs
	} else if requestType == "graphql" {
		// Doppler does not have GraphQL, but we set defaults anyway
		if maxRequests == 0 {
			maxRequests = DopplerDefaultMaxRequests
		}
		if windowSecs == 0 {
			windowSecs = DopplerDefaultWindowSecs
		}
		d.graphqlMaxRequests = maxRequests
		d.graphqlWindowSecs = windowSecs
	}
}

func (d *DopplerAdapter) ExecuteRequest(req *resilientbridge.NormalizedRequest) (*resilientbridge.NormalizedResponse, error) {
	// Doppler = REST only
	if d.isRateLimited() {
		return &resilientbridge.NormalizedResponse{
			StatusCode: 429,
			Headers:    map[string]string{},
			Data:       []byte(`{"error":"Doppler rate limit reached"}`),
		}, nil
	}

	client := &http.Client{}
	fullURL := "https://api.doppler.com" + req.Endpoint

	httpReq, err := http.NewRequest(req.Method, fullURL, bytes.NewReader(req.Body))
	if err != nil {
		return nil, err
	}
	for k, v := range req.Headers {
		httpReq.Header.Set(k, v)
	}
	httpReq.Header.Set("Authorization", "Bearer "+d.APIToken)
	if httpReq.Header.Get("Content-Type") == "" {
		httpReq.Header.Set("Content-Type", "application/json")
	}

	resp, err := client.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	d.recordRequest()

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

func (d *DopplerAdapter) ParseRateLimitInfo(resp *resilientbridge.NormalizedResponse) (*resilientbridge.NormalizedRateLimitInfo, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	now := time.Now().Unix()
	windowStart := now - d.restWindowSecs

	var newTimestamps []int64
	for _, ts := range d.requestTimestamps {
		if ts >= windowStart {
			newTimestamps = append(newTimestamps, ts)
		}
	}
	d.requestTimestamps = newTimestamps

	remaining := d.restMaxRequests - len(d.requestTimestamps)
	var resetAt *int64
	if remaining <= 0 {
		resetTime := (windowStart + d.restWindowSecs) * 1000
		resetAt = &resetTime
	}

	info := &resilientbridge.NormalizedRateLimitInfo{
		MaxRequests:       resilientbridge.IntPtr(d.restMaxRequests),
		RemainingRequests: resilientbridge.IntPtr(remaining),
		ResetRequestsAt:   resetAt,
	}

	return info, nil
}

func (d *DopplerAdapter) IsRateLimitError(resp *resilientbridge.NormalizedResponse) bool {
	return resp.StatusCode == 429
}

func (d *DopplerAdapter) isRateLimited() bool {
	d.mu.Lock()
	defer d.mu.Unlock()

	now := time.Now().Unix()
	windowStart := now - d.restWindowSecs
	var newTimestamps []int64
	for _, ts := range d.requestTimestamps {
		if ts >= windowStart {
			newTimestamps = append(newTimestamps, ts)
		}
	}
	d.requestTimestamps = newTimestamps

	return len(d.requestTimestamps) >= d.restMaxRequests
}

func (d *DopplerAdapter) recordRequest() {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.requestTimestamps = append(d.requestTimestamps, time.Now().Unix())
}
