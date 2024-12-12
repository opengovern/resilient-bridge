package resilientbridge

import (
	"fmt"
	"time"
)

// RequestExecutor handles retry logic, backoff, and consulting RateLimiter.
type RequestExecutor struct {
	sdk *ReslientBridge
}

func NewRequestExecutor(sdk *ReslientBridge) *RequestExecutor {
	return &RequestExecutor{sdk: sdk}
}

func (re *RequestExecutor) ExecuteWithRetry(providerName string, operation func() (*NormalizedResponse, error), adapter ProviderAdapter) (*NormalizedResponse, error) {
	config := re.sdk.getProviderConfig(providerName)
	maxRetries := config.MaxRetries
	baseBackoff := config.BaseBackoff
	if baseBackoff == 0 {
		baseBackoff = time.Second
	}

	attempts := 0
	for {
		if !re.sdk.rateLimiter.canProceed(providerName) {
			delay := re.sdk.rateLimiter.delayBeforeNextRequest(providerName)
			if delay > 0 {
				fmt.Printf("[DEBUG] Provider %s: Must wait %v before next request due to rate limit.\n", providerName, delay)
				time.Sleep(delay)
			}
		}

		fmt.Printf("[DEBUG] Provider %s: Sending request (attempt %d)...\n", providerName, attempts+1)
		resp, err := operation()
		if err != nil {
			if attempts < maxRetries {
				wait := re.calculateBackoff(baseBackoff, attempts)
				fmt.Printf("[DEBUG] Provider %s: Operation error: %v. Retrying in %v (attempt %d/%d)...\n", providerName, err, wait, attempts+1, maxRetries)
				time.Sleep(wait)
				attempts++
				continue
			}
			fmt.Printf("[DEBUG] Provider %s: Max retries reached after error: %v\n", providerName, err)
			return nil, err
		}

		// Update rate limits if info available
		if rateInfo, parseErr := adapter.ParseRateLimitInfo(resp); parseErr == nil && rateInfo != nil {
			re.sdk.rateLimiter.UpdateRateLimits(providerName, rateInfo, config)
		}

		if adapter.IsRateLimitError(resp) {
			// 429 encountered
			if attempts < maxRetries {
				wait := re.waitForRateLimit(providerName, attempts, maxRetries, baseBackoff)
				fmt.Printf("[DEBUG] Provider %s: 429 rate limit error. Backing off for %v before retry...\n", providerName, wait)
				time.Sleep(wait)
				attempts++
				continue
			}
			fmt.Printf("[DEBUG] Provider %s: Rate limit (429) encountered and max retries reached. Giving up.\n", providerName)
			return nil, fmt.Errorf("rate limit exceeded and max retries reached")
		}

		if resp.StatusCode >= 500 && attempts < maxRetries {
			wait := re.calculateBackoff(baseBackoff, attempts)
			fmt.Printf("[DEBUG] Provider %s: Server error %d. Retrying in %v (attempt %d/%d)...\n", providerName, resp.StatusCode, wait, attempts+1, maxRetries)
			time.Sleep(wait)
			attempts++
			continue
		} else if resp.StatusCode >= 400 {
			fmt.Printf("[DEBUG] Provider %s: Client error %d encountered. Not retrying.\n", providerName, resp.StatusCode)
			return resp, fmt.Errorf("client error: %d", resp.StatusCode)
		}

		if attempts > 0 {
			fmt.Printf("[DEBUG] Provider %s: Request succeeded after %d attempts.\n", providerName, attempts+1)
		} else {
			fmt.Printf("[DEBUG] Provider %s: Request succeeded on first attempt.\n", providerName)
		}
		return resp, nil
	}
}

func (re *RequestExecutor) waitForRateLimit(providerName string, attempts, maxRetries int, baseBackoff time.Duration) time.Duration {
	if !re.sdk.rateLimiter.canProceed(providerName) {
		delay := re.sdk.rateLimiter.delayBeforeNextRequest(providerName)
		if delay > 0 {
			fmt.Printf("[DEBUG] Provider %s: Hit rate limit on attempt %d/%d. Waiting %v (from rate limit info) before retry...\n", providerName, attempts+1, maxRetries, delay)
			return delay
		}
	}
	// If no delay from RateLimiter, fallback to exponential backoff
	return re.calculateBackoff(baseBackoff, attempts)
}

func (re *RequestExecutor) calculateBackoff(base time.Duration, attempt int) time.Duration {
	backoff := base * (1 << attempt) // base * 2^attempt
	if backoff > 30*time.Second {
		backoff = 30 * time.Second
	}
	fmt.Printf("[DEBUG] Calculated exponential backoff: attempt %d, backoff=%v\n", attempt+1, backoff)
	return backoff
}
