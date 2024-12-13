package resilientbridge

import (
	"fmt"
	"math/rand"
	"time"
)

type RequestExecutor struct {
	sdk *ResilientBridge
}

func NewRequestExecutor(sdk *ResilientBridge) *RequestExecutor {
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
				re.debugf("Provider %s: Must wait %v before next request due to rate limit.\n", providerName, delay)
				time.Sleep(delay)
			}
		}

		re.debugf("Provider %s: Sending request (attempt %d)...\n", providerName, attempts+1)
		resp, err := operation()
		if err != nil {
			if attempts < maxRetries {
				wait := re.calculateBackoffWithJitter(baseBackoff, attempts)
				re.debugf("Provider %s: Operation error: %v. Retrying in %v (attempt %d/%d)...\n", providerName, err, wait, attempts+1, maxRetries)
				time.Sleep(wait)
				attempts++
				continue
			}
			re.debugf("Provider %s: Max retries reached after error: %v\n", providerName, err)
			return nil, err
		}

		if rateInfo, parseErr := adapter.ParseRateLimitInfo(resp); parseErr == nil && rateInfo != nil {
			re.sdk.rateLimiter.UpdateRateLimits(providerName, rateInfo, config)
		}

		if adapter.IsRateLimitError(resp) {
			if attempts < maxRetries {
				wait := re.calculateBackoffWithJitter(baseBackoff, attempts)
				re.debugf("Provider %s: 429 rate limit error. Backing off for %v before retry...\n", providerName, wait)
				time.Sleep(wait)
				attempts++
				continue
			}
			re.debugf("Provider %s: Rate limit (429) encountered and max retries reached. Giving up.\n", providerName)
			return resp, fmt.Errorf("rate limit exceeded and max retries reached")
		}

		if resp.StatusCode >= 500 && attempts < maxRetries {
			wait := re.calculateBackoffWithJitter(baseBackoff, attempts)
			re.debugf("Provider %s: Server error %d. Retrying in %v (attempt %d/%d)...\n", providerName, resp.StatusCode, wait, attempts+1, maxRetries)
			time.Sleep(wait)
			attempts++
			continue
		} else if resp.StatusCode >= 400 {
			re.debugf("Provider %s: Client error %d encountered. Not retrying.\n", providerName, resp.StatusCode)
			return resp, fmt.Errorf("client error: %d", resp.StatusCode)
		}

		if attempts > 0 {
			re.debugf("Provider %s: Request succeeded after %d attempts.\n", providerName, attempts+1)
		} else {
			re.debugf("Provider %s: Request succeeded on first attempt.\n", providerName)
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

// debugf prints debug messages only if SDK's debug mode is enabled
func (re *RequestExecutor) debugf(format string, args ...interface{}) {
	if re.sdk.Debug {
		fmt.Printf("[DEBUG] "+format, args...)
	}
}
