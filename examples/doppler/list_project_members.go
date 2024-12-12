// list_project_members.go
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

// Adjust the response struct based on actual API return:
type DopplerProjectMembersResponse struct {
	Members []struct {
		Email string `json:"email"`
		Name  string `json:"name"`
	} `json:"members"`
	Page       int  `json:"page"`
	TotalPages int  `json:"total_pages"`
	Success    bool `json:"success"`
}

func main() {
	token := os.Getenv("YOUR_DOPPLER_API_TOKEN")
	if token == "" {
		log.Fatal("YOUR_DOPPLER_API_TOKEN not set")
	}

	sdk := unifiedsdk.NewUnifiedSDK()
	sdk.RegisterProvider("doppler", &adapters.DopplerAdapter{APIToken: token}, nil)

	projectSlug := "project" // replace with actual project slug
	q := url.Values{}
	q.Set("page", "1")
	q.Set("per_page", "20")

	req := &unifiedsdk.NormalizedRequest{
		Method:   "GET",
		Endpoint: "/v3/projects/" + projectSlug + "/members?" + q.Encode(),
		Headers:  map[string]string{"accept": "application/json"},
	}

	resp, err := sdk.Request("doppler", req)
	if err != nil {
		log.Fatalf("Error listing project members: %v", err)
	}

	if resp.StatusCode >= 400 {
		log.Fatalf("Error %d: %s", resp.StatusCode, string(resp.Data))
	}

	var membersResp DopplerProjectMembersResponse
	if err := json.Unmarshal(resp.Data, &membersResp); err != nil {
		log.Fatalf("Error parsing project members response: %v", err)
	}

	fmt.Printf("Page: %d / %d\n", membersResp.Page, membersResp.TotalPages)
	for _, m := range membersResp.Members {
		fmt.Printf("Member: %s (%s)\n", m.Name, m.Email)
	}
}
