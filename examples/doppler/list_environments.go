// list_environments.go
package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/url"
	"os"
	"unifiedsdk"

	"github.com/opengovern/resilient-bridge/adapters"
)

// Adjust fields based on actual return
type DopplerEnvironmentsResponse struct {
	Environments []struct {
		Name string `json:"name"`
		Slug string `json:"slug"`
	} `json:"environments"`
	Success bool `json:"success"`
}

func main() {
	token := os.Getenv("YOUR_DOPPLER_API_TOKEN")
	if token == "" {
		log.Fatal("YOUR_DOPPLER_API_TOKEN not set")
	}

	sdk := unifiedsdk.NewUnifiedSDK()
	sdk.RegisterProvider("doppler", &adapters.DopplerAdapter{APIToken: token}, nil)

	// 'project' query param is required. Replace 'my-project' with actual project slug.
	q := url.Values{}
	q.Set("project", "my-project")

	req := &unifiedsdk.NormalizedRequest{
		Method:   "GET",
		Endpoint: "/v3/environments?" + q.Encode(),
		Headers:  map[string]string{"accept": "application/json"},
	}

	resp, err := sdk.Request("doppler", req)
	if err != nil {
		log.Fatalf("Error listing environments: %v", err)
	}

	if resp.StatusCode >= 400 {
		log.Fatalf("Error %d: %s", resp.StatusCode, string(resp.Data))
	}

	var envResp DopplerEnvironmentsResponse
	if err := json.Unmarshal(resp.Data, &envResp); err != nil {
		log.Fatalf("Error parsing environments response: %v", err)
	}

	fmt.Println("Environments:")
	for _, e := range envResp.Environments {
		fmt.Printf("- %s (Slug: %s)\n", e.Name, e.Slug)
	}
}
