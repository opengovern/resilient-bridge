package main

import (
	"log"
	"os"
	"time"

	"github.com/opengovern/resilient-bridge/adapters"
)

func intPtr(i int) *int { return &i }

func main() {
	sdk := resilientbridge.NewResilientBridge()
	apiToken := os.Getenv("HF_API_TOKEN")
	if apiToken == "" {
		log.Println("HF_API_TOKEN not set; you may only be able to access public repos")
	}
	sdk.RegisterProvider("huggingface", adapters.NewHuggingFaceAdapter(apiToken), &resilientbridge.ProviderConfig{
		UseProviderLimits:   false, // Change to true if future HuggingFace rate limits are known
		MaxRetries:          3,
		BaseBackoff:         time.Second,
		MaxRequestsOverride: intPtr(100),
	})

	req := &resilientbridge.NormalizedRequest{
		Method:   "GET",
		Endpoint: "/api/models", // Example: list models
		Headers: map[string]string{
			"accept": "application/json",
		},
	}

	resp, err := sdk.Request("huggingface", req)
	if err != nil {
		log.Fatalf("Error: %v", err)
	}

	if resp.StatusCode >= 400 {
		log.Fatalf("HTTP Error %d: %s", resp.StatusCode, string(resp.Data))
	}

	log.Println("HuggingFace response:", string(resp.Data))
}
