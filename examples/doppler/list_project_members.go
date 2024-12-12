package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/url"
	"os"

	resilientbridge "github.com/opengovern/resilient-bridge"
	"github.com/opengovern/resilient-bridge/adapters"
)

// DopplerProjectMembersResponse represents the JSON response for listing project members.
// Adjust fields based on the actual API return.
type DopplerProjectMembersResponse struct {
	Members []struct {
		Email string `json:"email"`
		Name  string `json:"name"`
	} `json:"members"`
	Page       int  `json:"page"`
	TotalPages int  `json:"total_pages"`
	Success    bool `json:"success"`
}

func main() {
	token := os.Getenv("YOUR_DOPPLER_API_TOKEN")
	if token == "" {
		log.Fatal("YOUR_DOPPLER_API_TOKEN not set")
	}

	// Create a new instance of the SDK
	sdk := resilientbridge.NewResilientBridge()
	// Optionally provide a ProviderConfig if you want retries/backoff
	sdk.RegisterProvider("doppler", &adapters.DopplerAdapter{APIToken: token}, &resilientbridge.ProviderConfig{
		UseProviderLimits: true,
		MaxRetries:        3,
		BaseBackoff:       0,
	})

	projectSlug := "project" // replace with the actual project slug
	q := url.Values{}
	q.Set("page", "1")
	q.Set("per_page", "20")

	req := &resilientbridge.NormalizedRequest{
		Method:   "GET",
		Endpoint: "/v3/projects/" + projectSlug + "/members?" + q.Encode(),
		Headers:  map[string]string{"accept": "application/json"},
	}

	resp, err := sdk.Request("doppler", req)
	if err != nil {
		log.Fatalf("Error listing project members: %v", err)
	}

	if resp.StatusCode >= 400 {
		log.Fatalf("Error %d: %s", resp.StatusCode, string(resp.Data))
	}

	var membersResp DopplerProjectMembersResponse
	if err := json.Unmarshal(resp.Data, &membersResp); err != nil {
		log.Fatalf("Error parsing project members response: %v", err)
	}

	fmt.Printf("Page: %d / %d\n", membersResp.Page, membersResp.TotalPages)
	for _, m := range membersResp.Members {
		fmt.Printf("Member: %s (%s)\n", m.Name, m.Email)
	}
}
