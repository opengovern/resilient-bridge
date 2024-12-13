package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/url"
	"os"
	"time"

	resilientbridge "github.com/opengovern/resilient-bridge"
	"github.com/opengovern/resilient-bridge/adapters"
)

// PostgresListResponse represents the JSON response for listing Postgres instances.
type PostgresListResponse []struct {
	Postgres struct {
		ID            string `json:"id"`
		DatabaseName  string `json:"databaseName"`
		DatabaseUser  string `json:"databaseUser"`
		EnvironmentID string `json:"environmentId"`
		Name          string `json:"name"`
		Owner         struct {
			ID                   string `json:"id"`
			Name                 string `json:"name"`
			Email                string `json:"email"`
			TwoFactorAuthEnabled bool   `json:"twoFactorAuthEnabled"`
			Type                 string `json:"type"`
		} `json:"owner"`
		Plan                    string `json:"plan"`
		DiskSizeGB              int    `json:"diskSizeGB"`
		Region                  string `json:"region"`
		Status                  string `json:"status"`
		Role                    string `json:"role"`
		Version                 string `json:"version"`
		Suspended               string `json:"suspended"`
		CreatedAt               string `json:"createdAt"`
		UpdatedAt               string `json:"updatedAt"`
		ExpiresAt               string `json:"expiresAt"`
		DashboardUrl            string `json:"dashboardUrl"`
		HighAvailabilityEnabled bool   `json:"highAvailabilityEnabled"`
	} `json:"postgres"`
	Cursor string `json:"cursor"`
}

func main() {
	apiToken := os.Getenv("RENDER_API_TOKEN")
	if apiToken == "" {
		log.Fatal("RENDER_API_TOKEN environment variable not set")
	}

	// Create the SDK instance
	sdk := resilientbridge.NewResilientBridge()

	// Create and register the Render adapter
	// We'll assume our RenderAdapter is named RenderAdapter and located in adapters package
	// The adapter should handle rate limits and request classification as previously described.
	renderAdapter := adapters.NewRenderAdapter(apiToken)

	// We can set overrides if desired. For example, if we want to override GET limits:
	// sdk.RegisterProvider("render", renderAdapter, &resilientbridge.ProviderConfig{
	//     UseProviderLimits: true,
	//     MaxRetries:        3,
	//     BaseBackoff:       200 * time.Millisecond,
	//     // e.g. override GET requests limit if we had a category named "get"
	//     // For this example, let's just rely on defaults.
	// })

	sdk.RegisterProvider("render", renderAdapter, &resilientbridge.ProviderConfig{
		UseProviderLimits: true,
		MaxRetries:        3,
		BaseBackoff:       200 * time.Millisecond,
	})

	q := url.Values{}
	q.Set("includeReplicas", "true")
	q.Set("limit", "20")

	req := &resilientbridge.NormalizedRequest{
		Method:   "GET",
		Endpoint: "/v1/postgres?" + q.Encode(),
		Headers:  map[string]string{"accept": "application/json"},
	}

	resp, err := sdk.Request("render", req)
	if err != nil {
		log.Fatalf("Error listing Postgres instances: %v", err)
	}

	if resp.StatusCode >= 400 {
		log.Fatalf("Error %d: %s", resp.StatusCode, string(resp.Data))
	}

	var pgResp PostgresListResponse
	if err := json.Unmarshal(resp.Data, &pgResp); err != nil {
		log.Fatalf("Error parsing Postgres response: %v", err)
	}

	fmt.Printf("Retrieved %d Postgres instances.\n", len(pgResp))
	for _, pgItem := range pgResp {
		fmt.Printf("Postgres: ID=%s, Name=%s, Status=%s, Plan=%s\n",
			pgItem.Postgres.ID, pgItem.Postgres.Name, pgItem.Postgres.Status, pgItem.Postgres.Plan)
	}
}
