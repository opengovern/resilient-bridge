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

type Deployment struct {
	Slug     string `json:"slug"`
	ID       int    `json:"id"`
	Name     string `json:"name"`
	Findings struct {
		URL string `json:"url"`
	} `json:"findings"`
}

type DeploymentsResponse struct {
	Deployments []Deployment `json:"deployments"`
}

type Project struct {
	ID            int      `json:"id"`
	Name          string   `json:"name"`
	URL           string   `json:"url"`
	Tags          []string `json:"tags"`
	CreatedAt     string   `json:"created_at"`
	LatestScanAt  string   `json:"latest_scan_at"`
	PrimaryBranch string   `json:"primary_branch"`
	DefaultBranch string   `json:"default_branch"`
}

type ProjectsResponse struct {
	Projects []Project `json:"projects"`
}

type SingleProjectResponse struct {
	Project Project `json:"project"`
}

func main() {
	apiToken := os.Getenv("SEMGREP_API_TOKEN")
	if apiToken == "" {
		log.Fatal("SEMGREP_API_TOKEN environment variable not set")
	}

	// Optional: Enable debug mode if needed
	debugMode := false
	if val := os.Getenv("DEBUG"); val == "true" {
		debugMode = true
	}

	// Set up SDK and register Semgrep provider
	sdk := resilientbridge.NewResilientBridge()
	sdk.SetDebug(debugMode)
	sdk.RegisterProvider("semgrep", adapters.NewSemgrepAdapter(apiToken), &resilientbridge.ProviderConfig{
		UseProviderLimits: true,
		MaxRetries:        3,
		BaseBackoff:       0,
	})

	// 1. Get Deployments
	depReq := &resilientbridge.NormalizedRequest{
		Method:   "GET",
		Endpoint: "/deployments",
		Headers:  map[string]string{"Accept": "application/json"},
	}

	depResp, err := sdk.Request("semgrep", depReq)
	if err != nil {
		log.Fatalf("Error requesting deployments: %v", err)
	}

	if depResp.StatusCode >= 400 {
		log.Fatalf("Error %d: %s", depResp.StatusCode, string(depResp.Data))
	}

	var depResult DeploymentsResponse
	if err := json.Unmarshal(depResp.Data, &depResult); err != nil {
		log.Fatalf("Error parsing deployments response: %v", err)
	}

	if len(depResult.Deployments) == 0 {
		log.Println("No deployments accessible.")
		return
	}

	deployment := depResult.Deployments[0]
	fmt.Printf("Accessing deployment: Slug=%s, Name=%s, ID=%d\n", deployment.Slug, deployment.Name, deployment.ID)

	// 2. List Projects in the deployment
	page := 0
	pageSize := 100
	q := url.Values{}
	q.Set("page", fmt.Sprintf("%d", page))
	q.Set("page_size", fmt.Sprintf("%d", pageSize))

	projReq := &resilientbridge.NormalizedRequest{
		Method:   "GET",
		Endpoint: fmt.Sprintf("/deployments/%s/projects?%s", deployment.Slug, q.Encode()),
		Headers:  map[string]string{"Accept": "application/json"},
	}

	projResp, err := sdk.Request("semgrep", projReq)
	if err != nil {
		log.Fatalf("Error listing projects: %v", err)
	}

	if projResp.StatusCode >= 400 {
		log.Fatalf("Error %d: %s", projResp.StatusCode, string(projResp.Data))
	}

	var projResult ProjectsResponse
	if err := json.Unmarshal(projResp.Data, &projResult); err != nil {
		log.Fatalf("Error parsing projects response: %v", err)
	}

	if len(projResult.Projects) == 0 {
		log.Println("No projects found in this deployment.")
		return
	}

	fmt.Printf("Found %d projects in deployment %s:\n", len(projResult.Projects), deployment.Slug)
	for _, p := range projResult.Projects {
		fmt.Printf("- ID: %d, Name: %s, URL: %s, Tags: %v, Created: %s\n", p.ID, p.Name, p.URL, p.Tags, p.CreatedAt)
	}

	// 3. Get details for a specific project
	// Let's take the first project
	targetProject := projResult.Projects[0].Name
	fmt.Printf("Retrieving details for project: %s\n", targetProject)

	detailReq := &resilientbridge.NormalizedRequest{
		Method:   "GET",
		Endpoint: fmt.Sprintf("/deployments/%s/projects/%s", deployment.Slug, url.PathEscape(targetProject)),
		Headers:  map[string]string{"Accept": "application/json"},
	}

	detailResp, err := sdk.Request("semgrep", detailReq)
	if err != nil {
		log.Fatalf("Error getting project details: %v", err)
	}

	if detailResp.StatusCode >= 400 {
		log.Fatalf("Error %d: %s", detailResp.StatusCode, string(detailResp.Data))
	}

	var singleProj SingleProjectResponse
	if err := json.Unmarshal(detailResp.Data, &singleProj); err != nil {
		log.Fatalf("Error parsing single project detail response: %v", err)
	}

	p := singleProj.Project
	fmt.Printf("Project details:\n")
	fmt.Printf("ID: %d, Name: %s, URL: %s, Tags: %v, Created: %s, Latest Scan: %s, Primary Branch: %s, Default Branch: %s\n",
		p.ID, p.Name, p.URL, p.Tags, p.CreatedAt, p.LatestScanAt, p.PrimaryBranch, p.DefaultBranch)
}
