package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/url"
	"os"
	"unifiedsdk"
	"unifiedsdk/adapters"
)

// DopplerConfigsResponse represents a possible response structure for listing configs.
// Adjust fields after seeing real API responses.
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
	// Command-line flags
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

	sdk := unifiedsdk.NewUnifiedSDK()
	sdk.RegisterProvider("doppler", &adapters.DopplerAdapter{APIToken: token}, &unifiedsdk.ProviderConfig{
		UseProviderLimits: true,
		MaxRetries:        3,
		BaseBackoff:       0,
	})

	q := url.Values{}
	q.Set("project", *project)
	if *environment != "" {
		q.Set("environment", *environment)
	}
	q.Set("page", fmt.Sprintf("%d", *page))
	q.Set("per_page", fmt.Sprintf("%d", *perPage))

	req := &unifiedsdk.NormalizedRequest{
		Method:   "GET",
		Endpoint: "/v3/configs?" + q.Encode(),
		Headers:  map[string]string{"accept": "application/json"},
	}

	resp, err := sdk.Request("doppler", req)
	if err != nil {
		log.Fatalf("Error listing configs: %v", err)
	}

	if resp.StatusCode >= 400 {
		log.Fatalf("Error %d: %s", resp.StatusCode, string(resp.Data))
	}

	var configsResp DopplerConfigsResponse
	if err := json.Unmarshal(resp.Data, &configsResp); err != nil {
		log.Fatalf("Error parsing configs response: %v", err)
	}

	fmt.Printf("Page: %d / %d\n", configsResp.Page, configsResp.TotalPages)
	fmt.Println("Configs:")
	for _, c := range configsResp.Configs {
		fmt.Printf("- %s (Slug: %s)\n", c.Name, c.Slug)
	}
}
