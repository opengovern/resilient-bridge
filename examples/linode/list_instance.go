// list_instance.go
package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/url"
	"os"
	"strconv"

	resilientbridge "github.com/opengovern/resilient-bridge"
	"github.com/opengovern/resilient-bridge/adapters"
)

// LinodesResponse represents the paginated response from the Linode list endpoint
type LinodesResponse struct {
	Data    []Linode `json:"data"`
	Page    int      `json:"page"`
	Pages   int      `json:"pages"`
	Results int      `json:"results"`
}

type Linode struct {
	ID     int      `json:"id"`
	Label  string   `json:"label"`
	Region string   `json:"region"`
	IPv4   []string `json:"ipv4"`
	Status string   `json:"status"`
	Type   string   `json:"type"`
}

func main() {
	apiToken := os.Getenv("LINODE_API_TOKEN")
	if apiToken == "" {
		log.Fatal("LINODE_API_TOKEN environment variable not set")
	}

	// Check if debug mode is enabled
	debugMode := false
	if val := os.Getenv("DEBUG"); val == "true" {
		debugMode = true
	}

	// Set up the SDK and register Linode provider
	sdk := resilientbridge.NewResilientBridge()
	sdk.SetDebug(debugMode) // Assume we have a SetDebug method to control debug output

	sdk.RegisterProvider("linode", adapters.NewLinodeAdapter(apiToken), &resilientbridge.ProviderConfig{
		UseProviderLimits: true,
		MaxRetries:        3,
		BaseBackoff:       0,
	})

	// Pagination parameters
	page := 1
	pageSize := 100 // adjust between 25 and 500 if needed

	var allLinodes []Linode

	for {
		q := url.Values{}
		q.Set("page", strconv.Itoa(page))
		q.Set("page_size", strconv.Itoa(pageSize))

		req := &resilientbridge.NormalizedRequest{
			Method:   "GET",
			Endpoint: "/linode/instances?" + q.Encode(),
			Headers:  map[string]string{"accept": "application/json"},
		}

		resp, err := sdk.Request("linode", req)
		if err != nil {
			log.Fatalf("Error requesting linodes: %v", err)
		}

		if resp.StatusCode >= 400 {
			log.Fatalf("Error %d: %s", resp.StatusCode, string(resp.Data))
		}

		var linodesResp LinodesResponse
		if err := json.Unmarshal(resp.Data, &linodesResp); err != nil {
			log.Fatalf("Error parsing response: %v", err)
		}

		allLinodes = append(allLinodes, linodesResp.Data...)

		fmt.Printf("Fetched page %d of %d, total results: %d\n", linodesResp.Page, linodesResp.Pages, linodesResp.Results)

		if linodesResp.Page >= linodesResp.Pages {
			break
		}
		page++
	}

	fmt.Printf("Fetched %d Linodes in total.\n", len(allLinodes))
	for _, linode := range allLinodes {
		fmt.Printf("ID: %d, Label: %s, Region: %s, Status: %s, Type: %s, IPv4: %v\n",
			linode.ID, linode.Label, linode.Region, linode.Status, linode.Type, linode.IPv4)
	}
}
