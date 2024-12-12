package adapters

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

type OpenAIAdapter struct {
	APIKey string
}

func (o *OpenAIAdapter) ExecuteRequest(req *reslientbridge.NormalizedRequest) (*reslientbridge.NormalizedResponse, error) {
	client := &http.Client{}

	fullURL := "https://api.openai.com" + req.Endpoint

	httpReq, err := http.NewRequest(req.Method, fullURL, bytes.NewReader(req.Body))
	if err != nil {
		return nil, err
	}

	// Set request headers
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

func (o *OpenAIAdapter) ParseRateLimitInfo(resp *reslientbridge.NormalizedResponse) (*reslientbridge.NormalizedRateLimitInfo, error) {
	h := resp.Headers
	parseInt := func(key string) *int {
		if val, ok := h[key]; ok {
			if i, err := strconv.Atoi(val); err == nil {
				return &i
			}
		}
		return nil
	}

	parseDuration := func(d string) *int64 {
		if d == "" {
			return nil
		}
		ms := parseTimeStr(d)
		if ms > 0 {
			t := time.Now().UnixMilli() + ms
			return &t
		}
		return nil
	}

	info := &reslientbridge.NormalizedRateLimitInfo{
		MaxRequests:       parseInt("x-ratelimit-limit-requests"),
		RemainingRequests: parseInt("x-ratelimit-remaining-requests"),
		MaxTokens:         parseInt("x-ratelimit-limit-tokens"),
		RemainingTokens:   parseInt("x-ratelimit-remaining-tokens"),
	}

	if val, ok := h["x-ratelimit-reset-requests"]; ok {
		info.ResetRequestsAt = parseDuration(val)
	}
	if val, ok := h["x-ratelimit-reset-tokens"]; ok {
		info.ResetTokensAt = parseDuration(val)
	}

	return info, nil
}

func (o *OpenAIAdapter) IsRateLimitError(resp *reslientbridge.NormalizedResponse) bool {
	return resp.StatusCode == 429
}

// parseTimeStr converts strings like "1s", "6m0s", "30s" into milliseconds.
func parseTimeStr(s string) int64 {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}

	// Try "XmYs" format
	var minutes, seconds int
	n, err := fmt.Sscanf(s, "%dm%ds", &minutes, &seconds)
	if err == nil && n == 2 {
		return int64(minutes)*60_000 + int64(seconds)*1_000
	}

	// If no 'm', maybe just seconds like "30s"
	if strings.HasSuffix(s, "s") && !strings.Contains(s, "m") {
		val := strings.TrimSuffix(s, "s")
		if sec, err := strconv.Atoi(val); err == nil {
			return int64(sec) * 1000
		}
	}

	return 0
}
