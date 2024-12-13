// rate_limiter.go
// ----------------
// This file defines the RateLimiter type, which stores and manages rate limit information
// for each provider and call type. It integrates with the adapter-provided NormalizedRateLimitInfo
// to determine if a request can proceed immediately or if it must wait until a reset time.
//
// Responsibilities:
// - Storing rate limit info keyed by "provider:callType".
// - Checking if requests can proceed based on RemainingRequests and ResetRequestsAt.
// - Calculating delay durations before the next allowed request if the rate limit is exceeded.
// - Integrating with ProviderConfig overrides if UseProviderLimits is false.
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

// UpdateRateLimits updates the stored rate limit info for a given provider and call type,
// applying overrides from the ProviderConfig if UseProviderLimits is false.
func (r *RateLimiter) UpdateRateLimits(provider string, callType string, info *NormalizedRateLimitInfo, config *ProviderConfig) {
	r.mu.Lock()
	defer r.mu.Unlock()

	key := provider + ":" + callType

	// Apply user overrides if not using provider limits
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

// canProceed checks if a request can proceed immediately for a given provider and callType.
// It returns false if the rate limit has been hit and the reset time hasn't passed yet.
func (r *RateLimiter) canProceed(provider string, callType string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	key := provider + ":" + callType
	info, ok := r.providerLimits[key]
	if !ok || info == nil {
		// No known limits, assume proceed
		return true
	}

	if info.RemainingRequests != nil && *info.RemainingRequests <= 0 {
		// Out of requests, check if reset time is still in the future
		if info.ResetRequestsAt != nil && time.Now().UnixMilli() < *info.ResetRequestsAt {
			return false
		}
	}
	return true
}

// delayBeforeNextRequest calculates how long we must wait before making another request
// if the rate limit is currently exceeded. It returns a duration to sleep, if any.
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

// GetRateLimitInfo returns a copy of the rate limit info for a given provider's "rest" call type.
// For simplicity, if multiple callTypes exist, it returns only the "rest" type info.
func (r *RateLimiter) GetRateLimitInfo(provider string) *NormalizedRateLimitInfo {
	r.mu.Lock()
	defer r.mu.Unlock()

	key := provider + ":rest"
	if info, ok := r.providerLimits[key]; ok {
		copyInfo := *info
		return &copyInfo
	}
	return nil
}
