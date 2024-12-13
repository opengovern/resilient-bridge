package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"

	resilientbridge "github.com/opengovern/resilient-bridge"
	"github.com/opengovern/resilient-bridge/adapters"
)

type Owner struct {
	Login string `json:"login"`
}

type Package struct {
	ID          int    `json:"id"`
	Name        string `json:"name"`
	PackageType string `json:"package_type"`
	Visibility  string `json:"visibility"`
	HTMLURL     string `json:"html_url"`
	CreatedAt   string `json:"created_at"`
	UpdatedAt   string `json:"updated_at"`
	Owner       Owner  `json:"owner"`
	URL         string `json:"url"`
}

// This example:
// 1. Lists container packages in an organization
// 2. Retrieves details for one specific package (e.g., first returned package)

func main() {
	apiToken := os.Getenv("GITHUB_API_TOKEN")
	if apiToken == "" {
		log.Fatal("GITHUB_API_TOKEN environment variable not set or missing read:packages scope")
	}

	// The organization name and package type:
	org := "opengovern"        // replace with your organization
	packageType := "container" // or "docker" if you have docker registry images

	sdk := resilientbridge.NewResilientBridge()
	sdk.RegisterProvider("github", adapters.NewGitHubAdapter(apiToken), &resilientbridge.ProviderConfig{
		UseProviderLimits: true,
		MaxRetries:        3,
		BaseBackoff:       0,
	})

	// 1. List packages for the organization of specified type
	listReq := &resilientbridge.NormalizedRequest{
		Method:   "GET",
		Endpoint: fmt.Sprintf("/orgs/%s/packages?package_type=%s", org, packageType),
		Headers:  map[string]string{"Accept": "application/vnd.github+json"},
	}

	listResp, err := sdk.Request("github", listReq)
	if err != nil {
		log.Fatalf("Error listing packages: %v", err)
	}
	if listResp.StatusCode >= 400 {
		log.Fatalf("HTTP error %d: %s", listResp.StatusCode, string(listResp.Data))
	}

	var packages []Package
	if err := json.Unmarshal(listResp.Data, &packages); err != nil {
		log.Fatalf("Error parsing packages list response: %v", err)
	}

	fmt.Printf("Found %d %s packages in organization %s:\n", len(packages), packageType, org)
	for _, p := range packages {
		fmt.Printf("- Name: %s, Type: %s, Visibility: %s, URL: %s\n", p.Name, p.PackageType, p.Visibility, p.HTMLURL)
	}

	if len(packages) == 0 {
		log.Println("No packages found.")
		return
	}

	// 2. Get a specific package (e.g., the first one)
	packageName := packages[0].Name
	getReq := &resilientbridge.NormalizedRequest{
		Method:   "GET",
		Endpoint: fmt.Sprintf("/orgs/%s/packages/%s/%s", org, packageType, packageName),
		Headers:  map[string]string{"Accept": "application/vnd.github+json"},
	}

	getResp, err := sdk.Request("github", getReq)
	if err != nil {
		log.Fatalf("Error getting package details: %v", err)
	}
	if getResp.StatusCode >= 400 {
		log.Fatalf("HTTP error %d: %s", getResp.StatusCode, string(getResp.Data))
	}

	var singlePackage Package
	if err := json.Unmarshal(getResp.Data, &singlePackage); err != nil {
		log.Fatalf("Error parsing package detail response: %v", err)
	}

	fmt.Printf("Package details for %s:\n", singlePackage.Name)
	fmt.Printf("ID: %d, Visibility: %s, CreatedAt: %s, UpdatedAt: %s\n", singlePackage.ID, singlePackage.Visibility, singlePackage.CreatedAt, singlePackage.UpdatedAt)
	fmt.Printf("HTML URL: %s\n", singlePackage.HTMLURL)
}
