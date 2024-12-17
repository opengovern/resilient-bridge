//
// Usage:
//   go run list_packages.go -scope=github.com/<org>/
//   go run list_packages.go -scope=github.com/<org>/<package>
//   go run list_packages.go -scope=github.com/<org>/<package>:<version>
//
// Description:
//   This program lists and displays details about npm packages hosted in a GitHub organization.
//
//   - If you provide an org-level scope (e.g., github.com/my-org/), it will:
//     1. Fetch all npm packages from the organization (paging through all results).
//     2. For each package, fetch detailed information including version_count, repository, owner details, etc.
//     3. Print each package's details in a nicely formatted JSON, one by one, as they are fetched.
//
//   - If you provide a package-level scope (e.g., github.com/my-org/@my-scope/my-package),
//     it will print detailed information about that single npm package.
//
//   - If you provide a package-level with a specific version (e.g., github.com/my-org/@my-scope/my-package:1.0.0),
//     it will print details relevant to that package version (though currently the code focuses on package details).
//
// Prerequisites:
//   - Set the GITHUB_API_TOKEN environment variable to a GitHub token with read:packages permission.
//
// Examples:
//   go run list_packages.go -scope=github.com/my-org/ > my_org_packages.json
//   go run list_packages.go -scope=github.com/my-org/@my-scope/my-package
//
// The output will be JSON structures with details about the npm packages, formatted for readability.
// Each package will be printed immediately as it is fetched, rather than waiting until the end.
//

package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"

	resilientbridge "github.com/opengovern/resilient-bridge"
	"github.com/opengovern/resilient-bridge/adapters"
)

type OwnerDetail struct {
	Login        string `json:"login"`
	ID           int    `json:"id,omitempty"`
	NodeID       string `json:"node_id,omitempty"`
	HTMLURL      string `json:"html_url,omitempty"`
	Type         string `json:"type,omitempty"`
	UserViewType string `json:"user_view_type,omitempty"`
	SiteAdmin    bool   `json:"site_admin,omitempty"`
}

type RepoOwnerDetail struct {
	Login     string `json:"login"`
	ID        int    `json:"id,omitempty"`
	NodeID    string `json:"node_id,omitempty"`
	HTMLURL   string `json:"html_url,omitempty"`
	Type      string `json:"type,omitempty"`
	SiteAdmin bool   `json:"site_admin,omitempty"`
}

type Repository struct {
	ID          int             `json:"id"`
	NodeID      string          `json:"node_id"`
	Name        string          `json:"name"`
	FullName    string          `json:"full_name"`
	Private     bool            `json:"private"`
	Owner       RepoOwnerDetail `json:"owner"`
	HTMLURL     string          `json:"html_url"`
	Description string          `json:"description"`
	Fork        bool            `json:"fork"`
	URL         string          `json:"url"`
}

type PackageDetail struct {
	ID           int         `json:"id"`
	Name         string      `json:"name"`
	PackageType  string      `json:"package_type"` // "npm"
	Owner        OwnerDetail `json:"owner"`
	VersionCount int         `json:"version_count"`
	Visibility   string      `json:"visibility"`
	URL          string      `json:"url"`
	CreatedAt    string      `json:"created_at"`
	UpdatedAt    string      `json:"updated_at"`
	Repository   Repository  `json:"repository"`
	HTMLURL      string      `json:"html_url"`
}

type PackageListItem struct {
	ID          int    `json:"id"`
	Name        string `json:"name"`
	PackageType string `json:"package_type"` // "npm"
	Visibility  string `json:"visibility"`
	HTMLURL     string `json:"html_url"`
	CreatedAt   string `json:"created_at"`
	UpdatedAt   string `json:"updated_at"`
	Owner       struct {
		Login string `json:"login"`
	} `json:"owner"`
	URL string `json:"url"`
}

