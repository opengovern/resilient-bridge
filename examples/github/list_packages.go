package main

import (
	"encoding/json"
	"flag"
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

type PackageVersion struct {
	ID             int    `json:"id"`
	Name           string `json:"name"`
	URL            string `json:"url"`
	PackageHTMLURL string `json:"package_html_url"`
	CreatedAt      string `json:"created_at"`
	UpdatedAt      string `json:"updated_at"`
	HTMLURL        string `json:"html_url"`
	Metadata       struct {
		Container struct {
			Tags []string `json:"tags"`
		} `json:"container"`
	} `json:"metadata"`
}

func main() {
	// Define flags
	orgFlag := flag.String("org", "opengovern", "GitHub organization name (default: opengovern)")
	typeFlag := flag.String("type", "container", "Package type (npm, maven, rubygems, nuget, container)")
	flag.Parse()

	org := *orgFlag
	packageType := *typeFlag

	apiToken := os.Getenv("GITHUB_API_TOKEN")
	if apiToken == "" {
		log.Fatal("GITHUB_API_TOKEN environment variable not set or missing read:packages scope")
	}

	// Allowed package types: npm, maven, rubygems, nuget, container
	allowedTypes := map[string]bool{"npm": true, "maven": true, "rubygems": true, "nuget": true, "container": true}
	if !allowedTypes[packageType] {
		log.Fatalf("Unsupported package type: %s. Allowed: npm, maven, rubygems, nuget, container", packageType)
	}

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

	// 3. List package versions for that package
	versionsReq := &resilientbridge.NormalizedRequest{
		Method:   "GET",
		Endpoint: fmt.Sprintf("/orgs/%s/packages/%s/%s/versions", org, packageType, packageName),
		Headers:  map[string]string{"Accept": "application/vnd.github+json"},
	}

	versionsResp, err := sdk.Request("github", versionsReq)
	if err != nil {
		log.Fatalf("Error listing package versions: %v", err)
	}
	if versionsResp.StatusCode >= 400 {
		log.Fatalf("HTTP error %d: %s", versionsResp.StatusCode, string(versionsResp.Data))
	}

	var versions []PackageVersion
	if err := json.Unmarshal(versionsResp.Data, &versions); err != nil {
		log.Fatalf("Error parsing package versions response: %v", err)
	}

	// Only show the last 20 versions or fewer if less than 20 are available
	const maxVersionsToShow = 20
	if len(versions) > maxVersionsToShow {
		versions = versions[len(versions)-maxVersionsToShow:]
	}

	fmt.Printf("Showing up to last %d versions (found %d total) for package %s:\n", maxVersionsToShow, len(versions), packageName)
	for _, v := range versions {
		fmt.Printf("- Version ID: %d, Name: %s, CreatedAt: %s, UpdatedAt: %s, HTML: %s\n",
			v.ID, v.Name, v.CreatedAt, v.UpdatedAt, v.HTMLURL)
		if len(v.Metadata.Container.Tags) > 0 {
			fmt.Printf("  Tags: %v\n", v.Metadata.Container.Tags)
		}
	}
}
