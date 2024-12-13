// semgrep_adapter.go
package adapters

import (
	"bytes"
	"io"
	"net/http"
	"strings"
	"sync"

	resilientbridge "github.com/opengovern/resilient-bridge"
)

type SemgrepAdapter struct {
	APIToken string

	mu sync.Mutex
}

func NewSemgrepAdapter(apiToken string) *SemgrepAdapter {
	return &SemgrepAdapter{
		APIToken: apiToken,
	}
}

func (s *SemgrepAdapter) SetRateLimitDefaultsForType(requestType string, maxRequests int, windowSecs int64) {
	// Semgrep rate limits are not specified, ignoring overrides.
}

func (s *SemgrepAdapter) ExecuteRequest(req *resilientbridge.NormalizedRequest) (*resilientbridge.NormalizedResponse, error) {
	client := &http.Client{}
	baseURL := "https://semgrep.dev/api/v1"
	fullURL := baseURL + req.Endpoint

	httpReq, err := http.NewRequest(req.Method, fullURL, bytes.NewReader(req.Body))
	if err != nil {
		return nil, err
	}

	for k, v := range req.Headers {
		httpReq.Header.Set(k, v)
	}
	if httpReq.Header.Get("Authorization") == "" && s.APIToken != "" {
		httpReq.Header.Set("Authorization", "Bearer "+s.APIToken)
	}
	if httpReq.Header.Get("Content-Type") == "" {
		httpReq.Header.Set("Content-Type", "application/json")
	}

	// Simple request execution
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

	return &resilientbridge.NormalizedResponse{
		StatusCode: resp.StatusCode,
		Headers:    headers,
		Data:       data,
	}, nil
}

func (s *SemgrepAdapter) ParseRateLimitInfo(resp *resilientbridge.NormalizedResponse) (*resilientbridge.NormalizedRateLimitInfo, error) {
	// No documented rate limit headers
	return nil, nil
}

func (s *SemgrepAdapter) IsRateLimitError(resp *resilientbridge.NormalizedResponse) bool {
	return resp.StatusCode == 429
}
