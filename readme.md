# Resilient-Bridge SDK

## Overview

**Resilient-Bridge** is a Go-based SDK designed to simplify integration with multiple third-party APIs (e.g., OpenAI, Doppler, Heroku, GitHub, Linode, Semgrep, etc.) by providing a unified interface for requests, centralized rate limiting, retries with exponential backoff, and provider-specific configurations. With Resilient-Bridge, you can scale your integrations faster and maintain a consistent approach to error handling and rate limit management across all providers.

## Key Features

- **Unified Interface**: Define requests and parse responses consistently, regardless of which provider you’re integrating.
- **Rate Limit & Retry Management**: Automatically handle rate limits and apply intelligent retries with exponential backoff, respecting `Retry-After` headers when present.
- **Per-Provider Customization**: Configure each provider with its own capacity targets, limits, and retry behavior.
- **Simple Adapters**: Keep provider-specific logic isolated in simple adapters, without rewriting core request logic.

## Basics of the SDK

Resilient-Bridge offers a streamlined way to interact with various APIs through adapters. An **adapter** is a provider-specific module that implements the `ProviderAdapter` interface, handling authentication, request execution, and rate limit parsing. This abstraction allows the core SDK to manage retries and rate limits uniformly, ensuring scalability and maintainability without duplicating logic for each provider.

## How to Use the SDK

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
    "log"
    "time"

    "github.com/opengovern/resilient-bridge"
    "github.com/opengovern/resilient-bridge/adapters"
)

func intPtr(i int) *int { return &i }

func main() {
    sdk := resilientbridge.NewResilientBridge()
    sdk.RegisterProvider("doppler", adapters.NewDopplerAdapter("YOUR_DOPPLER_API_TOKEN"), &resilientbridge.ProviderConfig{
        UseProviderLimits:   true,
        MaxRetries:          3,
        BaseBackoff:         time.Second,
        MaxRequestsOverride: intPtr(50), // Optional: Override default limits
    })
    // ...
}
```

### 4. Make Requests

Construct a `NormalizedRequest` and call `sdk.Request`:

```go
req := &resilientbridge.NormalizedRequest{
    Method:   "GET",
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

### 6. Concurrent Requests and Limits

When making concurrent requests (e.g., using goroutines), Resilient-Bridge ensures that hitting provider rate limits is minimized. Each request checks the rate limiter first; if the SDK detects that you’re close to a limit, it backs off before sending requests, reducing the chance of 429 responses.

```go
var wg sync.WaitGroup

endpoints := []string{"/endpoint1", "/endpoint2", "/endpoint3"} // Example endpoints

for _, endpoint := range endpoints {
    wg.Add(1)
    go func(ep string) {
        defer wg.Done()
        req := &resilientbridge.NormalizedRequest{
            Method:   "GET",
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

## How to Add a New Adapter

Adding support for a new provider involves creating an adapter that implements the `ProviderAdapter` interface.

### Steps to Create a New Adapter

#### 1. Create a New File in `/adapters`

For example, to integrate "SuperAPI," create `superapi_adapter.go` in the `/adapters` directory.

#### 2. Define Struct & Configuration

```go
type SuperAPIAdapter struct {
    APIToken string // internal fields
}

func NewSuperAPIAdapter(apiToken string) *SuperAPIAdapter {
    return &SuperAPIAdapter{APIToken: apiToken}
}
```

#### 3. Implement `ProviderAdapter` Interface

```go
func (s *SuperAPIAdapter) SetRateLimitDefaultsForType(requestType string, maxRequests int, windowSecs int64) {
    // Setup defaults per request type
}

func (s *SuperAPIAdapter) IdentifyRequestType(req *resilientbridge.NormalizedRequest) string {
    if req.Endpoint == "/graphql" {
        return "graphql"
    }
    if strings.ToUpper(req.Method) == "GET" {
        return "read"
    }
    return "write"
}

func (s *SuperAPIAdapter) ExecuteRequest(req *resilientbridge.NormalizedRequest) (*resilientbridge.NormalizedResponse, error) {
    // Construct and send the HTTP request.
}

func (s *SuperAPIAdapter) ParseRateLimitInfo(resp *resilientbridge.NormalizedResponse) (*resilientbridge.NormalizedRateLimitInfo, error) {
    return nil, nil
}

func (s *SuperAPIAdapter) IsRateLimitError(resp *resilientbridge.NormalizedResponse) bool {
    return resp.StatusCode == 429
}
```

#### 4. Register the New Adapter

```go
sdk.RegisterProvider("superapi", adapters.NewSuperAPIAdapter("YOUR_SUPERAPI_TOKEN"), &resilientbridge.ProviderConfig{
    UseProviderLimits:   true,
    MaxRetries:          3,
    BaseBackoff:         0,
    MaxRequestsOverride: intPtr(100),
})
```

#### 5. Test Your Adapter

```go
req := &resilientbridge.NormalizedRequest{
    Method:   "GET",
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

Developers can categorize or "group" requests into different types to apply distinct rate limits and retry behavior.

```go
func (a *MyProviderAdapter) IdentifyRequestType(req *resilientbridge.NormalizedRequest) string {
    if req.Endpoint == "/graphql" {
        return "graphql"
    }
    if strings.ToUpper(req.Method) == "GET" {
        return "read"
    }
    return "write"
}
```

Set rate limits per type:

```go
adapter.SetRateLimitDefaultsForType("read", 1000, 60)      // 1000 requests/min for read
adapter.SetRateLimitDefaultsForType("write", 200, 60)      // 200 requests/min for write
adapter.SetRateLimitDefaultsForType("graphql", 300, 300)  // 300 queries/5 min
```

## Configuration Options

`ProviderConfig` can be customized for each provider:

- **UseProviderLimits**: Use the provider’s reported rate limits.
- **MaxRequestsOverride**: Override default max requests.
- **MaxRetries**: Set how many times to retry after errors.
- **BaseBackoff**: Initial wait time for exponential backoff.
- **WindowSecsOverride**: Override the default rate limit window.

### Example

```go
sdk.RegisterProvider("doppler", adapters.NewDopplerAdapter("API_TOKEN"), &resilientbridge.ProviderConfig{
    UseProviderLimits:   true,
    MaxRetries:          5,
    BaseBackoff:         2 * time.Second,
    MaxRequestsOverride: intPtr(100),
})
```

## Examples

Check the `examples` directory for sample code:

- **Doppler**:
  - `examples/doppler/list_users.go`: Lists Doppler users.
- **OpenAI**:
  - `examples/openai/list_assistants.go`: Lists OpenAI assistants.
- **Semgrep**:
  - `examples/semgrep/list_deployments_and_projects.go`: Lists deployments and projects in Semgrep.

### Run a Doppler Example

```bash
go run ./examples/doppler/list_users.go
```

## Contributing & Customization

- Extend `internal/time_parser.go` or create new helpers as needed.
- Add tests for adapters, rate limiting, and retry logic.
- Integrate logging or monitoring as needed.

## License

Elastic License V2

---

With Resilient-Bridge, integrate once and confidently handle API limits, retries, and scaling across multiple providers.