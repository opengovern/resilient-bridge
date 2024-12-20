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

// Default rate limit assumptions. HuggingFace doesn't clearly document them,
// so we set some generous defaults and rely on 429 handling.
// These can be overridden by SetRateLimitDefaultsForType calls.
const (
	HuggingFaceDefaultReadMaxRequests  = 5000
	HuggingFaceDefaultReadWindowSecs   = 3600
	HuggingFaceDefaultWriteMaxRequests = 200
	HuggingFaceDefaultWriteWindowSecs  = 3600
)

// HuggingFaceAdapter implements the ProviderAdapter interface for HuggingFace Hub.
// It can perform requests to HuggingFace Hub APIs, e.g. GET /api/models.
type HuggingFaceAdapter struct {
	APIToken string

	mu sync.Mutex

	// Rate limit configuration per request type
	readMaxRequests  int
	readWindowSecs   int64
	writeMaxRequests int
	writeWindowSecs  int64

	readRequestTimes  []int64
	writeRequestTimes []int64
}

// NewHuggingFaceAdapter creates a new HuggingFace adapter.
func NewHuggingFaceAdapter(apiToken string) *HuggingFaceAdapter {
	return &HuggingFaceAdapter{
		APIToken:         apiToken,
		readMaxRequests:  HuggingFaceDefaultReadMaxRequests,
		readWindowSecs:   HuggingFaceDefaultReadWindowSecs,
		writeMaxRequests: HuggingFaceDefaultWriteMaxRequests,
		writeWindowSecs:  HuggingFaceDefaultWriteWindowSecs,
	}
}

// SetRateLimitDefaultsForType allows overriding the default rate limits for read/write requests.
func (h *HuggingFaceAdapter) SetRateLimitDefaultsForType(requestType string, maxRequests int, windowSecs int64) {
	h.mu.Lock()
	defer h.mu.Unlock()

	switch requestType {
	case "read":
		if maxRequests > 0 {
			h.readMaxRequests = maxRequests
		}
		if windowSecs > 0 {
			h.readWindowSecs = windowSecs
		}
	case "write":
		if maxRequests > 0 {
			h.writeMaxRequests = maxRequests
		}
		if windowSecs > 0 {
			h.writeWindowSecs = windowSecs
		}
	default:
		// Unknown request type, do nothing or log a warning
	}
}

// IdentifyRequestType categorizes requests. HuggingFace Hub requests are often GET for reading info.
// For simplicity, GET requests -> "read", others -> "write". Adjust as needed.
func (h *HuggingFaceAdapter) IdentifyRequestType(req *resilientbridge.NormalizedRequest) string {
	if strings.ToUpper(req.Method) == "GET" {
		return "read"
	}
	return "write"
}

// ExecuteRequest performs the HTTP request.
func (h *HuggingFaceAdapter) ExecuteRequest(req *resilientbridge.NormalizedRequest) (*resilientbridge.NormalizedResponse, error) {
	requestType := h.IdentifyRequestType(req)
	if h.isRateLimited(requestType) {
		// Return a simulated 429 to trigger backoff/retries
		return &resilientbridge.NormalizedResponse{
			StatusCode: 429,
			Headers:    map[string]string{},
			Data:       []byte(`{"error":"HuggingFace rate limit reached"}`),
		}, nil
	}

	client := &http.Client{}
	baseURL := "https://huggingface.co"
	fullURL := baseURL + req.Endpoint

	httpReq, err := http.NewRequest(req.Method, fullURL, bytes.NewReader(req.Body))
	if err != nil {
		return nil, err
	}

	for k, v := range req.Headers {
		httpReq.Header.Set(k, v)
	}

	if httpReq.Header.Get("Authorization") == "" && h.APIToken != "" {
		httpReq.Header.Set("Authorization", "Bearer "+h.APIToken)
	}

	// Set Content-Type if not already set
	if httpReq.Header.Get("Content-Type") == "" {
		httpReq.Header.Set("Content-Type", "application/json")
	}

	resp, err := client.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	h.recordRequest(requestType)

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

// ParseRateLimitInfo attempts to parse rate limit info from response headers.
// HuggingFace docs do not mention standard rate limit headers as of now.
// If they are introduced, parse them here.
func (h *HuggingFaceAdapter) ParseRateLimitInfo(resp *resilientbridge.NormalizedResponse) (*resilientbridge.NormalizedRateLimitInfo, error) {
	// Example: If in the future HuggingFace adds headers like:
	// X-RateLimit-Limit, X-RateLimit-Remaining, X-RateLimit-Reset
	// We can parse them here.
	parseInt := func(key string) *int {
		if val, ok := resp.Headers[key]; ok {
			if i, err := strconv.Atoi(val); err == nil {
				return &i
			}
		}
		return nil
	}

	parseReset := func(key string) *int64 {
		if val, ok := resp.Headers[key]; ok {
			if ts, err := strconv.ParseInt(val, 10, 64); err == nil {
				ms := ts * 1000
				return &ms
			}
		}
		return nil
	}

	// Placeholder: If no rate limit headers exist, return nil.
	info := &resilientbridge.NormalizedRateLimitInfo{
		MaxRequests:       parseInt("x-ratelimit-limit"),
		RemainingRequests: parseInt("x-ratelimit-remaining"),
		ResetRequestsAt:   parseReset("x-ratelimit-reset"),
	}

	if info.MaxRequests == nil && info.RemainingRequests == nil && info.ResetRequestsAt == nil {
		return nil, nil
	}
	return info, nil
}

// IsRateLimitError returns true if the response status code indicates hitting rate limits.
func (h *HuggingFaceAdapter) IsRateLimitError(resp *resilientbridge.NormalizedResponse) bool {
	return resp.StatusCode == 429
}

func (h *HuggingFaceAdapter) isRateLimited(requestType string) bool {
	h.mu.Lock()
	defer h.mu.Unlock()

	var maxReq int
	var windowSecs int64
	var timestamps []int64

	now := time.Now().Unix()

	if requestType == "read" {
		maxReq = h.readMaxRequests
		windowSecs = h.readWindowSecs
		timestamps = h.readRequestTimes
	} else {
		maxReq = h.writeMaxRequests
		windowSecs = h.writeWindowSecs
		timestamps = h.writeRequestTimes
	}

	windowStart := now - windowSecs
	var newTimestamps []int64
	for _, ts := range timestamps {
		if ts >= windowStart {
			newTimestamps = append(newTimestamps, ts)
		}
	}

	if requestType == "read" {
		h.readRequestTimes = newTimestamps
	} else {
		h.writeRequestTimes = newTimestamps
	}

	return len(newTimestamps) >= maxReq
}

func (h *HuggingFaceAdapter) recordRequest(requestType string) {
	h.mu.Lock()
	defer h.mu.Unlock()

	now := time.Now().Unix()
	if requestType == "read" {
		h.readRequestTimes = append(h.readRequestTimes, now)
	} else {
		h.writeRequestTimes = append(h.writeRequestTimes, now)
	}
}
