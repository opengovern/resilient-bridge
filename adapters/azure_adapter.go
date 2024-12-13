// azure_adapter.go
// ----------------
// This adapter integrates with Azure Resource Manager (ARM) and supports both old (hourly-based) and new (token-bucket) throttling.
//
// Approach:
//  - Determine scope (tenant vs subscription) and operation type (read/write/delete) from URL and method.
//  - Attempt to parse legacy headers for remaining requests.
//  - If we encounter a 429 and a short Retry-After (<=60 seconds), assume new token-bucket model.
//    Then set `MaxRequests` based on known bucket sizes for that scope/operation and `RemainingRequests` from the observed patterns.
//
// Without official distinct headers for the new token bucket model, we rely on heuristics.
// If no known headers found, return nil. SDK will handle retries on 429 with Retry-After.
//
// Operation types -> from doc:
//  Old model defaults (per hour):
//    subscription reads: max 12000/hour
//    subscription writes: max 1200/hour
//    subscription deletes: max 15000/hour
//    tenant reads: max 12000/hour
//    tenant writes: max 1200/hour
//
// New model (token bucket):
//   Per subscription or tenant:
//     reads: bucket 250, refill 25/sec
//     writes: bucket 200, refill 10/sec
//     deletes: bucket 200, refill 10/sec
//
// We detect the new model heuristically: if we got a 429 before and `Retry-After` <= 60s, we assume token bucket mode.
//
// NOTE: This logic is partly guesswork since docs don't give distinct header keys for the new model.

package adapters

import (
	"bytes"
	"io"
	"net/http"
	"strconv"
	"strings"

	resilientbridge "github.com/opengovern/resilient-bridge"
)

type AzureAdapter struct {
	APIToken          string
	lastScope         string // "subscription" or "tenant"
	lastOperationType string // "read", "write", "delete"

	// If we detect the new model after a 429 and short retry, we store a flag
	useTokenBucket bool
}

func NewAzureAdapter(apiToken string) *AzureAdapter {
	return &AzureAdapter{
		APIToken: apiToken,
	}
}

func (a *AzureAdapter) SetRateLimitDefaultsForType(requestType string, maxRequests int, windowSecs int64) {
	// No manual overrides; rely on headers and heuristics.
}

func (a *AzureAdapter) IdentifyRequestType(req *resilientbridge.NormalizedRequest) string {
	method := strings.ToUpper(req.Method)
	switch method {
	case "GET", "HEAD", "OPTIONS":
		return "read"
	case "POST", "PUT", "PATCH":
		return "write"
	case "DELETE":
		return "delete"
	default:
		return "write"
	}
}

