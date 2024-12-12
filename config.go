package resilientbridge

import "time"

// ProviderConfig allows per-provider customization of rate limits, retries, etc.
type ProviderConfig struct {
	UseProviderLimits   bool
	MaxRequestsOverride *int
	MaxTokensOverride   *int
	MaxRetries          int
	BaseBackoff         time.Duration
}
