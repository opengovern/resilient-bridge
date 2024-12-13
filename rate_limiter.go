// rate_limiter.go
package resilientbridge

import (
	"sync"
	"time"
)

type RateLimiter struct {
	mu             sync.Mutex
	providerLimits map[string]*NormalizedRateLimitInfo
}

func NewRateLimiter() *RateLimiter {
	return &RateLimiter{
		providerLimits: make(map[string]*NormalizedRateLimitInfo),
	}
}

// UpdateRateLimits updates the rate limit info for the given provider and callType.
func (r *RateLimiter) UpdateRateLimits(provider string, callType string, info *NormalizedRateLimitInfo, config *ProviderConfig) {
	r.mu.Lock()
	defer r.mu.Unlock()

	key := provider + ":" + callType

	if info != nil && !config.UseProviderLimits {
		if config.MaxRequestsOverride != nil {
			info.MaxRequests = config.MaxRequestsOverride
			if info.RemainingRequests == nil || *info.RemainingRequests > *info.MaxRequests {
				newRem := *info.MaxRequests
				info.RemainingRequests = &newRem
			}
		}
		if config.MaxTokensOverride != nil {
			info.MaxTokens = config.MaxTokensOverride
			if info.RemainingTokens == nil || *info.RemainingTokens > *info.MaxTokens {
				newRem := *info.MaxTokens
				info.RemainingTokens = &newRem
			}
		}
	}

	r.providerLimits[key] = info
}

func (r *RateLimiter) canProceed(provider string, callType string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	key := provider + ":" + callType
	info, ok := r.providerLimits[key]
	if !ok || info == nil {
		// No known limits, just proceed
		return true
	}

	if info.RemainingRequests != nil && *info.RemainingRequests <= 0 {
		// No requests left. Check if there's a reset time.
		if info.ResetRequestsAt != nil && time.Now().UnixMilli() < *info.ResetRequestsAt {
			return false
		}
	}
	return true
}

func (r *RateLimiter) delayBeforeNextRequest(provider string, callType string) time.Duration {
	r.mu.Lock()
	defer r.mu.Unlock()

	key := provider + ":" + callType
	info, ok := r.providerLimits[key]
	if !ok || info == nil {
		return 0
	}

	if info.RemainingRequests != nil && *info.RemainingRequests <= 0 && info.ResetRequestsAt != nil {
		nowMs := time.Now().UnixMilli()
		if nowMs < *info.ResetRequestsAt {
			delayMs := *info.ResetRequestsAt - nowMs
			return time.Duration(delayMs) * time.Millisecond
		}
	}

	return 0
}

func (r *RateLimiter) GetRateLimitInfo(provider string) *NormalizedRateLimitInfo {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Without specifying callType, we return nil or just one type.
	// For simplicity, return nil here or pick a default callType "rest".
	// If multiple callTypes exist, you might want to combine or pick the main one.
	// For now, assume "rest" is the main type.
	key := provider + ":rest"
	if info, ok := r.providerLimits[key]; ok {
		// Return a copy of info
		copyInfo := *info
		return &copyInfo
	}
	return nil
}
