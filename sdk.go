// sdk.go
// ------
// The sdk.go file contains the core ResilientBridge struct and its methods.
// This is the main entry point of the SDK for users.
//
// Key functionalities include:
// - Initializing the SDK with NewResilientBridge()
// - Registering providers with RegisterProvider()
// - Making requests via sdk.Request()
// - Managing and retrieving provider configurations and rate limit info
//
// The ResilientBridge relies on a RateLimiter and a RequestExecutor to handle
// rate limiting and retries, ensuring consistent behavior across all providers.
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

	Debug bool // If true, print debug info
}

func NewResilientBridge() *ResilientBridge {
	sdk := &ResilientBridge{
		providers:   make(map[string]ProviderAdapter),
		configs:     make(map[string]*ProviderConfig),
		rateLimiter: NewRateLimiter(),
		Debug:       false,
	}
	sdk.executor = NewRequestExecutor(sdk)
	return sdk
}

// SetDebug enables or disables debug logging for the SDK.
func (sdk *ResilientBridge) SetDebug(enabled bool) {
	sdk.mu.Lock()
	defer sdk.mu.Unlock()
	sdk.Debug = enabled
}

// RegisterProvider associates a ProviderAdapter with a provider name and configuration.
// It also sets default rate limits for the "rest" request type (and can be extended for others).
func (sdk *ResilientBridge) RegisterProvider(name string, adapter ProviderAdapter, config *ProviderConfig) {
	sdk.mu.Lock()
	defer sdk.mu.Unlock()
	sdk.providers[name] = adapter
	sdk.configs[name] = config

	// Apply REST defaults
	var restMaxRequests int
	var restWindowSecs int64
	if config.MaxRequestsOverride != nil {
		restMaxRequests = *config.MaxRequestsOverride
	}
	if config.WindowSecsOverride != nil {
		restWindowSecs = *config.WindowSecsOverride
	}
	adapter.SetRateLimitDefaultsForType("rest", restMaxRequests, restWindowSecs)

	sdk.debugf("Registered provider %q with config: %+v\n", name, config)
}

// Request sends a NormalizedRequest to the specified provider and returns a NormalizedResponse.
// It uses the RequestExecutor to handle retries, rate limits, and backoff.
func (sdk *ResilientBridge) Request(providerName string, req *NormalizedRequest) (*NormalizedResponse, error) {
	sdk.mu.Lock()
	adapter, ok := sdk.providers[providerName]
	sdk.mu.Unlock()
	if !ok {
		return nil, fmt.Errorf("provider %q not registered", providerName)
	}

	callType := adapter.IdentifyRequestType(req)
	sdk.debugf("Requesting provider %s (callType=%s) at endpoint %s\n", providerName, callType, req.Endpoint)
	return sdk.executor.ExecuteWithRetry(providerName, callType, func() (*NormalizedResponse, error) {
		return adapter.ExecuteRequest(req)
	}, adapter)
}

// getProviderConfig retrieves the ProviderConfig for a given provider, or a default if not found.
func (sdk *ResilientBridge) getProviderConfig(providerName string) *ProviderConfig {
	sdk.mu.Lock()
	defer sdk.mu.Unlock()

	config, ok := sdk.configs[providerName]
	if !ok || config == nil {
		// Default config if none provided
		return &ProviderConfig{
			UseProviderLimits: true,
			MaxRetries:        3,
			BaseBackoff:       0,
		}
	}

	return config
}

// GetRateLimitInfo returns the current known rate limit info for a given provider.
func (sdk *ResilientBridge) GetRateLimitInfo(providerName string) *NormalizedRateLimitInfo {
	return sdk.rateLimiter.GetRateLimitInfo(providerName)
}

// debugf prints debug messages if Debug mode is enabled.
func (sdk *ResilientBridge) debugf(format string, args ...interface{}) {
	if sdk.Debug {
		fmt.Printf("[DEBUG] "+format, args...)
	}
}
