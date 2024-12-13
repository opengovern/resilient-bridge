package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/url"
	"os"
	"sync"

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

	// 1. List Deployments
	deployment, err := listDeployments(sdk)
	if err != nil {
		log.Fatalf("Error listing deployments: %v", err)
	}
	fmt.Printf("Accessing deployment: Slug=%s, Name=%s, ID=%d\n", deployment.Slug, deployment.Name, deployment.ID)

	// 2. List Projects in this deployment
	projects, err := listProjects(sdk, deployment.Slug, 0, 100)
	if err != nil {
		log.Fatalf("Error listing projects: %v", err)
	}
	if len(projects) == 0 {
		log.Println("No projects found in this deployment.")
		return
	}
	fmt.Printf("Found %d projects in deployment %s.\n", len(projects), deployment.Slug)

	// 3. Concurrently get project details
	// We will limit concurrency to, say, 5 workers.
	details := getProjectDetailsConcurrently(sdk, deployment.Slug, projects, 5)

	fmt.Println("Project details retrieved:")
	for _, d := range details {
		fmt.Printf("Name: %s, URL: %s, CreatedAt: %s, LatestScanAt: %s\n",
			d.Name, d.URL, d.CreatedAt, d.LatestScanAt)
	}
}

// listDeployments requests the Semgrep deployments your token can access and returns the first one.
func listDeployments(sdk *resilientbridge.ResilientBridge) (*Deployment, error) {
	req := &resilientbridge.NormalizedRequest{
		Method:   "GET",
		Endpoint: "/deployments",
		Headers:  map[string]string{"Accept": "application/json"},
	}

	resp, err := sdk.Request("semgrep", req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("HTTP error %d: %s", resp.StatusCode, string(resp.Data))
	}

	var depResult DeploymentsResponse
	if err := json.Unmarshal(resp.Data, &depResult); err != nil {
		return nil, fmt.Errorf("error parsing deployments response: %v", err)
	}

	if len(depResult.Deployments) == 0 {
		return nil, fmt.Errorf("no deployments accessible")
	}

	return &depResult.Deployments[0], nil
}

// listProjects lists the projects for the specified deployment.
// page and pageSize determine pagination. Semgrep defaults to page=0 and page_size=100.
func listProjects(sdk *resilientbridge.ResilientBridge, deploymentSlug string, page, pageSize int) ([]Project, error) {
	q := url.Values{}
	q.Set("page", fmt.Sprintf("%d", page))
	q.Set("page_size", fmt.Sprintf("%d", pageSize))

	req := &resilientbridge.NormalizedRequest{
		Method:   "GET",
		Endpoint: fmt.Sprintf("/deployments/%s/projects?%s", deploymentSlug, q.Encode()),
		Headers:  map[string]string{"Accept": "application/json"},
	}

	resp, err := sdk.Request("semgrep", req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("HTTP error %d: %s", resp.StatusCode, string(resp.Data))
	}

	var projResult ProjectsResponse
	if err := json.Unmarshal(resp.Data, &projResult); err != nil {
		return nil, fmt.Errorf("error parsing projects response: %v", err)
	}

	return projResult.Projects, nil
}

// getProjectDetails retrieves details for a single project by name.
func getProjectDetails(sdk *resilientbridge.ResilientBridge, deploymentSlug, projectName string) (*Project, error) {
	req := &resilientbridge.NormalizedRequest{
		Method:   "GET",
		Endpoint: fmt.Sprintf("/deployments/%s/projects/%s", deploymentSlug, url.PathEscape(projectName)),
		Headers:  map[string]string{"Accept": "application/json"},
	}

	resp, err := sdk.Request("semgrep", req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("HTTP error %d: %s", resp.StatusCode, string(resp.Data))
	}

	var singleProj SingleProjectResponse
	if err := json.Unmarshal(resp.Data, &singleProj); err != nil {
		return nil, fmt.Errorf("error parsing single project detail response: %v", err)
	}

	return &singleProj.Project, nil
}

// getProjectDetailsConcurrently fetches details for a slice of projects concurrently.
// concurrency sets how many goroutines run at once to fetch details.
func getProjectDetailsConcurrently(sdk *resilientbridge.ResilientBridge, deploymentSlug string, projects []Project, concurrency int) []Project {
	details := make([]Project, len(projects))
	jobs := make(chan int, len(projects))
	results := make(chan struct {
		index  int
		detail *Project
		err    error
	}, len(projects))

	// Start worker pool
	var wg sync.WaitGroup
	wg.Add(concurrency)

	for w := 0; w < concurrency; w++ {
		go func() {
			defer wg.Done()
			for i := range jobs {
				p := projects[i]
				detail, err := getProjectDetails(sdk, deploymentSlug, p.Name)
				results <- struct {
					index  int
					detail *Project
					err    error
				}{i, detail, err}
			}
		}()
	}

	for i := range projects {
		jobs <- i
	}
	close(jobs)

	// Wait for all results
	go func() {
		wg.Wait()
		close(results)
	}()

	for r := range results {
		if r.err != nil {
			log.Printf("Error getting details for project %s: %v", projects[r.index].Name, r.err)
			continue
		}
		details[r.index] = *r.detail
	}

	return details
}
