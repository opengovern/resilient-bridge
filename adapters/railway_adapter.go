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

const (
	RailwayDefaultMaxRequests = 1000
	RailwayDefaultWindowSecs  = 3600 // 1 hour
)

type RailwayAdapter struct {
	APIToken string

	mu sync.Mutex
	// requestHistory tracks timestamps per category: "graphql" and "rest"
	requestHistory map[string][]int64

	categories map[string]struct {
		maxReq     int
		windowSecs int64
	}
}

func NewRailwayAdapter(apiToken string) *RailwayAdapter {
	return &RailwayAdapter{
		APIToken:       apiToken,
		requestHistory: make(map[string][]int64),
		categories: make(map[string]struct {
			maxReq     int
			windowSecs int64
		}),
	}
}

func (r *RailwayAdapter) SetRateLimitDefaultsForType(requestType string, maxRequests int, windowSecs int64) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if maxRequests == 0 {
		maxRequests = RailwayDefaultMaxRequests
	}
	if windowSecs == 0 {
		windowSecs = RailwayDefaultWindowSecs
	}

	r.categories[requestType] = struct {
		maxReq     int
		windowSecs int64
	}{maxRequests, windowSecs}
}

func (r *RailwayAdapter) IdentifyRequestType(req *resilientbridge.NormalizedRequest) string {
	// Railway: POST /graphql/v2 = graphql, else rest
	if req.Method == "POST" && strings.HasPrefix(req.Endpoint, "/graphql/v2") {
		return "graphql"
	}
	return "rest"
}

func (r *RailwayAdapter) ExecuteRequest(req *resilientbridge.NormalizedRequest) (*resilientbridge.NormalizedResponse, error) {
	category := r.IdentifyRequestType(req)
	if r.isRateLimited(category) {
		return &resilientbridge.NormalizedResponse{
			StatusCode: 429,
			Headers:    map[string]string{},
			Data:       []byte(`{"error":"Railway rate limit reached"}`),
		}, nil
	}

	client := &http.Client{}
	baseURL := "https://backboard.railway.app"
	fullURL := baseURL + req.Endpoint

	httpReq, err := http.NewRequest(req.Method, fullURL, bytes.NewReader(req.Body))
	if err != nil {
		return nil, err
	}

	for k, v := range req.Headers {
		httpReq.Header.Set(k, v)
	}
	if httpReq.Header.Get("Authorization") == "" && r.APIToken != "" {
		httpReq.Header.Set("Authorization", "Bearer "+r.APIToken)
	}
	if httpReq.Header.Get("Content-Type") == "" {
		httpReq.Header.Set("Content-Type", "application/json")
	}

	resp, err := client.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	r.recordRequest(category)

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

func (r *RailwayAdapter) ParseRateLimitInfo(resp *resilientbridge.NormalizedResponse) (*resilientbridge.NormalizedRateLimitInfo, error) {
	h := resp.Headers
	parseInt := func(key string) *int {
		if val, ok := h[key]; ok {
			if i, err := strconv.Atoi(val); err == nil {
				return resilientbridge.IntPtr(i)
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

func (r *RailwayAdapter) IsRateLimitError(resp *resilientbridge.NormalizedResponse) bool {
	return resp.StatusCode == 429
}

func (r *RailwayAdapter) isRateLimited(category string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	cat, ok := r.categories[category]
	if !ok {
		cat = struct {
			maxReq     int
			windowSecs int64
		}{RailwayDefaultMaxRequests, RailwayDefaultWindowSecs}
		r.categories[category] = cat
	}

	now := time.Now().Unix()
	windowStart := now - cat.windowSecs

	timestamps := r.requestHistory[category]
	var newTimestamps []int64
	for _, ts := range timestamps {
		if ts >= windowStart {
			newTimestamps = append(newTimestamps, ts)
		}
	}
	r.requestHistory[category] = newTimestamps

	return len(newTimestamps) >= cat.maxReq
}

func (r *RailwayAdapter) recordRequest(category string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	timestamps := r.requestHistory[category]
	timestamps = append(timestamps, time.Now().Unix())
	r.requestHistory[category] = timestamps
}
