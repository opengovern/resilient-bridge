package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"

	resilientbridge "github.com/opengovern/resilient-bridge"
	"github.com/opengovern/resilient-bridge/adapters"
)

// DopplerRolesResponse represents the JSON structure returned by the Doppler API for listing roles.
type DopplerRolesResponse struct {
	Roles []struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	} `json:"roles"`
	Success bool `json:"success"`
}

func main() {
	token := os.Getenv("YOUR_DOPPLER_API_TOKEN")
	if token == "" {
		log.Fatal("Environment variable YOUR_DOPPLER_API_TOKEN not set")
	}

	// Create a new instance of the SDK
	sdk := resilientbridge.NewResilientBridge()
	// If you want to see debug logs for the SDK, uncomment the next line:
	// sdk.SetDebug(true)

	// Register the Doppler provider. Since we're not relying on provider limits, set UseProviderLimits to false.
	// If you'd like to enforce limits or overrides, adjust config accordingly.
	sdk.RegisterProvider("doppler", &adapters.DopplerAdapter{APIToken: token}, &resilientbridge.ProviderConfig{
		UseProviderLimits:   false,
		MaxRequestsOverride: nil,
		MaxRetries:          3,
		BaseBackoff:         0,
	})

	req := &resilientbridge.NormalizedRequest{
		Method:   "GET",
		Endpoint: "/v3/workplace/roles",
		Headers:  map[string]string{"accept": "application/json"},
	}

	// Execute the request through the SDK
	resp, err := sdk.Request("doppler", req)
	if err != nil {
		log.Fatalf("Error listing workplace roles: %v", err)
	}

	// Handle HTTP errors
	if resp.StatusCode >= 400 && resp.StatusCode < 500 {
		log.Fatalf("Client error %d: %s", resp.StatusCode, string(resp.Data))
	} else if resp.StatusCode >= 500 {
		log.Fatalf("Server error %d: %s", resp.StatusCode, string(resp.Data))
	}

	// Parse the JSON response into our DopplerRolesResponse struct
	var rolesResp DopplerRolesResponse
	if err := json.Unmarshal(resp.Data, &rolesResp); err != nil {
		log.Fatalf("Error parsing roles response: %v", err)
	}

	// Validate that the request succeeded
	if !rolesResp.Success {
		fmt.Println("The request did not succeed according to the 'success' field.")
	} else {
		fmt.Println("Workplace Roles:")
		for _, role := range rolesResp.Roles {
			fmt.Printf("- ID: %s, Name: %s\n", role.ID, role.Name)
		}
	}
}
