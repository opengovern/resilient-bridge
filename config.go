package resilientbridge

import "time"

// ProviderConfig allows per-provider customization of rate limits, retries, etc.
type ProviderConfig struct {
	UseProviderLimits   bool
	MaxRequestsOverride *int   // override default max requests if set
	WindowSecsOverride  *int64 // override default window seconds if set
	MaxTokensOverride   *int
	MaxRetries          int
	BaseBackoff         time.Duration
}