func (a *AzureAdapter) ExecuteRequest(req *resilientbridge.NormalizedRequest) (*resilientbridge.NormalizedResponse, error) {
	// Determine scope
	scope := "tenant"
	if strings.Contains(strings.ToLower(req.Endpoint), "/subscriptions/") {
		scope = "subscription"
	}

	opType := a.IdentifyRequestType(req)

	a.lastScope = scope
	a.lastOperationType = opType

	client := &http.Client{}
	baseURL := "https://management.azure.com"
	fullURL := baseURL + req.Endpoint

	httpReq, err := http.NewRequest(req.Method, fullURL, bytes.NewReader(req.Body))
	if err != nil {
		return nil, err
	}

	if a.APIToken != "" && httpReq.Header.Get("Authorization") == "" {
		httpReq.Header.Set("Authorization", "Bearer "+a.APIToken)
	}
	if httpReq.Header.Get("Content-Type") == "" {
		httpReq.Header.Set("Content-Type", "application/json")
	}
	for k, v := range req.Headers {
		httpReq.Header.Set(k, v)
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

	// If we got a 429, check Retry-After. If <= 60, assume token bucket mode for future calls
	if resp.StatusCode == 429 {
		if val, ok := headers["retry-after"]; ok {
			if sec, err := strconv.Atoi(val); err == nil && sec <= 60 {
				a.useTokenBucket = true
			}
		}
	}

	return &resilientbridge.NormalizedResponse{
		StatusCode: resp.StatusCode,
		Headers:    headers,
		Data:       data,
	}, nil
}

func (a *AzureAdapter) ParseRateLimitInfo(resp *resilientbridge.NormalizedResponse) (*resilientbridge.NormalizedRateLimitInfo, error) {
	h := resp.Headers
	getIntHeader := func(name string) *int {
		if val, ok := h[name]; ok {
			if i, err := strconv.Atoi(val); err == nil {
				return &i
			}
		}
		return nil
	}

	scopePrefix := "subscription"
	if a.lastScope == "tenant" {
		scopePrefix = "tenant"
	}

	var headerName string
	var fallbackMaxRequests int
	// Old model defaults per hour (just for fallback):
	switch a.lastOperationType {
	case "read":
		headerName = "x-ms-ratelimit-remaining-" + scopePrefix + "-reads"
		if a.lastScope == "subscription" {
			fallbackMaxRequests = 12000
		} else {
			fallbackMaxRequests = 12000
		}
	case "write":
		headerName = "x-ms-ratelimit-remaining-" + scopePrefix + "-writes"
		if a.lastScope == "subscription" {
			fallbackMaxRequests = 1200
		} else {
			fallbackMaxRequests = 1200
		}
	case "delete":
		if a.lastScope == "subscription" {
			headerName = "x-ms-ratelimit-remaining-subscription-deletes"
			fallbackMaxRequests = 15000
		} else {
			// tenant-level deletes fallback to writes logic
			headerName = "x-ms-ratelimit-remaining-tenant-writes"
			fallbackMaxRequests = 1200
		}
	}

	remaining := getIntHeader(headerName)
	if remaining == nil {
		// Check resource headers
		resourceHeaders := []string{
			"x-ms-ratelimit-remaining-" + scopePrefix + "-resource-requests",
			"x-ms-ratelimit-remaining-" + scopePrefix + "-resource-entities-read",
		}
		for _, rh := range resourceHeaders {
			rem := getIntHeader(rh)
			if rem != nil {
				remaining = rem
				// Just assume read limit if resource based
				fallbackMaxRequests = 12000
				break
			}
		}
	}

	if remaining == nil && !a.useTokenBucket {
		// No info found and we are not in token bucket mode
		return nil, nil
	}

	// If token bucket mode detected:
	// Use the known bucket sizes from doc:
	// Scope: subscription or tenant
	// Operation: read/write/delete
	// Bucket sizes from doc:
	// subscription/tenant reads: 250
	// subscription/tenant writes: 200
	// subscription/tenant deletes: 200
	var maxRequests int
	if a.useTokenBucket {
		switch a.lastOperationType {
		case "read":
			maxRequests = 250
		case "write":
			maxRequests = 200
		case "delete":
			maxRequests = 200
		}

		if remaining == nil {
			// In token bucket mode, if no header found, just assume full
			remaining = resilientbridge.IntPtr(maxRequests)
		}
	} else {
		// old model
		maxRequests = fallbackMaxRequests
	}

	// No direct reset info from headers. In old model, reset is hourly. In new model, reset happens continuously.
	// For old model, we can guess resetAt = now+remaining*(windowSecondsFromRatio)
	// But no exact ratio. We'll leave ResetRequestsAt nil for simplicity.
	// The SDK relies on actual 429 for blocking anyway.

	info := &resilientbridge.NormalizedRateLimitInfo{
		MaxRequests:       &maxRequests,
		RemainingRequests: remaining,
		// ResetRequestsAt: nil
	}
	return info, nil
}

func (a *AzureAdapter) IsRateLimitError(resp *resilientbridge.NormalizedResponse) bool {
	return resp.StatusCode == 429
}
