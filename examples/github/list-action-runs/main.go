package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/url"
	"os"
	"strings"

	resilientbridge "github.com/opengovern/resilient-bridge"
	"github.com/opengovern/resilient-bridge/adapters"
)

func main() {
	repoFlag := flag.String("repo", "", "Repository in the format https://github.com/<owner>/<repo>")
	branchFlag := flag.String("branch", "", "Branch name (e.g. main)")
	maxRunsFlag := flag.Int("maxruns", 50, "Maximum number of workflow runs to fetch (default 50)")
	flag.Parse()

	if *repoFlag == "" {
		log.Fatal("You must provide a -repo parameter, e.g. -repo=https://github.com/apache/cloudstack")
	}
	if *branchFlag == "" {
		log.Fatal("You must provide a -branch parameter, e.g. -branch=main")
	}

	// Set up the resilient SDK
	apiToken := os.Getenv("GITHUB_API_TOKEN")
	if apiToken == "" {
		log.Println("GITHUB_API_TOKEN not set; you may only be able to access public repos")
	}
	sdk := resilientbridge.NewResilientBridge()
	sdk.RegisterProvider("github", adapters.NewGitHubAdapter(apiToken), &resilientbridge.ProviderConfig{
		UseProviderLimits: true,
		MaxRetries:        3,
		BaseBackoff:       0,
	})

	owner, repo, err := parseRepoURL(*repoFlag)
	if err != nil {
		log.Fatalf("Error parsing repo URL: %v", err)
	}

	active, err := checkRepositoryActive(sdk, owner, repo)
	if err != nil {
		log.Fatalf("Error checking repository: %v", err)
	}

	if !active {
		// Repository is archived or disabled, return no runs
		return
	}

	maxRuns := *maxRunsFlag
	if maxRuns <= 0 {
		maxRuns = 50
	}

	runs, err := fetchWorkflowRuns(sdk, owner, repo, *branchFlag, maxRuns)
	if err != nil {
		log.Fatalf("Error fetching workflow runs: %v", err)
	}

	for _, run := range runs {
		data, err := json.MarshalIndent(run, "", "  ")
		if err != nil {
			log.Printf("Error marshaling run: %v", err)
			continue
		}
		os.Stdout.Write(data)
		os.Stdout.Write([]byte("\n"))
	}
}

func parseRepoURL(repoURL string) (string, string, error) {
	u, err := url.Parse(strings.TrimSpace(repoURL))
	if err != nil {
		return "", "", err
	}
	if u.Host != "github.com" {
		return "", "", fmt.Errorf("URL must be from github.com")
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(parts) < 2 {
		return "", "", fmt.Errorf("URL path must be in format /<owner>/<repo>")
	}
	owner := parts[0]
	repo := parts[1]
	return owner, repo, nil
}

// checkRepositoryActive returns false if the repository is archived or disabled, true otherwise.
func checkRepositoryActive(sdk *resilientbridge.ResilientBridge, owner, repo string) (bool, error) {
	req := &resilientbridge.NormalizedRequest{
		Method:   "GET",
		Endpoint: fmt.Sprintf("/repos/%s/%s", owner, repo),
		Headers:  map[string]string{"Accept": "application/vnd.github+json"},
	}

	resp, err := sdk.Request("github", req)
	if err != nil {
		return false, fmt.Errorf("error checking repository: %w", err)
	}
	if resp.StatusCode == 404 {
		// Repo not found, treat as inactive
		return false, nil
	} else if resp.StatusCode >= 400 {
		return false, fmt.Errorf("HTTP error %d: %s", resp.StatusCode, string(resp.Data))
	}

	var repoInfo struct {
		Archived bool `json:"archived"`
		Disabled bool `json:"disabled"`
	}
	if err := json.Unmarshal(resp.Data, &repoInfo); err != nil {
		return false, fmt.Errorf("error decoding repository info: %w", err)
	}

	if repoInfo.Archived || repoInfo.Disabled {
		return false, nil
	}
	return true, nil
}

// workflowRun represents a GitHub Actions workflow run
type workflowRun struct {
	ID            int    `json:"id"`
	Name          string `json:"name"`
	HeadBranch    string `json:"head_branch"`
	HeadSHA       string `json:"head_sha"`
	Status        string `json:"status"`
	Conclusion    string `json:"conclusion"`
	URL           string `json:"url"`
	HTMLURL       string `json:"html_url"`
	WorkflowID    int    `json:"workflow_id"`
	RunNumber     int    `json:"run_number"`
	Event         string `json:"event"`
	CreatedAt     string `json:"created_at"`
	UpdatedAt     string `json:"updated_at"`
	RunAttempt    int    `json:"run_attempt"`
	RunStartedAt  string `json:"run_started_at"`
	JobsURL       string `json:"jobs_url"`
	LogsURL       string `json:"logs_url"`
	CheckSuiteURL string `json:"check_suite_url"`
	ArtifactsURL  string `json:"artifacts_url"`
}

// workflowRunsResponse is the response format from the GitHub API for listing runs
type workflowRunsResponse struct {
	TotalCount   int           `json:"total_count"`
	WorkflowRuns []workflowRun `json:"workflow_runs"`
}

// fetchWorkflowRuns returns up to maxRuns workflow runs filtered by branch
func fetchWorkflowRuns(sdk *resilientbridge.ResilientBridge, owner, repo, branch string, maxRuns int) ([]workflowRun, error) {
	var allRuns []workflowRun
	perPage := 100
	page := 1

	for len(allRuns) < maxRuns {
		remaining := maxRuns - len(allRuns)
		if remaining < perPage {
			perPage = remaining
		}

		endpoint := fmt.Sprintf("/repos/%s/%s/actions/runs?branch=%s&per_page=%d&page=%d",
			owner, repo, url.QueryEscape(branch), perPage, page)

		req := &resilientbridge.NormalizedRequest{
			Method:   "GET",
			Endpoint: endpoint,
			Headers:  map[string]string{"Accept": "application/vnd.github+json"},
		}

		resp, err := sdk.Request("github", req)
		if err != nil {
			return nil, fmt.Errorf("error fetching workflow runs: %w", err)
		}

		if resp.StatusCode >= 400 {
			return nil, fmt.Errorf("HTTP error %d: %s", resp.StatusCode, string(resp.Data))
		}

		var runsResp workflowRunsResponse
		if err := json.Unmarshal(resp.Data, &runsResp); err != nil {
			return nil, fmt.Errorf("error decoding workflow runs: %w", err)
		}

		if len(runsResp.WorkflowRuns) == 0 {
			// No more runs
			break
		}

		allRuns = append(allRuns, runsResp.WorkflowRuns...)
		if len(allRuns) >= maxRuns {
			break
		}
		page++
	}

	if len(allRuns) > maxRuns {
		allRuns = allRuns[:maxRuns]
	}

	return allRuns, nil
}
