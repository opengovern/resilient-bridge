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

	// If we have overrides, call SetRateLimitDefaults on the adapter
	var maxRequests int
	var windowSecs int64
	if config.MaxRequestsOverride != nil {
		maxRequests = *config.MaxRequestsOverride
	}
	if config.WindowSecsOverride != nil {
		windowSecs = *config.WindowSecsOverride
	}

	// Even if zero, adapter should fallback to its internal defaults.
	adapter.SetRateLimitDefaults(maxRequests, windowSecs)
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
