# Resilient-Bridge SDK

## Overview

Resilient-Bridge is a Go-based SDK designed to simplify integration with multiple third-party APIs (e.g., OpenAI, Doppler, Heroku, GitHub, Linode, Semgrep, etc.) by providing a unified interface for requests, centralized rate limiting, retries with exponential backoff, and provider-specific configurations. With Resilient-Bridge, you can scale your integrations faster and maintain a consistent approach to error handling and rate limit management across all providers.

## Key Features

- **Unified Interface**: Define requests and parse responses consistently, regardless of which provider you’re integrating.
- **Rate Limit & Retry Management**: Automatically handle rate limits and apply intelligent retries with exponential backoff.
- **Per-Provider Customization**: Configure each provider with its own capacity targets, limits, and retry behavior.
- **Simple Adapters**: Keep provider-specific logic isolated in simple adapters, without rewriting core request logic.

## Getting Started

### 1. Install & Import

Assuming your repository is public at `github.com/opengovern/resilient-bridge`:

```bash
go get github.com/opengovern/resilient-bridge
```

In your Go file:

```go
import "github.com/opengovern/resilient-bridge"
```

### 2. Initialize the SDK

```go
sdk := resilientbridge.NewResilientBridge()
```

### 3. Register Providers

Each provider must implement the `ProviderAdapter` interface. For example, to register Doppler:

```go
import (
    "github.com/opengovern/resilient-bridge"
    "github.com/opengovern/resilient-bridge/adapters"
)

sdk.RegisterProvider("doppler", adapters.NewDopplerAdapter("YOUR_DOPPLER_API_TOKEN"), &resilientbridge.ProviderConfig{
    UseProviderLimits:    true,
    MaxRetries:           3,
    BaseBackoff:          time.Second,
    MaxRequestsOverride:  intPtr(50), // Optional: Override default limits
})
```

### 4. Make Requests

Construct a `NormalizedRequest` and call `sdk.Request`:

```go
req := &resilientbridge.NormalizedRequest{
    Method:  "GET",
    Endpoint: "/v3/workplace/users",
    Headers: map[string]string{
        "accept": "application/json",
    },
}
resp, err := sdk.Request("doppler", req)
if err != nil {
    log.Fatalf("Error: %v", err)
}
if resp.StatusCode >= 400 {
    log.Fatalf("HTTP Error %d: %s", resp.StatusCode, string(resp.Data))
}
fmt.Println("Data:", string(resp.Data))
```

### 5. Enable Debugging

To see debug logs for requests, retries, and backoff:

```go
sdk.SetDebug(true)
```

## Advanced Usage

### Adding New Providers

When adding support for a new provider, create a new adapter that implements the `ProviderAdapter` interface.

#### Steps to Create a New Adapter

1. **Create a New File in `/adapters`**  
   For example, to integrate "SuperAPI," create `superapi_adapter.go` in the `/adapters` directory.

2. **Define Struct & Configuration**

    ```go
    type SuperAPIAdapter struct {
        APIToken string
        // internal fields, like timestamps or limits
    }

    func NewSuperAPIAdapter(apiToken string) *SuperAPIAdapter {
        return &SuperAPIAdapter{APIToken: apiToken}
    }
    ```

3. **Implement `ProviderAdapter` Interface**

    ```go
    // SetRateLimitDefaultsForType sets default rate limits for different request types
    func (s *SuperAPIAdapter) SetRateLimitDefaultsForType(requestType string, maxRequests int, windowSecs int64) {
        // Initialize your adapter’s internal counters and defaults.
    }

    // IdentifyRequestType categorizes the request type
    func (s *SuperAPIAdapter) IdentifyRequestType(req *resilientbridge.NormalizedRequest) string {
        // Example logic to categorize request types
        if req.Endpoint == "/graphql" {
            return "graphql"
        }
        if strings.ToUpper(req.Method) == "GET" {
            return "read"
        }
        return "write"
    }

    // ExecuteRequest performs the actual HTTP call to the provider
    func (s *SuperAPIAdapter) ExecuteRequest(req *resilientbridge.NormalizedRequest) (*resilientbridge.NormalizedResponse, error) {
        // Construct the HTTP request, add auth headers, and send it.
        // Parse the response body and headers into NormalizedResponse.
    }

    // ParseRateLimitInfo extracts rate limit information from the response
    func (s *SuperAPIAdapter) ParseRateLimitInfo(resp *resilientbridge.NormalizedResponse) (*resilientbridge.NormalizedRateLimitInfo, error) {
        // Parse rate limit headers like X-RateLimit-Limit, etc.
        return nil, nil
    }

    // IsRateLimitError checks if the response indicates a rate limit error
    func (s *SuperAPIAdapter) IsRateLimitError(resp *resilientbridge.NormalizedResponse) bool {
        return resp.StatusCode == 429
    }
    ```

4. **Register the New Adapter**

    ```go
    sdk.RegisterProvider("superapi", adapters.NewSuperAPIAdapter("YOUR_SUPERAPI_TOKEN"), &resilientbridge.ProviderConfig{
        UseProviderLimits:    true,
        MaxRetries:           3,
        BaseBackoff:          0,
        MaxRequestsOverride:  intPtr(100), // Optional
    })
    ```

