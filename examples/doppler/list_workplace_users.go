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

type DopplerUsersResponse struct {
	WorkplaceUsers []struct {
		ID        string `json:"id"`
		Access    string `json:"access"`
		CreatedAt string `json:"created_at"`
		User      struct {
			Email           string `json:"email"`
			Name            string `json:"name"`
			Username        string `json:"username"`
			ProfileImageURL string `json:"profile_image_url"`
		} `json:"user"`
	} `json:"workplace_users"`
	Page    int  `json:"page"`
	Success bool `json:"success"`
}

func main() {
	token := os.Getenv("YOUR_DOPPLER_API_TOKEN")
	if token == "" {
		log.Fatal("Environment variable YOUR_DOPPLER_API_TOKEN not set")
	}

	sdk := resilientbridge.NewResilientBridge()
	sdk.SetDebug(false)

	sdk.RegisterProvider("doppler", &adapters.DopplerAdapter{APIToken: token}, &resilientbridge.ProviderConfig{
		UseProviderLimits:   false,
		MaxRequestsOverride: nil,
		MaxRetries:          3,
		BaseBackoff:         0,
	})

	page := 1
	q := url.Values{}
	q.Set("page", fmt.Sprintf("%d", page))

	req := &resilientbridge.NormalizedRequest{
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

	fmt.Printf("Page: %d\n", usersResp.Page)
	if len(usersResp.WorkplaceUsers) == 0 {
		fmt.Println("No users found.")
		return
	}

	for _, wUser := range usersResp.WorkplaceUsers {
		fmt.Printf("User: %s (%s)\n", wUser.User.Name, wUser.User.Email)
	}
}
