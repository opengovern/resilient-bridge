package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"

	resilientbridge "github.com/opengovern/resilient-bridge"
	"github.com/opengovern/resilient-bridge/adapters"
)

type DopplerProjectRolesResponse struct {
	Roles []struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	} `json:"roles"`
	Success bool `json:"success"`
}

func main() {
	token := os.Getenv("YOUR_DOPPLER_API_TOKEN")
	if token == "" {
		log.Fatal("YOUR_DOPPLER_API_TOKEN not set")
	}

	// Create a new instance of the SDK
	sdk := resilientbridge.NewResilientBridge()
	// Register provider with no special ProviderConfig (nil is allowed)
	sdk.RegisterProvider("doppler", &adapters.DopplerAdapter{APIToken: token}, nil)

	req := &resilientbridge.NormalizedRequest{
		Method:   "GET",
		Endpoint: "/v3/projects/roles",
		Headers:  map[string]string{"accept": "application/json"},
	}

	resp, err := sdk.Request("doppler", req)
	if err != nil {
		log.Fatalf("Error listing project roles: %v", err)
	}

	if resp.StatusCode >= 400 {
		log.Fatalf("Error %d: %s", resp.StatusCode, string(resp.Data))
	}

	var rolesResp DopplerProjectRolesResponse
	if err := json.Unmarshal(resp.Data, &rolesResp); err != nil {
		log.Fatalf("Error parsing project roles response: %v", err)
	}

	fmt.Println("Project Roles:")
	for _, r := range rolesResp.Roles {
		fmt.Printf("- ID: %s, Name: %s\n", r.ID, r.Name)
	}
}
