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

// Incident represents a secret incident
type Incident struct {
	ID         int    `json:"id"`
	Date       string `json:"date"`
	Status     string `json:"status"`
	AssigneeID int    `json:"assignee_id"`
	Severity   string `json:"severity"`
}

func main() {
	apiToken := os.Getenv("GITGUARDIAN_API_TOKEN")
	if apiToken == "" {
		log.Fatal("GITGUARDIAN_API_TOKEN not set")
	}

	// Suppose we have a personal token and a paid plan
	sdk := resilientbridge.NewResilientBridge()
	sdk.RegisterProvider("gitguardian", adapters.NewGitGuardianAdapter(apiToken, "personal", true), &resilientbridge.ProviderConfig{
		UseProviderLimits: true,
		MaxRetries:        3,
		BaseBackoff:       0,
	})

	q := url.Values{}
	q.Set("per_page", "50") // get 50 incidents per page
	req := &resilientbridge.NormalizedRequest{
		Method:   "GET",
		Endpoint: "/v1/incidents/secrets?" + q.Encode(),
		Headers:  map[string]string{"accept": "application/json"},
	}

	resp, err := sdk.Request("gitguardian", req)
	if err != nil {
		log.Fatalf("Error requesting incidents: %v", err)
	}

	if resp.StatusCode >= 400 {
		log.Fatalf("Error %d: %s", resp.StatusCode, string(resp.Data))
	}

	var incidents []Incident
	if err := json.Unmarshal(resp.Data, &incidents); err != nil {
		log.Fatalf("Error parsing response: %v", err)
	}

	fmt.Printf("Fetched %d incidents.\n", len(incidents))
	for _, inc := range incidents {
		fmt.Printf("ID: %d, Date: %s, Status: %s, Severity: %s\n", inc.ID, inc.Date, inc.Status, inc.Severity)
	}
}