func main() {
	scopeFlag := flag.String("scope", "", "Scope: github.com/<org>/, github.com/<org>/<package>, or github.com/<org>/<package>:<version>")
	flag.Parse()

	if *scopeFlag == "" {
		log.Fatal("You must provide a -scope parameter")
	}

	apiToken := os.Getenv("GITHUB_API_TOKEN")
	if apiToken == "" {
		log.Fatal("GITHUB_API_TOKEN environment variable not set or missing read:packages scope")
	}

	sdk := resilientbridge.NewResilientBridge()
	sdk.RegisterProvider("github", adapters.NewGitHubAdapter(apiToken), &resilientbridge.ProviderConfig{
		UseProviderLimits: true,
		MaxRetries:        3,
		BaseBackoff:       0,
	})

	scope := *scopeFlag
	if !strings.HasPrefix(scope, "github.com/") {
		log.Fatal("Scope must start with github.com/")
	}

	parts := strings.Split(strings.TrimPrefix(scope, "github.com/"), "/")
	org := parts[0]

	// If just org-level, we list all npm packages and print each one’s details
	if strings.HasSuffix(scope, "/") {
		// Fetch all packages with pagination
		packages := fetchAllPackages(sdk, org, "npm")

		// For each package, fetch full details and print immediately
		for _, p := range packages {
			fullDetails, err := fetchPackageDetails(sdk, org, "npm", p.Name)
			if err != nil {
				log.Printf("Error fetching details for package %s/%s: %v", org, p.Name, err)
				continue
			}

			printPackageIndented(fullDetails)
		}
		return
	}

	// Otherwise handle package or package:version scope
	lastPart := parts[len(parts)-1]
	refParts := strings.SplitN(lastPart, ":", 2)
	if len(refParts) == 2 {
		// Single version case
		packagePathParts := parts[1 : len(parts)-1]
		packageName := strings.Join(append(packagePathParts, refParts[0]), "/")

		details, err := fetchPackageDetails(sdk, org, "npm", packageName)
		if err != nil {
			log.Fatalf("Error fetching details for package %s/%s: %v", org, packageName, err)
		}
		printPackageIndented(details)
	} else {
		// Package-level scope
		packageName := strings.Join(parts[1:], "/")

		details, err := fetchPackageDetails(sdk, org, "npm", packageName)
		if err != nil {
			log.Fatalf("Error fetching details for package %s/%s: %v", org, packageName, err)
		}
		printPackageIndented(details)
	}
}

func fetchAllPackages(sdk *resilientbridge.ResilientBridge, org, packageType string) []PackageListItem {
	var allPackages []PackageListItem
	page := 1
	perPage := 100 // maximum allowed by the GitHub Packages API

	for {
		endpoint := fmt.Sprintf("/orgs/%s/packages?package_type=%s&page=%d&per_page=%d", org, packageType, page, perPage)
		listReq := &resilientbridge.NormalizedRequest{
			Method:   "GET",
			Endpoint: endpoint,
			Headers:  map[string]string{"Accept": "application/vnd.github+json"},
		}

		listResp, err := sdk.Request("github", listReq)
		if err != nil {
			log.Fatalf("Error listing packages: %v", err)
		}
		if listResp.StatusCode >= 400 {
			log.Fatalf("HTTP error %d: %s", listResp.StatusCode, string(listResp.Data))
		}

		var packages []PackageListItem
		if err := json.Unmarshal(listResp.Data, &packages); err != nil {
			log.Fatalf("Error parsing packages list response: %v", err)
		}

		if len(packages) == 0 {
			break
		}

		allPackages = append(allPackages, packages...)
		page++
	}
	return allPackages
}

func fetchPackageDetails(sdk *resilientbridge.ResilientBridge, org, packageType, packageName string) (PackageDetail, error) {
	var pd PackageDetail
	endpoint := fmt.Sprintf("/orgs/%s/packages/%s/%s", org, packageType, packageName)
	req := &resilientbridge.NormalizedRequest{
		Method:   "GET",
		Endpoint: endpoint,
		Headers:  map[string]string{"Accept": "application/vnd.github+json"},
	}

	resp, err := sdk.Request("github", req)
	if err != nil {
		return pd, fmt.Errorf("error fetching package details: %w", err)
	}
	if resp.StatusCode >= 400 {
		return pd, fmt.Errorf("HTTP error %d: %s", resp.StatusCode, string(resp.Data))
	}

	if err := json.Unmarshal(resp.Data, &pd); err != nil {
		return pd, fmt.Errorf("error parsing package details: %w", err)
	}
	return pd, nil
}

func printPackageIndented(pd PackageDetail) {
	outBytes, err := json.MarshalIndent(pd, "", "  ")
	if err != nil {
		log.Printf("Error marshalling output: %v", err)
		return
	}
	os.Stdout.Write(outBytes)
	os.Stdout.Write([]byte("\n"))
	os.Stdout.Sync() // ensure immediate output
}
