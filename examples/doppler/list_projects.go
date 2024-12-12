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

type DopplerProjectsResponse struct {
	Projects []struct {
		Name string `json:"name"`
		Slug string `json:"slug"`
	} `json:"projects"`
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
	// Register the doppler provider without special config if desired
	sdk.RegisterProvider("doppler", &adapters.DopplerAdapter{APIToken: token}, nil)

	q := url.Values{}
	q.Set("page", "1")
	q.Set("per_page", "20")

	req := &resilientbridge.NormalizedRequest{
		Method:   "GET",
		Endpoint: "/v3/projects?" + q.Encode(),
		Headers:  map[string]string{"accept": "application/json"},
	}

	resp, err := sdk.Request("doppler", req)
	if err != nil {
		log.Fatalf("Error listing projects: %v", err)
	}

	if resp.StatusCode >= 400 {
		log.Fatalf("Error %d: %s", resp.StatusCode, string(resp.Data))
	}

	var projectsResp DopplerProjectsResponse
	if err := json.Unmarshal(resp.Data, &projectsResp); err != nil {
		log.Fatalf("Error parsing projects response: %v", err)
	}

	fmt.Printf("Page: %d / %d\n", projectsResp.Page, projectsResp.TotalPages)
	for _, p := range projectsResp.Projects {
		fmt.Printf("Project: %s (Slug: %s)\n", p.Name, p.Slug)
	}
}
