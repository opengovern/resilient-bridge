package resilientbridge

import "time"

// ProviderConfig allows per-provider customization of rate limits, retries, etc.
type ProviderConfig struct {
	UseProviderLimits   bool
	MaxRequestsOverride *int   // override default max requests for REST if set
	WindowSecsOverride  *int64 // override default window seconds for REST if set

	GraphQLMaxRequestsOverride *int   // override default max requests for GraphQL if set
	GraphQLWindowSecsOverride  *int64 // override default window seconds for GraphQL if set

	MaxTokensOverride *int
	MaxRetries        int
	BaseBackoff       time.Duration
}
