package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/url"
	"os"

	"unifiedsdk"
	"unifiedsdk/adapters"
)

// DopplerUsersResponse represents the JSON structure returned by the Doppler API for listing users.
// Adjust fields based on the actual API response.
type DopplerUsersResponse struct {
	Users []struct {
		Email string `json:"email"`
		Name  string `json:"name"`
	} `json:"users"`
	Page       int  `json:"page"`
	TotalPages int  `json:"total_pages"`
	Success    bool `json:"success"`
}

func main() {
	token := os.Getenv("YOUR_DOPPLER_API_TOKEN")
	if token == "" {
		log.Fatal("Environment variable YOUR_DOPPLER_API_TOKEN not set")
	}

	sdk := unifiedsdk.NewUnifiedSDK()
	sdk.RegisterProvider("doppler", &adapters.DopplerAdapter{APIToken: token}, &unifiedsdk.ProviderConfig{
		UseProviderLimits:   false,
		MaxRequestsOverride: nil,
		MaxRetries:          3,
		BaseBackoff:         0,
	})

	page := 1
	q := url.Values{}
	q.Set("page", fmt.Sprintf("%d", page))
	// Optional: q.Set("email", "someemail@example.com")

	req := &unifiedsdk.NormalizedRequest{
		Method:   "GET",
		Endpoint: "/v3/workplace/users?" + q.Encode(),
		Headers:  map[string]string{"accept": "application/json"},
	}

	resp, err := sdk.Request("doppler", req)
	if err != nil {
		log.Fatalf("Error listing workplace users: %v", err)
	}

	if resp.StatusCode >= 400 && resp.StatusCode < 500 {
		log.Fatalf("Client error %d: %s", resp.StatusCode, string(resp.Data))
	} else if resp.StatusCode >= 500 {
		log.Fatalf("Server error %d: %s", resp.StatusCode, string(resp.Data))
	}

	var usersResp DopplerUsersResponse
	if err := json.Unmarshal(resp.Data, &usersResp); err != nil {
		log.Fatalf("Error parsing users response: %v", err)
	}

	fmt.Printf("Page: %d / %d\n", usersResp.Page, usersResp.TotalPages)
	if len(usersResp.Users) == 0 {
		fmt.Println("No users found.")
		return
	}

	for _, user := range usersResp.Users {
		fmt.Printf("User: %s (%s)\n", user.Name, user.Email)
	}
}
