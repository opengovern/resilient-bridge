package main

import (
	"fmt"
	"log"
	"time"

	resilientbridge "github.com/opengovern/resilient-bridge"
	"github.com/opengovern/resilient-bridge/adapters"
)

func intPtr(i int) *int { return &i }

func main() {
	sdk := resilientbridge.NewResilientBridge()
	// Register the HuggingFace provider (assuming adapter is already created)
	sdk.RegisterProvider("huggingface", adapters.NewHuggingFaceAdapter("YOUR_HF_API_TOKEN"), &resilientbridge.ProviderConfig{
		UseProviderLimits:   false,
		MaxRetries:          3,
		BaseBackoff:         time.Second,
		MaxRequestsOverride: intPtr(50),
	})

	req := &resilientbridge.NormalizedRequest{
		Method:   "GET",
		Endpoint: "/api/models",
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

	fmt.Println("Data:", string(resp.Data))
}
