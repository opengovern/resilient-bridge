package adapters

import (
	"bytes"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	reslientbridge "github.com/opengovern/resilient-bridge"
)

type DopplerAdapter struct {
	APIToken string
}

func (d *DopplerAdapter) ExecuteRequest(req *reslientbridge.NormalizedRequest) (*reslientbridge.NormalizedResponse, error) {
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

	data, _ := io.ReadAll(resp.Body)
	headers := make(map[string]string)
	for k, vals := range resp.Header {
		if len(vals) > 0 {
			headers[strings.ToLower(k)] = vals[0]
		}
	}

	return &reslientbridge.NormalizedResponse{
		StatusCode: resp.StatusCode,
		Headers:    headers,
		Data:       data,
	}, nil
}

func (d *DopplerAdapter) ParseRateLimitInfo(resp *reslientbridge.NormalizedResponse) (*reslientbridge.NormalizedRateLimitInfo, error) {
	h := resp.Headers
	parseInt := func(key string) *int {
		if val, ok := h[key]; ok {
			if i, err := strconv.Atoi(val); err == nil {
				return &i
			}
		}
		return nil
	}

	parseUnixTimestamp := func(key string) *int64 {
		if val, ok := h[key]; ok {
			if ts, err := strconv.ParseInt(val, 10, 64); err == nil {
				ms := ts * 1000
				return &ms
			}
		}
		return nil
	}

	info := &reslientbridge.NormalizedRateLimitInfo{
		MaxRequests:       parseInt("x-ratelimit-limit"),
		RemainingRequests: parseInt("x-ratelimit-remaining"),
		ResetRequestsAt:   parseUnixTimestamp("x-ratelimit-reset"),
	}

	// retry-after is in seconds and only present if rate-limited (429).
	if val, ok := h["retry-after"]; ok {
		if sec, err := strconv.Atoi(val); err == nil {
			future := time.Now().UnixMilli() + int64(sec)*1000
			// If ResetRequestsAt is not set or this future is later, update it.
			if info.ResetRequestsAt == nil || future > *info.ResetRequestsAt {
				info.ResetRequestsAt = &future
			}
		}
	}

	return info, nil
}

func (d *DopplerAdapter) IsRateLimitError(resp *reslientbridge.NormalizedResponse) bool {
	return resp.StatusCode == 429
}
