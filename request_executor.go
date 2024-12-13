// request_executor.go
// -------------------
// This file defines the RequestExecutor type responsible for executing requests
// with retry logic, exponential backoff, and handling rate limit (429) responses.
// It integrates with the RateLimiter and ProviderAdapter interfaces to determine
// how to retry and when to respect provider-specific rate limits. It also checks
// for Retry-After headers and applies jitter to wait durations.
//
// The ExecuteWithRetry method is the core entry point, called by the SDK to issue
// a request repeatedly until success or until the configured max retries are reached.
package resilientbridge

import (
	"fmt"
	"math/rand"
	"strconv"
	"time"
)

type RequestExecutor struct {
	sdk *ResilientBridge
}

func NewRequestExecutor(sdk *ResilientBridge) *RequestExecutor {
	return &RequestExecutor{sdk: sdk}
}

func (re *RequestExecutor) ExecuteWithRetry(providerName string, callType string, operation func() (*NormalizedResponse, error), adapter ProviderAdapter) (*NormalizedResponse, error) {
	config := re.sdk.getProviderConfig(providerName)
	maxRetries := config.MaxRetries
	baseBackoff := config.BaseBackoff
	if baseBackoff == 0 {
		baseBackoff = time.Second
	}

	attempts := 0
	for {
		// Preemptively wait if the SDK knows we must delay due to rate limit info
		if !re.sdk.rateLimiter.canProceed(providerName, callType) {
			delay := re.sdk.rateLimiter.delayBeforeNextRequest(providerName, callType)
			if delay > 0 && re.sdk.Debug {
				fmt.Printf("[DEBUG] Provider %s (callType=%s): Must wait %v due to preemptive rate limit.\n", providerName, callType, delay)
			}
			time.Sleep(delay)
		}

		re.sdk.debugf("Provider %s (callType=%s): Sending request (attempt %d)...\n", providerName, callType, attempts+1)
		resp, err := operation()
		if err != nil {
			// Non-HTTP/network error
			if attempts < maxRetries {
				wait := re.calculateBackoffWithJitter(baseBackoff, attempts)
				re.sdk.debugf("Provider %s (callType=%s): Operation error: %v. Retrying in %v (attempt %d/%d)...\n", providerName, callType, err, wait, attempts+1, maxRetries)
				time.Sleep(wait)
				attempts++
				continue
			}
			re.sdk.debugf("Provider %s (callType=%s): Max retries reached after error: %v\n", providerName, callType, err)
			return nil, err
		}

		// Parse and update rate limit info if provided
		if rateInfo, parseErr := adapter.ParseRateLimitInfo(resp); parseErr == nil && rateInfo != nil {
			re.sdk.rateLimiter.UpdateRateLimits(providerName, callType, rateInfo, config)
		}

		// Handle rate limit (429) responses
		if adapter.IsRateLimitError(resp) {
			if attempts < maxRetries {
				retryAfter := re.parseRetryAfter(resp)
				if retryAfter > 0 {
					jitter := re.calculateJitter(retryAfter, 0.1)
					totalWait := retryAfter + jitter
					re.sdk.debugf("Provider %s (callType=%s): 429 rate limit, Retry-After present: waiting %v (+%v jitter) before retry (attempt %d/%d).\n", providerName, callType, retryAfter, jitter, attempts+1, maxRetries)
					time.Sleep(totalWait)
				} else {
					wait := re.calculateBackoffWithJitter(baseBackoff, attempts)
					re.sdk.debugf("Provider %s (callType=%s): 429 rate limit, no Retry-After header. Backing off %v before retry (attempt %d/%d)...\n", providerName, callType, wait, attempts+1, maxRetries)
					time.Sleep(wait)
				}
				attempts++
				continue
			}
			re.sdk.debugf("Provider %s (callType=%s): Actual 429 encountered and max retries reached. Giving up.\n", providerName, callType)
			return resp, fmt.Errorf("rate limit exceeded and max retries reached")
		}

		// Handle server errors (5xx)
		if resp.StatusCode >= 500 && attempts < maxRetries {
			wait := re.calculateBackoffWithJitter(baseBackoff, attempts)
			re.sdk.debugf("Provider %s (callType=%s): Server error %d. Retrying in %v (attempt %d/%d)...\n", providerName, callType, resp.StatusCode, wait, attempts+1, maxRetries)
			time.Sleep(wait)
			attempts++
			continue
		} else if resp.StatusCode >= 400 {
			// Client error (4xx), do not retry
			re.sdk.debugf("Provider %s (callType=%s): Client error %d encountered. Not retrying.\n", providerName, callType, resp.StatusCode)
			return resp, fmt.Errorf("client error: %d", resp.StatusCode)
		}

		// Success
		if attempts > 0 && re.sdk.Debug {
			fmt.Printf("[DEBUG] Provider %s (callType=%s): Request succeeded after %d attempts.\n", providerName, callType, attempts+1)
		} else if re.sdk.Debug {
			fmt.Printf("[DEBUG] Provider %s (callType=%s): Request succeeded on first attempt.\n", providerName, callType)
		}
		return resp, nil
	}
}

func (re *RequestExecutor) calculateBackoffWithJitter(base time.Duration, attempt int) time.Duration {
	backoff := base * (1 << attempt)
	if backoff > 30*time.Second {
		backoff = 30 * time.Second
	}
	jitterFactor := 0.5
	jitter := time.Duration(rand.Float64() * float64(backoff) * jitterFactor)
	return backoff + jitter
}

func (re *RequestExecutor) parseRetryAfter(resp *NormalizedResponse) time.Duration {
	if val, ok := resp.Headers["retry-after"]; ok {
		if seconds, err := strconv.Atoi(val); err == nil {
			return time.Duration(seconds) * time.Second
		}
	}
	return 0
}

func (re *RequestExecutor) calculateJitter(base time.Duration, fraction float64) time.Duration {
	jitter := time.Duration(rand.Float64() * float64(base) * fraction)
	return jitter
}
