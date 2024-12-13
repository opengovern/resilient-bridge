// list_configs.go
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/url"
	"os"
	"time"

	resilientbridge "github.com/opengovern/resilient-bridge"
	"github.com/opengovern/resilient-bridge/adapters"
)

// DopplerConfigsResponse represents the JSON response for listing configs.
type DopplerConfigsResponse struct {
	Configs []struct {
		Name string `json:"name"`
		Slug string `json:"slug"`
	} `json:"configs"`
	Page       int  `json:"page"`
	TotalPages int  `json:"total_pages"`
	Success    bool `json:"success"`
}

func main() {
	// Define command-line flags
	project := flag.String("project", "", "Project slug (required)")
	environment := flag.String("environment", "", "Environment slug (optional)")
	page := flag.Int("page", 1, "Page number")
	perPage := flag.Int("per_page", 20, "Items per page")

	flag.Parse()

	// Validate required params
	if *project == "" {
		log.Fatal("The --project parameter is required")
	}

	token := os.Getenv("YOUR_DOPPLER_API_TOKEN")
	if token == "" {
		log.Fatal("Environment variable YOUR_DOPPLER_API_TOKEN not set")
	}

	// Create a new instance of the SDK
	sdk := resilientbridge.NewResilientBridge()
	// If you want to see debug logs, uncomment the next line:
	// sdk.SetDebug(true)

	// Optional overrides for demonstration:
	// Using custom limits for REST calls only since Doppler doesn't use GraphQL.
	restMaxRequests := 500      // override REST max requests
	restWindowSecs := int64(60) // override REST window in seconds

	// Register Doppler provider
	sdk.RegisterProvider("doppler", &adapters.DopplerAdapter{APIToken: token}, &resilientbridge.ProviderConfig{
		UseProviderLimits:   true,
		MaxRequestsOverride: &restMaxRequests,
		WindowSecsOverride:  &restWindowSecs,
		MaxRetries:          3,
		BaseBackoff:         200 * time.Millisecond,
	})

	// Build query parameters
	q := url.Values{}
	q.Set("project", *project)
	if *environment != "" {
		q.Set("environment", *environment)
	}
	q.Set("page", fmt.Sprintf("%d", *page))
	q.Set("per_page", fmt.Sprintf("%d", *perPage))

	req := &resilientbridge.NormalizedRequest{
		Method:   "GET",
		Endpoint: "/v3/configs?" + q.Encode(),
		Headers:  map[string]string{"accept": "application/json"},
	}

	// Execute the request through the SDK
	resp, err := sdk.Request("doppler", req)
	if err != nil {
		log.Fatalf("Error listing configs: %v", err)
	}

	// Check HTTP status
	if resp.StatusCode >= 400 {
		log.Fatalf("Error %d: %s", resp.StatusCode, string(resp.Data))
	}

	// Parse the JSON response into our DopplerConfigsResponse struct
	var configsResp DopplerConfigsResponse
	if err := json.Unmarshal(resp.Data, &configsResp); err != nil {
		log.Fatalf("Error parsing configs response: %v", err)
	}

	// Display results
	fmt.Printf("Page: %d / %d\n", configsResp.Page, configsResp.TotalPages)
	fmt.Println("Configs:")
	for _, c := range configsResp.Configs {
		fmt.Printf("- %s (Slug: %s)\n", c.Name, c.Slug)
	}
}
