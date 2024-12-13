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

func (r *RateLimiter) UpdateRateLimits(provider string, info *NormalizedRateLimitInfo, config *ProviderConfig) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.providerLimits[provider] = info
}

func (r *RateLimiter) canProceed(provider string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	info, ok := r.providerLimits[provider]
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

func (r *RateLimiter) delayBeforeNextRequest(provider string) time.Duration {
	r.mu.Lock()
	defer r.mu.Unlock()

	info, ok := r.providerLimits[provider]
	if !ok || info == nil {
		return 0
	}
	if info.ResetRequestsAt != nil && time.Now().UnixMilli() < *info.ResetRequestsAt {
		return time.Duration(*info.ResetRequestsAt-time.Now().UnixMilli()) * time.Millisecond
	}
	return 0
}

func (r *RateLimiter) GetRateLimitInfo(provider string) *NormalizedRateLimitInfo {
	r.mu.Lock()
	defer r.mu.Unlock()

	if info, ok := r.providerLimits[provider]; ok && info != nil {
		copy := *info
		return &copy
	}
	return nil
}
