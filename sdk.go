package resilientbridge

import (
	"fmt"
	"sync"
)

// ReslientBridge is the main entry point for using the SDK.
type ReslientBridge struct {
	mu          sync.Mutex
	providers   map[string]ProviderAdapter
	configs     map[string]*ProviderConfig
	rateLimiter *RateLimiter
	executor    *RequestExecutor
}

// NewReslientBridge creates a new instance of the SDK.
func NewReslientBridge() *ReslientBridge {
	sdk := &ReslientBridge{
		providers:   make(map[string]ProviderAdapter),
		configs:     make(map[string]*ProviderConfig),
		rateLimiter: NewRateLimiter(),
	}
	sdk.executor = NewRequestExecutor(sdk)
	return sdk
}

// RegisterProvider attaches a provider adapter and its config to the SDK.
func (sdk *ReslientBridge) RegisterProvider(name string, adapter ProviderAdapter, config *ProviderConfig) {
	sdk.mu.Lock()
	defer sdk.mu.Unlock()
	sdk.providers[name] = adapter
	sdk.configs[name] = config
}

// Request sends a request to a registered provider.
func (sdk *ReslientBridge) Request(providerName string, req *NormalizedRequest) (*NormalizedResponse, error) {
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

// getProviderConfig retrieves the config for a provider, returning a default if nil or not found.
func (sdk *ReslientBridge) getProviderConfig(providerName string) *ProviderConfig {
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

// GetRateLimitInfo returns the current NormalizedRateLimitInfo for the given provider.
// Returns nil if no info is available.
func (sdk *ReslientBridge) GetRateLimitInfo(providerName string) *NormalizedRateLimitInfo {
	return sdk.rateLimiter.GetRateLimitInfo(providerName)
}