5. **Test Your Adapter**

    ```go
    req := &resilientbridge.NormalizedRequest{
        Method:  "GET",
        Endpoint: "/v1/superapi/data",
        Headers: map[string]string{
            "accept": "application/json",
        },
    }
    resp, err := sdk.Request("superapi", req)
    if err != nil {
        log.Fatal(err)
    }
    fmt.Println("SuperAPI Response:", string(resp.Data))
    ```

### Grouping Requests by Type

Developers can categorize or "group" requests into different types to apply distinct rate limits and retry behavior for each group.

#### How to Group and Apply Custom Limits by Type

1. **Identify Different Request Types**: Decide how to classify requests, e.g., "rest", "graphql", "read" vs. "write".

2. **Implement `IdentifyRequestType` in the Adapter**:

    ```go
    func (a *MyProviderAdapter) IdentifyRequestType(req *resilientbridge.NormalizedRequest) string {
        // Example logic to determine request type
        if req.Endpoint == "/graphql" {
            return "graphql"
        }
        if strings.ToUpper(req.Method) == "GET" {
            return "read"
        }
        return "write"
    }
    ```

3. **Set Rate Limit Defaults for Each Type**:

    ```go
    sdk := resilientbridge.NewResilientBridge()
    adapter := adapters.NewMyProviderAdapter("API_TOKEN")
    
    // Register the provider
    sdk.RegisterProvider("myprovider", adapter, &resilientbridge.ProviderConfig{
        UseProviderLimits:    true,
        MaxRetries:           3,
        BaseBackoff:          0,
    })
    
    // Set custom limits
    adapter.SetRateLimitDefaultsForType("read", 1000, 60)      // 1000 requests/min for read
    adapter.SetRateLimitDefaultsForType("write", 200, 60)      // 200 requests/min for write
    adapter.SetRateLimitDefaultsForType("graphql", 300, 300)  // 300 queries per 5 minutes
    ```

4. **Enforcement in `ExecuteRequest`**:  
   The SDK's `RateLimiter` checks the appropriate rate limits based on the request type before proceeding.

#### Benefits of Grouping by Type

- **Granular Control**: Adjust limits based on API-specific rules or business needs.
- **Different Plans or Endpoints**: Enforce distinct limits for premium vs. basic access.
- **Easy Maintenance**: Update logic without altering core SDK functions.

## Configuration Options

`ProviderConfig` allows you to customize the behavior of each provider:

- `UseProviderLimits`: Use the provider’s actual rate limits or override them.
- `MaxRequestsOverride` / `MaxTokensOverride`: Control the upper limit of requests/tokens.
- `MaxRetries`: Set how many times to retry requests.
- `BaseBackoff`: Set initial backoff duration for exponential retry delays.

Example:

```go
sdk.RegisterProvider("doppler", adapters.NewDopplerAdapter("API_TOKEN"), &resilientbridge.ProviderConfig{
    UseProviderLimits:    true,
    MaxRetries:           5,
    BaseBackoff:          time.Second * 2,
    MaxRequestsOverride:  intPtr(100), // Custom overrides
})
```

## Concurrent Requests and Limits

When making concurrent requests (e.g., using goroutines to speed up data fetching), the SDK’s rate limit and retry logic ensures that hitting provider rate limits is minimized. Each request checks with the `RateLimiter` before proceeding, and if rate-limited, backs off automatically.

Example:

```go
var wg sync.WaitGroup
endpoints := []string{"/endpoint1", "/endpoint2", "/endpoint3"} // Example endpoints

for _, endpoint := range endpoints {
    wg.Add(1)
    go func(ep string) {
        defer wg.Done()
        req := &resilientbridge.NormalizedRequest{
            Method:  "GET",
            Endpoint: ep,
            Headers: map[string]string{
                "accept": "application/json",
            },
        }
        resp, err := sdk.Request("doppler", req)
        if err != nil {
            log.Println("Error:", err)
            return
        }
        fmt.Printf("Fetched data from %s: %s\n", ep, string(resp.Data))
    }(endpoint)
}

wg.Wait()
```

## Examples

See the `examples` directory for sample code demonstrating how to use Resilient-Bridge with different providers:

- **Doppler**:
  - `examples/doppler/list_users.go`: Lists all Doppler users.
  - `examples/doppler/get_user.go`: Retrieves a Doppler user by email.

- **OpenAI**:
  - `examples/openai/list_assistants.go`: Lists OpenAI assistants.

- **Semgrep**:
  - `examples/semgrep/list_deployments.go`: Lists Semgrep deployments.
  - `examples/semgrep/list_projects.go`: Lists Semgrep projects.
  - `examples/semgrep/get_project.go`: Retrieves a Semgrep project by ID.

### Running an Example

To run a Doppler example:

```bash
go run ./examples/doppler/list_users.go
```

## Contributing & Customization

- **Time Parsing & Other Helpers**: Extend `internal/time_parser.go` or create new helpers as needed.
- **Testing**: Add tests for adapters, rate limiting, and retry logic.
- **Logging & Telemetry**: Integrate logging or monitoring solutions as needed.

## License

Elastic License V2

---