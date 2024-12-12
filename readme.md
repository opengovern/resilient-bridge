# Unified SDK

## Objective

Create a unified SDK for rapid provider integration, handling rate limits & retries, ensuring easy scalability and faster development.

## Summary

We are building a unified, Go-based SDK designed to standardize and simplify interactions with multiple third-party API providers (e.g., OpenAI, Doppler, Heroku, Chainguard, GoDaddy, Fly.io, Tailscale, GitLab, Azure DevOps, TeamCity, Semgrep, Jenkins, JIRA, ServiceNow, ElasticCloud, Mongo, etc.). The goal is to create a single, coherent framework that can handle all aspects of request execution—rate limiting, retries, and backoff—without duplicating logic for each provider.

### What We Are Trying to Accomplish:

- **Unified Interface**: Define a common, provider-agnostic interface for making requests so that adding a new provider only requires implementing a simple adapter rather than re-engineering the entire call stack.
  
- **Centralized Rate Limiting**: Automatically detect and manage each provider’s rate limits (retrieved from response headers or known constraints). We aim to operate at a controlled capacity—optionally below provider maximums—to reduce the risk of hitting hard limits and ensure more predictable performance.
  
- **Retry & Backoff Strategies**: Implement a robust retry mechanism with configurable exponential backoff to handle transient errors and rate-limit responses gracefully. We will allow per-provider configurations for maximum retries, base backoff times, and overrideable capacity targets.
  
- **Scalability & Extensibility**: Once the SDK’s core is in place, adding new providers should be straightforward. The framework’s abstractions ensure that the complexity of each provider's integration (headers, endpoints, authentication, rate limit formats) remains isolated in its adapter.

### Outcome:

By completing this SDK, we will have a stable, testable, and easily maintainable foundation for integrating multiple APIs. This reduces repeated development effort, minimizes the risk of overstepping rate limits, and ensures consistent error handling and performance across all integrated services. The codebase will be easier to evolve as provider requirements change or as new integrations are introduced.

## Acceptance Criteria

- Must handle provider rate limits and apply custom user-defined caps.
- Must implement retries with exponential backoff on errors.
- Must define a unified request/response interface across providers.
- Must allow per-provider configs for capacity, retries, and backoff.
- Must support integrating multiple providers (e.g., OpenAI, Doppler).
- Must parse rate-limit headers and update internal state per request.
- Must return normalized responses regardless of provider differences.
- Must isolate provider-specific logic in separate adapter classes.
- Must enable adding new providers without core code modifications.
- Must have test coverage ensuring rate-limit and retry logic correctness.

## Directory Structure

```
/unifiedsdk
  # Core SDK code
/adapters
  # Provider-specific adapters
  openai_adapter.go
  doppler_adapter.go
  # ...other providers
/internal
  # Internal helpers (e.g., time parsing)
  time_parser.go
  config.go         # ProviderConfig definition
  interfaces.go     # ProviderAdapter interface
  rate_limiter.go   # RateLimiter logic
  request_executor.go # Retry & backoff logic
  request_response.go # NormalizedRequest/Response definitions
  sdk.go            # UnifiedSDK struct, registration & request methods

/examples
  /doppler
    list_users.go   # Example: listing Doppler users
    get_user.go     # Example: getting a single Doppler user by email
```

## Getting Started

1. **Install & Import:**

   ```bash
   go get opengovern/opencomply/utils/unifiedsdk
   ```

   In your Go file:

   ```go
   import "opengovern/opencomply/utils/unifiedsdk"
   ```

2. **Initialize the SDK:**

   ```go
   sdk := unifiedsdk.NewUnifiedSDK()
   ```

3. **Register Providers:**

   Each provider must implement the `ProviderAdapter` interface. Register an adapter and its config:

   ```go
   sdk.RegisterProvider("doppler", &adapters.DopplerAdapter{APIToken: "YOUR_TOKEN"}, &unifiedsdk.ProviderConfig{
       UseProviderLimits: false,
       MaxRequestsOverride: intPtr(50),
       MaxRetries: 3,
   })
   ```

4. **Make Requests:**

   Create a `NormalizedRequest` and call `sdk.Request`:

   ```go
   req := &unifiedsdk.NormalizedRequest{
       Method: "GET",
       Endpoint: "/v3/workplace/users?page=1",
       Headers: map[string]string{"accept": "application/json"},
   }
   resp, err := sdk.Request("doppler", req)
   if err != nil {
       log.Fatal(err)
   }
   // Parse resp.Data as JSON or whatever format is returned.
   ```

5. **Examples**

   See the `examples/doppler` directory for sample code:

   - `list_users.go`: Lists all Doppler users.
   - `get_user.go`: Retrieves a Doppler user by email.

   To run:

   ```bash
   go run ./examples/doppler/list_users.go
   ```

## Adding New Providers

- **Implement `ProviderAdapter`:**
  - `ExecuteRequest()`: How to call the provider’s API.
  - `ParseRateLimitInfo()`: How to parse and return the provider’s rate-limit headers.
  - `IsRateLimitError()`: How to detect rate-limit (e.g., 429) errors.

- **Register the new provider with `RegisterProvider`.**

- **Update your configs if needed to operate at custom capacities.**

## Configuration Options

`ProviderConfig` allows you to:

- `UseProviderLimits`: Use the provider’s actual limits or override them.
- `MaxRequestsOverride/MaxTokensOverride`: Control the upper limit of requests/tokens.
- `MaxRetries`: Set how many times to retry requests.
- `BaseBackoff`: Set initial backoff duration for exponential retry delays.

## Contributing & Customization

- **Time Parsing & Other Helpers:** Extend `internal/time_parser.go` or create new helpers as providers vary in their rate-limit time formats.
- **Testing:** Add tests for adapters, rate limiting, and retry logic.
- **Logging & Telemetry:** Integrate logging frameworks or monitoring solutions as needed.

## License

MIT
