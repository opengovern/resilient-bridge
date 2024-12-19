package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/url"
	"os"
	"strconv"
	"strings"

	resilientbridge "github.com/opengovern/resilient-bridge"
	"github.com/opengovern/resilient-bridge/adapters"
)

func main() {
	repoFlag := flag.String("repo", "", "Repository in the format https://github.com/<owner>/<repo>")
	branchFlag := flag.String("branch", "", "Branch name (e.g. main). Leave empty to fetch all runs.")
	maxRunsFlag := flag.Int("maxruns", 50, "Maximum number of workflow runs to fetch (default 50)")
	runNumberFlag := flag.String("run_number", "", "Specify run numbers or ranges, e.g. 23,25 or 23-56 or 23")
	flag.Parse()

	if *repoFlag == "" {
		log.Fatal("You must provide a -repo parameter, e.g. -repo=https://github.com/apache/cloudstack")
	}

	// Parse the run_number flag
	runNumbers := parseRunNumberFlag(*runNumberFlag)

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

	// If runNumbers is specified, filter runs by run_number
	if len(runNumbers) > 0 {
		runs = filterRunsByNumber(runs, runNumbers)
		if len(runs) == 0 {
			log.Println("No runs found matching the specified run_number criteria.")
			return
		}
	}

	for _, runBasic := range runs {
		runDetail, err := fetchRunDetails(sdk, owner, repo, runBasic.ID)
		if err != nil {
			log.Printf("Error fetching details for run %d: %v", runBasic.ID, err)
			continue
		}

		artifactCount, artifacts, err := fetchArtifactsForRun(sdk, owner, repo, runBasic.ID)
		if err != nil {
			log.Printf("Error fetching artifacts for run %d: %v", runBasic.ID, err)
			continue
		}
		runDetail.ArtifactCount = artifactCount
		runDetail.Artifacts = artifacts

		data, err := json.MarshalIndent(runDetail, "", "  ")
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

// parseRunNumberFlag parses the run_number flag.
// It handles:
// - Single run number: "23"
// - Comma-separated: "23,25"
// - Range: "23-56"
//
// The result is returned as a slice of runNumberCriterion, which can represent either single values or ranges.
type runNumberCriterion struct {
	From int
	To   int
}

func parseRunNumberFlag(flagVal string) []runNumberCriterion {
	if strings.TrimSpace(flagVal) == "" {
		return nil
	}

	parts := strings.Split(flagVal, ",")
	var criteria []runNumberCriterion
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if strings.Contains(p, "-") {
			rangeParts := strings.SplitN(p, "-", 2)
			if len(rangeParts) == 2 {
				startStr := strings.TrimSpace(rangeParts[0])
				endStr := strings.TrimSpace(rangeParts[1])
				start, err1 := strconv.Atoi(startStr)
				end, err2 := strconv.Atoi(endStr)
				if err1 == nil && err2 == nil && start <= end {
					criteria = append(criteria, runNumberCriterion{From: start, To: end})
				}
			}
		} else {
			// Single number
			n, err := strconv.Atoi(p)
			if err == nil {
				criteria = append(criteria, runNumberCriterion{From: n, To: n})
			}
		}
	}
	return criteria
}

// filterRunsByNumber filters the given runs to include only those that match the runNumberCriterion(s).
func filterRunsByNumber(runs []workflowRun, criteria []runNumberCriterion) []workflowRun {
	var filtered []workflowRun

	for _, run := range runs {
		if runNumberMatches(run.RunNumber, criteria) {
			filtered = append(filtered, run)
		}
	}
	return filtered
}

func runNumberMatches(runNum int, criteria []runNumberCriterion) bool {
	for _, c := range criteria {
		if runNum >= c.From && runNum <= c.To {
			return true
		}
	}
	return false
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

type simpleActor struct {
	Login  string `json:"login"`
	ID     int    `json:"id"`
	NodeID string `json:"node_id"`
	Type   string `json:"type"`
}

type simpleRepo struct {
	ID     int    `json:"id"`
	NodeID string `json:"node_id"`
}

type commitRef struct {
	ID string `json:"id"`
}

type workflowRun struct {
	ID                  int                `json:"id"`
	Name                string             `json:"name"`
	HeadBranch          string             `json:"head_branch"`
	HeadSHA             string             `json:"head_sha"`
	Status              string             `json:"status"`
	Conclusion          string             `json:"conclusion"`
	HTMLURL             string             `json:"html_url"`
	WorkflowID          int                `json:"workflow_id"`
	RunNumber           int                `json:"run_number"`
	Event               string             `json:"event"`
	CreatedAt           string             `json:"created_at"`
	UpdatedAt           string             `json:"updated_at"`
	RunAttempt          int                `json:"run_attempt"`
	RunStartedAt        string             `json:"run_started_at"`
	Actor               *simpleActor       `json:"triggering_actor,omitempty"`
	HeadCommit          *commitRef         `json:"head_commit,omitempty"`
	Repository          *simpleRepo        `json:"repository,omitempty"`
	HeadRepository      *simpleRepo        `json:"head_repository,omitempty"`
	ReferencedWorkflows []interface{}      `json:"referenced_workflows,omitempty"`
	ArtifactCount       int                `json:"artifact_count"`
	Artifacts           []workflowArtifact `json:"artifacts,omitempty"`
}

type workflowRunsResponse struct {
	TotalCount   int           `json:"total_count"`
	WorkflowRuns []workflowRun `json:"workflow_runs"`
}

type workflowArtifact struct {
	ID                 int    `json:"id"`
	NodeID             string `json:"node_id"`
	Name               string `json:"name"`
	SizeInBytes        int    `json:"size_in_bytes"`
	URL                string `json:"url"`
	ArchiveDownloadURL string `json:"archive_download_url"`
	Expired            bool   `json:"expired"`
	CreatedAt          string `json:"created_at"`
	UpdatedAt          string `json:"updated_at"`
	ExpiresAt          string `json:"expires_at"`
}

type artifactsResponse struct {
	TotalCount int                `json:"total_count"`
	Artifacts  []workflowArtifact `json:"artifacts"`
}

// fetchWorkflowRuns returns up to maxRuns workflow runs. If branch is specified, filter by that branch, otherwise fetch all.
func fetchWorkflowRuns(sdk *resilientbridge.ResilientBridge, owner, repo, branch string, maxRuns int) ([]workflowRun, error) {
	var allRuns []workflowRun
	perPage := 100
	page := 1

	for len(allRuns) < maxRuns {
		remaining := maxRuns - len(allRuns)
		if remaining < perPage {
			perPage = remaining
		}

		params := url.Values{}
		params.Set("per_page", fmt.Sprintf("%d", perPage))
		params.Set("page", fmt.Sprintf("%d", page))

		// If branch is provided, add it to the query params
		if branch != "" {
			params.Set("branch", branch)
		}

		endpoint := fmt.Sprintf("/repos/%s/%s/actions/runs?%s", owner, repo, params.Encode())

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

// fetchRunDetails fetches the full details for a specific run, including actor, repository info, and referenced_workflows.
func fetchRunDetails(sdk *resilientbridge.ResilientBridge, owner, repo string, runID int) (workflowRun, error) {
	endpoint := fmt.Sprintf("/repos/%s/%s/actions/runs/%d", owner, repo, runID)
	req := &resilientbridge.NormalizedRequest{
		Method:   "GET",
		Endpoint: endpoint,
		Headers:  map[string]string{"Accept": "application/vnd.github+json"},
	}

	resp, err := sdk.Request("github", req)
	if err != nil {
		return workflowRun{}, fmt.Errorf("error fetching run details: %w", err)
	}

	if resp.StatusCode >= 400 {
		return workflowRun{}, fmt.Errorf("HTTP error %d: %s", resp.StatusCode, string(resp.Data))
	}

	var fullDetail struct {
		ID                  int           `json:"id"`
		Name                string        `json:"name"`
		HeadBranch          string        `json:"head_branch"`
		HeadSHA             string        `json:"head_sha"`
		Status              string        `json:"status"`
		Conclusion          string        `json:"conclusion"`
		HTMLURL             string        `json:"html_url"`
		WorkflowID          int           `json:"workflow_id"`
		RunNumber           int           `json:"run_number"`
		Event               string        `json:"event"`
		CreatedAt           string        `json:"created_at"`
		UpdatedAt           string        `json:"updated_at"`
		RunAttempt          int           `json:"run_attempt"`
		RunStartedAt        string        `json:"run_started_at"`
		Actor               *simpleActor  `json:"actor"`
		HeadCommit          *commitRef    `json:"head_commit"`
		Repository          *simpleRepo   `json:"repository"`
		HeadRepository      *simpleRepo   `json:"head_repository"`
		ReferencedWorkflows []interface{} `json:"referenced_workflows"`
	}

	if err := json.Unmarshal(resp.Data, &fullDetail); err != nil {
		return workflowRun{}, fmt.Errorf("error decoding run details: %w", err)
	}

	return workflowRun{
		ID:                  fullDetail.ID,
		Name:                fullDetail.Name,
		HeadBranch:          fullDetail.HeadBranch,
		HeadSHA:             fullDetail.HeadSHA,
		Status:              fullDetail.Status,
		Conclusion:          fullDetail.Conclusion,
		HTMLURL:             fullDetail.HTMLURL,
		WorkflowID:          fullDetail.WorkflowID,
		RunNumber:           fullDetail.RunNumber,
		Event:               fullDetail.Event,
		CreatedAt:           fullDetail.CreatedAt,
		UpdatedAt:           fullDetail.UpdatedAt,
		RunAttempt:          fullDetail.RunAttempt,
		RunStartedAt:        fullDetail.RunStartedAt,
		Actor:               fullDetail.Actor,
		HeadCommit:          fullDetail.HeadCommit,
		Repository:          fullDetail.Repository,
		HeadRepository:      fullDetail.HeadRepository,
		ReferencedWorkflows: fullDetail.ReferencedWorkflows,
	}, nil
}

func fetchArtifactsForRun(sdk *resilientbridge.ResilientBridge, owner, repo string, runID int) (int, []workflowArtifact, error) {
	endpoint := fmt.Sprintf("/repos/%s/%s/actions/runs/%d/artifacts", owner, repo, runID)
	req := &resilientbridge.NormalizedRequest{
		Method:   "GET",
		Endpoint: endpoint,
		Headers:  map[string]string{"Accept": "application/vnd.github+json"},
	}

	resp, err := sdk.Request("github", req)
	if err != nil {
		return 0, nil, fmt.Errorf("error fetching artifacts: %w", err)
	}

	if resp.StatusCode >= 400 {
		return 0, nil, fmt.Errorf("HTTP error %d: %s", resp.StatusCode, string(resp.Data))
	}

	var artResp artifactsResponse
	if err := json.Unmarshal(resp.Data, &artResp); err != nil {
		return 0, nil, fmt.Errorf("error decoding artifacts response: %w", err)
	}

	return artResp.TotalCount, artResp.Artifacts, nil
}
