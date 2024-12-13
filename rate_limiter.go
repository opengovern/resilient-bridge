package resilientbridge

import (
	"sync"
	"time"
)

type RateLimiter struct {
	mu             sync.Mutex
	providerLimits map[string]*NormalizedRateLimitInfo
}

// NewRateLimiter creates a new RateLimiter instance
func NewRateLimiter() *RateLimiter {
	return &RateLimiter{
		providerLimits: make(map[string]*NormalizedRateLimitInfo),
	}
}

// UpdateRateLimits updates the rate limit info for the given provider and call type.
func (r *RateLimiter) UpdateRateLimits(provider string, callType string, info *NormalizedRateLimitInfo, config *ProviderConfig) {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Combine provider and callType as a key to store separate rate limit states
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

// canProceed checks if the request can proceed given the current rate limit info
func (r *RateLimiter) canProceed(provider string, callType string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	key := provider + ":" + callType
	info, ok := r.providerLimits[key]
	if !ok || info == nil {
		return true
	}

	if info.RemainingRequests != nil && *info.RemainingRequests <= 0 {
		if info.ResetRequestsAt != nil && time.Now().UnixMilli() < *info.ResetRequestsAt {
			return false
		}
	}
	return true
}

// delayBeforeNextRequest calculates how long to wait before the next request if rate-limited
func (r *RateLimiter) delayBeforeNextRequest(provider string, callType string) time.Duration {
	r.mu.Lock()
	defer r.mu.Unlock()

	key := provider + ":" + callType
	info, ok := r.providerLimits[key]
	if !ok || info == nil {
		return 0
	}

	if info.ResetRequestsAt != nil && time.Now().UnixMilli() < *info.ResetRequestsAt {
		return time.Duration(*info.ResetRequestsAt-time.Now().UnixMilli()) * time.Millisecond
	}

	return 0
}

// GetRateLimitInfo returns a copy of the current NormalizedRateLimitInfo for the given provider.
// Returns nil if none.
func (r *RateLimiter) GetRateLimitInfo(provider string) *NormalizedRateLimitInfo {
	r.mu.Lock()
	defer r.mu.Unlock()

	// If you need separate info by callType, you may need to return a map or just one type.
	// For now, return nil or a combined info. Without callType param, we can't pick a specific one.
	// This can be extended if needed.
	return nil
}
