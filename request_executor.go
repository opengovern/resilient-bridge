// request_executor.go
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
		// If we can't proceed due to known rate limit info, wait preemptively
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
			// Network or other non-HTTP error
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

		// Parse rate limit info if available
		if rateInfo, parseErr := adapter.ParseRateLimitInfo(resp); parseErr == nil && rateInfo != nil {
			re.sdk.rateLimiter.UpdateRateLimits(providerName, callType, rateInfo, config)
		}

		if adapter.IsRateLimitError(resp) {
			// Actual 429 from provider
			if attempts < maxRetries {
				// Check if Retry-After header is present
				retryAfter := re.parseRetryAfter(resp)
				if retryAfter > 0 {
					// Add jitter to the retry-after duration
					jitter := re.calculateJitter(retryAfter, 0.1) // 10% jitter
					totalWait := retryAfter + jitter
					re.sdk.debugf("Provider %s (callType=%s): 429 rate limit, Retry-After present: waiting %v (+%v jitter) before retry (attempt %d/%d).\n", providerName, callType, retryAfter, jitter, attempts+1, maxRetries)
					time.Sleep(totalWait)
				} else {
					// No Retry-After present, fallback to exponential backoff
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

		if resp.StatusCode >= 500 && attempts < maxRetries {
			// Server error, retry with backoff
			wait := re.calculateBackoffWithJitter(baseBackoff, attempts)
			re.sdk.debugf("Provider %s (callType=%s): Server error %d. Retrying in %v (attempt %d/%d)...\n", providerName, callType, resp.StatusCode, wait, attempts+1, maxRetries)
			time.Sleep(wait)
			attempts++
			continue
		} else if resp.StatusCode >= 400 {
			// Client error, no retry
			re.sdk.debugf("Provider %s (callType=%s): Client error %d encountered. Not retrying.\n", providerName, callType, resp.StatusCode)
			return resp, fmt.Errorf("client error: %d", resp.StatusCode)
		}

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
		// If it's not an integer, ignore and return 0
	}
	return 0
}

// calculateJitter calculates an additional jitter duration as a fraction of a base duration.
func (re *RequestExecutor) calculateJitter(base time.Duration, fraction float64) time.Duration {
	jitter := time.Duration(rand.Float64() * float64(base) * fraction)
	return jitter
}
