package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"time"

	resilientbridge "github.com/opengovern/resilient-bridge"
	"github.com/opengovern/resilient-bridge/adapters"
)

// AssistantsListResponse represents the JSON response from listing assistants.
type AssistantsListResponse struct {
	Object string `json:"object"`
	Data   []struct {
		ID             string         `json:"id"`
		Object         string         `json:"object"`
		CreatedAt      int64          `json:"created_at"`
		Name           *string        `json:"name"`
		Description    *string        `json:"description"`
		Model          string         `json:"model"`
		Instructions   *string        `json:"instructions"`
		Tools          []any          `json:"tools"`
		ToolResources  map[string]any `json:"tool_resources"`
		Metadata       map[string]any `json:"metadata"`
		TopP           float64        `json:"top_p"`
		Temperature    float64        `json:"temperature"`
		ResponseFormat interface{}    `json:"response_format"`
	} `json:"data"`
	FirstID string `json:"first_id"`
	LastID  string `json:"last_id"`
	HasMore bool   `json:"has_more"`
}

func main() {
	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		log.Fatal("OPENAI_API_KEY environment variable not set")
	}

	// Initialize the SDK
	sdk := resilientbridge.NewResilientBridge()
	// Register OpenAI provider, optionally overriding limits if needed
	// For demonstration, let's just use defaults (no overrides)
	sdk.RegisterProvider("openai", &adapters.OpenAIAdapter{APIKey: apiKey}, &resilientbridge.ProviderConfig{
		UseProviderLimits: true,
		MaxRetries:        3,
		BaseBackoff:       200 * time.Millisecond, // Adding a small base backoff for retries
	})

	req := &resilientbridge.NormalizedRequest{
		Method:   "GET",
		Endpoint: "/v1/assistants?order=desc&limit=20",
		Headers: map[string]string{
			"Content-Type": "application/json",
			"OpenAI-Beta":  "assistants=v2",
		},
	}

	resp, err := sdk.Request("openai", req)
	if err != nil {
		log.Fatalf("Error listing assistants: %v", err)
	}

	// Print raw response for debugging
	fmt.Println("Raw Response:", string(resp.Data))

	if resp.StatusCode >= 400 && resp.StatusCode < 500 {
		log.Fatalf("Client error %d: %s", resp.StatusCode, string(resp.Data))
	} else if resp.StatusCode >= 500 {
		log.Fatalf("Server error %d: %s", resp.StatusCode, string(resp.Data))
	}

	var assistantsResp AssistantsListResponse
	if err := json.Unmarshal(resp.Data, &assistantsResp); err != nil {
		log.Fatalf("Error parsing assistants list response: %v", err)
	}

	fmt.Printf("Retrieved %d assistants. Has more: %v\n", len(assistantsResp.Data), assistantsResp.HasMore)
	for _, asst := range assistantsResp.Data {
		nameVal := "<nil>"
		if asst.Name != nil {
			nameVal = *asst.Name
		}
		fmt.Printf("Assistant: ID=%s, Name=%s, Model=%s\n", asst.ID, nameVal, asst.Model)
	}
}
