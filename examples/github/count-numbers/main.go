package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/url"
	"os"
	"regexp"
	"strings"

	resilientbridge "github.com/opengovern/resilient-bridge"
	"github.com/opengovern/resilient-bridge/adapters"
)

func main() {
	repoFlag := flag.String("repo", "", "Repository in the format https://github.com/<owner>/<repo>/")
	flag.Parse()

	if *repoFlag == "" {
		log.Fatal("You must provide a -repo parameter, e.g. -repo=https://github.com/apache/airflow/")
	}

	apiToken := os.Getenv("GITHUB_API_TOKEN")
	if apiToken == "" {
		log.Println("GITHUB_API_TOKEN environment variable not set. You may access public repos without it, or set a token if needed.")
	}

	owner, repoName, err := parseRepoURL(*repoFlag)
	if err != nil {
		log.Fatalf("Error parsing repo URL: %v", err)
	}

	// Initialize resilient-bridge
	sdk := resilientbridge.NewResilientBridge()
	sdk.RegisterProvider("github", adapters.NewGitHubAdapter(apiToken), &resilientbridge.ProviderConfig{
		UseProviderLimits: true,
		MaxRetries:        3,
		BaseBackoff:       0,
	})

	// Count commits (as before)
	commitsCount, err := countCommits(sdk, owner, repoName)
	if err != nil {
		log.Printf("Error counting commits: %v", err)
	} else {
		fmt.Printf("Total commits in %s/%s: %d\n", owner, repoName, commitsCount)
	}

	// Count issues
	issuesCount, err := countIssues(sdk, owner, repoName)
	if err != nil {
		log.Printf("Error counting issues: %v", err)
	} else {
		fmt.Printf("Total issues in %s/%s: %d\n", owner, repoName, issuesCount)
	}

	// Count branches
	branchesCount, err := countBranches(sdk, owner, repoName)
	if err != nil {
		log.Printf("Error counting branches: %v", err)
	} else {
		fmt.Printf("Total branches in %s/%s: %d\n", owner, repoName, branchesCount)
	}

	// Count pull requests
	prCount, err := countPullRequests(sdk, owner, repoName)
	if err != nil {
		log.Printf("Error counting PRs: %v", err)
	} else {
		fmt.Printf("Total PRs in %s/%s: %d\n", owner, repoName, prCount)
	}
}

func countCommits(sdk *resilientbridge.ResilientBridge, owner, repoName string) (int, error) {
	// per_page=1 so last page number = total commits
	endpoint := fmt.Sprintf("/repos/%s/%s/commits?sha=main&per_page=1", owner, repoName)
	return countItemsFromEndpoint(sdk, endpoint)
}

func countIssues(sdk *resilientbridge.ResilientBridge, owner, repoName string) (int, error) {
	// per_page=1 so last page number = total issues
	// state=all to count all issues (including open and closed)
	endpoint := fmt.Sprintf("/repos/%s/%s/issues?state=all&per_page=1", owner, repoName)
	return countItemsFromEndpoint(sdk, endpoint)
}

func countBranches(sdk *resilientbridge.ResilientBridge, owner, repoName string) (int, error) {
	// per_page=1 so last page number = total branches
	endpoint := fmt.Sprintf("/repos/%s/%s/branches?per_page=1", owner, repoName)
	return countItemsFromEndpoint(sdk, endpoint)
}

func countPullRequests(sdk *resilientbridge.ResilientBridge, owner, repoName string) (int, error) {
	// per_page=1 so last page number = total PRs
	// state=all to count all PRs (open, closed, merged)
	endpoint := fmt.Sprintf("/repos/%s/%s/pulls?state=all&per_page=1", owner, repoName)
	return countItemsFromEndpoint(sdk, endpoint)
}

func countItemsFromEndpoint(sdk *resilientbridge.ResilientBridge, endpoint string) (int, error) {
	req := &resilientbridge.NormalizedRequest{
		Method:   "GET",
		Endpoint: endpoint,
		Headers:  map[string]string{"Accept": "application/vnd.github+json"},
	}

	resp, err := sdk.Request("github", req)
	if err != nil {
		return 0, fmt.Errorf("error fetching data: %w", err)
	}

	if resp.StatusCode >= 400 {
		return 0, fmt.Errorf("HTTP error %d: %s", resp.StatusCode, string(resp.Data))
	}

	// Find Link header (case-insensitive)
	var linkHeader string
	for k, v := range resp.Headers {
		if strings.ToLower(k) == "link" {
			linkHeader = v
			break
		}
	}

	if linkHeader == "" {
		// No Link header: possibly a single page of results.
		// Check if we got any item by looking at resp.Data.
		// If endpoint returns a JSON array, check length > 2 means not empty array "[]".
		if len(resp.Data) > 2 {
			// Parse the array and ensure it's at least one item.
			var items []interface{}
			if err := json.Unmarshal(resp.Data, &items); err != nil {
				// If parsing fails, assume at least one item since len(resp.Data)>2
				return 1, nil
			}
			return len(items), nil
		}
		return 0, nil
	}

	// Parse last page
	lastPage, err := parseLastPage(linkHeader)
	if err != nil {
		return 0, fmt.Errorf("could not parse last page: %w", err)
	}

	// per_page=1, so lastPage = total items
	return lastPage, nil
}

// parseLastPage extracts the page number from the `rel="last"` link in the Link header.
func parseLastPage(linkHeader string) (int, error) {
	re := regexp.MustCompile(`page=(\d+)>; rel="last"`)
	matches := re.FindStringSubmatch(linkHeader)
	if len(matches) < 2 {
		// If no last page found, possibly only one page of results.
		return 1, nil
	}
	var lastPage int
	_, err := fmt.Sscanf(matches[1], "%d", &lastPage)
	if err != nil {
		return 0, err
	}
	return lastPage, nil
}

// parseRepoURL extracts the owner and repo name from a GitHub URL.
// Expected format: https://github.com/<owner>/<repo>
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
		return "", "", fmt.Errorf("URL path must be in the format /<owner>/<repo>")
	}

	return parts[0], parts[1], nil
}
