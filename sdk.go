package resilientbridge

import (
	"fmt"
	"sync"
)

type ResilientBridge struct {
	mu          sync.Mutex
	providers   map[string]ProviderAdapter
	configs     map[string]*ProviderConfig
	rateLimiter *RateLimiter
	executor    *RequestExecutor
}

func NewResilientBridge() *ResilientBridge {
	sdk := &ResilientBridge{
		providers:   make(map[string]ProviderAdapter),
		configs:     make(map[string]*ProviderConfig),
		rateLimiter: NewRateLimiter(),
	}
	sdk.executor = NewRequestExecutor(sdk)
	return sdk
}

func (sdk *ResilientBridge) RegisterProvider(name string, adapter ProviderAdapter, config *ProviderConfig) {
	sdk.mu.Lock()
	defer sdk.mu.Unlock()
	sdk.providers[name] = adapter
	sdk.configs[name] = config

	// Apply REST limits
	var restMaxRequests int
	var restWindowSecs int64
	if config.MaxRequestsOverride != nil {
		restMaxRequests = *config.MaxRequestsOverride
	}
	if config.WindowSecsOverride != nil {
		restWindowSecs = *config.WindowSecsOverride
	}
	adapter.SetRateLimitDefaultsForType("rest", restMaxRequests, restWindowSecs)

	// Apply GraphQL limits if set
	var gqlMaxRequests int
	var gqlWindowSecs int64
	if config.GraphQLMaxRequestsOverride != nil {
		gqlMaxRequests = *config.GraphQLMaxRequestsOverride
	}
	if config.GraphQLWindowSecsOverride != nil {
		gqlWindowSecs = *config.GraphQLWindowSecsOverride
	}
	adapter.SetRateLimitDefaultsForType("graphql", gqlMaxRequests, gqlWindowSecs)
}

func (sdk *ResilientBridge) Request(providerName string, req *NormalizedRequest) (*NormalizedResponse, error) {
	sdk.mu.Lock()
	adapter, ok := sdk.providers[providerName]
	sdk.mu.Unlock()
	if !ok {
		return nil, fmt.Errorf("provider %q not registered", providerName)
	}

	return sdk.executor.ExecuteWithRetry(providerName, func() (*NormalizedResponse, error) {
		return adapter.ExecuteRequest(req)
	}, adapter)
}

func (sdk *ResilientBridge) getProviderConfig(providerName string) *ProviderConfig {
	sdk.mu.Lock()
	defer sdk.mu.Unlock()

	config, ok := sdk.configs[providerName]
	if !ok || config == nil {
		// Return a default config if none is set.
		return &ProviderConfig{
			UseProviderLimits: true,
			MaxRetries:        3,
			BaseBackoff:       0,
		}
	}

	return config
}

func (sdk *ResilientBridge) GetRateLimitInfo(providerName string) *NormalizedRateLimitInfo {
	return sdk.rateLimiter.GetRateLimitInfo(providerName)
}
