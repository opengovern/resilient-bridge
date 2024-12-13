# Resilient-Bridge SDK

## Objective

Create a unified SDK for rapid provider integration, handling rate limits & retries, ensuring easy scalability and faster development.

## Summary

We are building a Go-based SDK named **Resilient-Bridge** that standardizes and simplifies interactions with multiple third-party API providers (e.g., OpenAI, Doppler, Heroku, Chainguard.dev, GoDaddy, Fly.io, Tailscale, GitLab, Azure DevOps, TeamCity, Semgrep, Jenkins, JIRA, ServiceNow, ElasticCloud, Mongo, etc.). The goal is to create a single, coherent framework that can handle all aspects of request execution—rate limiting, retries, and backoff—without duplicating logic for each provider.

### What We Are Trying to Accomplish:

- **Unified Interface**: Provide a common, provider-agnostic interface for making requests, so that adding a new provider only requires implementing a simple adapter rather than re-engineering the entire call stack.
  
- **Centralized Rate Limiting**: Automatically detect and manage each provider’s rate limits (from response headers or known constraints). Operate at a controlled capacity—optionally below provider maximums—to reduce the risk of hitting hard limits and ensure predictable performance.

- **Retry & Backoff Strategies**: Implement robust retry logic with configurable exponential backoff to handle transient errors and rate-limit responses gracefully. Allow per-provider configurations for maximum retries, base backoff times, and capacity targets.
  
- **Scalability & Extensibility**: Once the SDK’s core is in place, adding new providers is straightforward. The framework’s abstractions ensure that provider-specific complexities remain isolated in adapters.

### Outcome:

By completing this SDK, we will have a stable, testable, and easily maintainable foundation for integrating multiple APIs. This reduces repeated development effort, minimizes rate limit risks, and ensures consistent error handling and performance across all integrated services. The codebase will be easier to evolve as provider requirements change or as we add new integrations.

## Acceptance Criteria

- Must handle provider rate limits and apply user-defined caps.
- Must implement retries with exponential backoff on errors.
- Must define a unified request/response interface across providers.
- Must allow per-provider configs for capacity, retries, and backoff.
- Must support integrating multiple providers (e.g., OpenAI, Doppler, Semgrep).
- Must parse rate-limit headers and update internal state per request.
- Must return normalized responses regardless of provider differences.
- Must isolate provider-specific logic in separate adapter classes.
- Must enable adding new providers without changing the core code.
- Must have test coverage ensuring rate-limit and retry logic correctness.

## Directory Structure

```
/resilient-bridge
  # Module root
/adapters
  # Provider-specific adapters
  openai_adapter.go
  doppler_adapter.go
  semgrep_adapter.go
  # ...other providers
/internal
  # Internal helpers (e.g., time parsing, config)
  time_parser.go
  config.go
  interfaces.go
  rate_limiter.go
  request_executor.go
  request_response.go
  sdk.go

/examples
  /doppler
    list_users.go
    get_user.go
  /openai
    list_assistants.go
  /semgrep
    list_deployments.go
    list_projects.go
    get_project.go
```

## Getting Started

1. **Install & Import:**

   Assuming your repository is public at `github.com/opengovern/resilient-bridge`:

   ```bash
   go get github.com/opengovern/resilient-bridge
   ```

   In your Go file:

   ```go
   import "github.com/opengovern/resilient-bridge"
   ```

2. **Initialize the SDK:**

   ```go
   sdk := resilientbridge.NewResilientBridge()
   ```

3. **Register Providers:**

   Each provider must implement the `ProviderAdapter` interface. For example (Doppler):

   ```go
   import (
       "github.com/opengovern/resilient-bridge"
       "github.com/opengovern/resilient-bridge/adapters"
   )

   sdk.RegisterProvider("doppler", &adapters.DopplerAdapter{APIToken: "YOUR_TOKEN"}, &resilientbridge.ProviderConfig{
       UseProviderLimits: false,
       MaxRequestsOverride: intPtr(50),
       MaxRetries: 3,
       BaseBackoff: 0,
   })
   ```

4. **Make Requests:**

   ```go
   req := &resilientbridge.NormalizedRequest{
       Method: "GET",
       Endpoint: "/v3/workplace/users?page=1",
       Headers: map[string]string{"accept": "application/json"},
   }
   resp, err := sdk.Request("doppler", req)
   if err != nil {
       log.Fatal(err)
   }
   // Parse resp.Data as needed.
   ```

5. **Examples:**

   See the examples directory for sample code:

   - `examples/doppler/list_users.go`: Lists all Doppler users.
   - `examples/openai/list_assistants.go`: Lists OpenAI assistants.
   - `examples/semgrep/list_deployments.go`, `list_projects.go`, and `get_project.go`: Demonstrate Semgrep API usage.

   To run an example:

   ```bash
   go run ./examples/doppler/list_users.go
   ```

## Adding New Providers

- **Implement `ProviderAdapter`:**
  - `ExecuteRequest()`: How to call the provider’s API.
  - `ParseRateLimitInfo()`: How to parse and return the provider’s rate-limit headers.
  - `IsRateLimitError()`: How to detect rate-limit (e.g., 429) errors.

- **Register the new provider using `sdk.RegisterProvider`.**

- **Update configurations if needed to operate at custom capacities.**

## Configuration Options

`ProviderConfig` allows you to:

- `UseProviderLimits`: Use the provider’s actual limits or override them.
- `MaxRequestsOverride / MaxTokensOverride`: Control the upper limit of requests/tokens.
- `MaxRetries`: Set how many times to retry requests.
- `BaseBackoff`: Set initial backoff duration for exponential retry delays.

## Contributing & Customization

- **Time Parsing & Other Helpers:** Extend `internal/time_parser.go` or create new helpers as needed.
- **Testing:** Add tests for adapters, rate limiting, and retry logic.
- **Logging & Telemetry:** Integrate logging or monitoring solutions as needed.

## License

Elastic License V2
