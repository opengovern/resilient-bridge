package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
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

	owner, repo, err := parseRepoURL(*repoFlag)
	if err != nil {
		log.Fatalf("Error parsing repo URL: %v", err)
	}

	active, err := checkRepositoryActive(owner, repo)
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

	runs, err := fetchWorkflowRuns(owner, repo, *branchFlag, maxRuns)
	if err != nil {
		log.Fatalf("Error fetching workflow runs: %v", err)
	}

	for _, run := range runs {
		// Print each run as a JSON object
		data, err := json.Marshal(run)
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

type repoInfo struct {
	Archived bool `json:"archived"`
	Disabled bool `json:"disabled"`
}

// checkRepositoryActive returns false if the repository is archived or disabled, true otherwise.
func checkRepositoryActive(owner, repo string) (bool, error) {
	u := fmt.Sprintf("https://api.github.com/repos/%s/%s", owner, repo)
	req, err := http.NewRequest("GET", u, nil)
	if err != nil {
		return false, fmt.Errorf("error creating request to check repo: %w", err)
	}
	req.Header.Set("Accept", "application/vnd.github+json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return false, fmt.Errorf("error checking repository: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == 404 {
		// Repo not found, treat as inactive
		return false, nil
	} else if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return false, fmt.Errorf("HTTP error %d: %s", resp.StatusCode, string(body))
	}

	var info repoInfo
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		return false, fmt.Errorf("error decoding repository info: %w", err)
	}

	// If archived or disabled, mark as inactive
	if info.Archived || info.Disabled {
		return false, nil
	}
	return true, nil
}

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

// response structure for listing workflow runs
type workflowRunsResponse struct {
	TotalCount   int           `json:"total_count"`
	WorkflowRuns []workflowRun `json:"workflow_runs"`
}

// fetchWorkflowRuns returns up to maxRuns workflow runs filtered by branch
func fetchWorkflowRuns(owner, repo, branch string, maxRuns int) ([]workflowRun, error) {
	var allRuns []workflowRun
	perPage := 100
	page := 1
	client := &http.Client{}

	for len(allRuns) < maxRuns {
		remaining := maxRuns - len(allRuns)
		if remaining < perPage {
			perPage = remaining
		}

		u := fmt.Sprintf("https://api.github.com/repos/%s/%s/actions/runs?branch=%s&per_page=%d&page=%d",
			owner, repo, url.QueryEscape(branch), perPage, page)

		req, err := http.NewRequest("GET", u, nil)
		if err != nil {
			return nil, fmt.Errorf("error creating request: %w", err)
		}
		req.Header.Set("Accept", "application/vnd.github+json")

		resp, err := client.Do(req)
		if err != nil {
			return nil, fmt.Errorf("error fetching workflow runs: %w", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode >= 400 {
			body, _ := io.ReadAll(resp.Body)
			return nil, fmt.Errorf("HTTP error %d: %s", resp.StatusCode, string(body))
		}

		var runsResp workflowRunsResponse
		err = json.NewDecoder(resp.Body).Decode(&runsResp)
		if err != nil {
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
