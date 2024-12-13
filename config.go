// config.go
// ----------
// This file defines the ProviderConfig structure, which allows per-provider customization
// of behavior such as using provider-defined limits or overrides, setting max retries,
// and base backoff duration.
//
// Fields can override default rate limits (MaxRequestsOverride, WindowSecsOverride),
// and also handle GraphQL-specific overrides if needed.
package resilientbridge

import "time"

// ProviderConfig allows per-provider customization of rate limits, retries, and other settings.
type ProviderConfig struct {
	UseProviderLimits   bool
	MaxRequestsOverride *int   // Override default max requests for REST if set
	WindowSecsOverride  *int64 // Override default window for REST if set

	GraphQLMaxRequestsOverride *int   // Override max requests for GraphQL if set
	GraphQLWindowSecsOverride  *int64 // Override window for GraphQL if set

	MaxTokensOverride *int          // If token-based rate limits apply
	MaxRetries        int           // Max number of retries on failure
	BaseBackoff       time.Duration // Initial backoff duration for exponential backoff
}
