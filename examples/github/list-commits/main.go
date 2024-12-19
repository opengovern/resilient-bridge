package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"

	"net/url"

	resilientbridge "github.com/opengovern/resilient-bridge"
	"github.com/opengovern/resilient-bridge/adapters"
)

func main() {
	repoFlag := flag.String("repo", "", "Repository in the format https://github.com/<owner>/<repo>")
	maxCommitsFlag := flag.Int("maxcommits", 250, "Maximum number of commits to fetch (default 250)")
	flag.Parse()

	if *repoFlag == "" {
		log.Fatal("You must provide a -repo parameter, e.g. -repo=https://github.com/apache/cloudstack")
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
		// Repository is archived or disabled, return 0 commits
		return
	}

	maxCommits := *maxCommitsFlag
	if maxCommits <= 0 {
		maxCommits = 100
	}

	commits, err := fetchCommitList(sdk, owner, repo, maxCommits)
	if err != nil {
		log.Fatalf("Error fetching commits list: %v", err)
	}

	for _, c := range commits {
		commitJSON, err := fetchCommitDetails(sdk, owner, repo, c.SHA)
		if err != nil {
			log.Printf("Error fetching commit %s details: %v", c.SHA, err)
			continue
		}
		os.Stdout.Write(commitJSON)
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

type commitRef struct {
	SHA string `json:"sha"`
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

// fetchCommitList returns up to maxCommits commit references from the repo’s default branch.
func fetchCommitList(sdk *resilientbridge.ResilientBridge, owner, repo string, maxCommits int) ([]commitRef, error) {
	var allCommits []commitRef
	perPage := 100
	page := 1

	for len(allCommits) < maxCommits {
		remaining := maxCommits - len(allCommits)
		if remaining < perPage {
			perPage = remaining
		}

		endpoint := fmt.Sprintf("/repos/%s/%s/commits?per_page=%d&page=%d", owner, repo, perPage, page)
		req := &resilientbridge.NormalizedRequest{
			Method:   "GET",
			Endpoint: endpoint,
			Headers:  map[string]string{"Accept": "application/vnd.github+json"},
		}

		resp, err := sdk.Request("github", req)
		if err != nil {
			return nil, fmt.Errorf("error fetching commits: %w", err)
		}

		if resp.StatusCode >= 400 {
			return nil, fmt.Errorf("HTTP error %d: %s", resp.StatusCode, string(resp.Data))
		}

		var commits []commitRef
		if err := json.Unmarshal(resp.Data, &commits); err != nil {
			return nil, fmt.Errorf("error decoding commit list: %w", err)
		}

		if len(commits) == 0 {
			// No more commits
			break
		}

		allCommits = append(allCommits, commits...)
		if len(allCommits) >= maxCommits {
			break
		}
		page++
	}

	if len(allCommits) > maxCommits {
		allCommits = allCommits[:maxCommits]
	}

	return allCommits, nil
}

func fetchCommitDetails(sdk *resilientbridge.ResilientBridge, owner, repo, sha string) ([]byte, error) {
	// Fetch the commit details
	req := &resilientbridge.NormalizedRequest{
		Method:   "GET",
		Endpoint: fmt.Sprintf("/repos/%s/%s/commits/%s", owner, repo, sha),
		Headers:  map[string]string{"Accept": "application/vnd.github+json"},
	}
	resp, err := sdk.Request("github", req)
	if err != nil {
		return nil, fmt.Errorf("error fetching commit details: %w", err)
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("HTTP error %d: %s", resp.StatusCode, string(resp.Data))
	}

	var commitData map[string]interface{}
	if err := json.Unmarshal(resp.Data, &commitData); err != nil {
		return nil, fmt.Errorf("error unmarshaling commit details: %w", err)
	}

	// Extract top-level fields
	commitSha, _ := commitData["sha"].(string)
	htmlURL, _ := commitData["html_url"].(string)

	commitSection, _ := commitData["commit"].(map[string]interface{})
	message, _ := commitSection["message"].(string)

	// date from commit.author.date
	var date string
	if commitAuthor, ok := commitSection["author"].(map[string]interface{}); ok {
		date, _ = commitAuthor["date"].(string)
	}

	// stats
	stats, _ := commitData["stats"].(map[string]interface{})
	additions, _ := stats["additions"].(float64)
	deletions, _ := stats["deletions"].(float64)
	total, _ := stats["total"].(float64)

	// author info
	authorObj := make(map[string]interface{})
	// From commit.author
	if commitAuthor, ok := commitSection["author"].(map[string]interface{}); ok {
		if email, ok := commitAuthor["email"].(string); ok {
			authorObj["email"] = email
		}
		if name, ok := commitAuthor["name"].(string); ok {
			authorObj["name"] = name
		}
	}

	// From top-level author
	if topAuthor, ok := commitData["author"].(map[string]interface{}); ok {
		if login, ok := topAuthor["login"].(string); ok {
			authorObj["login"] = login
		}
		if id, ok := topAuthor["id"].(float64); ok {
			authorObj["id"] = int(id)
		}
		if node, ok := topAuthor["node_id"].(string); ok {
			authorObj["node_id"] = node
		}
		if html, ok := topAuthor["html_url"].(string); ok {
			authorObj["html_url"] = html
		}
		if t, ok := topAuthor["type"].(string); ok {
			authorObj["type"] = t
		}
	}

	// files
	filesArray := []interface{}{}
	if files, ok := commitData["files"].([]interface{}); ok {
		for _, f := range files {
			if fm, ok := f.(map[string]interface{}); ok {
				newFile := map[string]interface{}{}
				if additionsVal, ok := fm["additions"].(float64); ok {
					newFile["additions"] = int(additionsVal)
				}
				if changesVal, ok := fm["changes"].(float64); ok {
					newFile["changes"] = int(changesVal)
				}
				if deletionsVal, ok := fm["deletions"].(float64); ok {
					newFile["deletions"] = int(deletionsVal)
				}
				if filename, ok := fm["filename"].(string); ok {
					newFile["filename"] = filename
				}
				if shaVal, ok := fm["sha"].(string); ok {
					newFile["sha"] = shaVal
				}
				if status, ok := fm["status"].(string); ok {
					newFile["status"] = status
				}
				filesArray = append(filesArray, newFile)
			}
		}
	}

	// parents
	parentsArray := []interface{}{}
	if parents, ok := commitData["parents"].([]interface{}); ok {
		for _, p := range parents {
			if pm, ok := p.(map[string]interface{}); ok {
				newParent := map[string]interface{}{}
				if shaVal, ok := pm["sha"].(string); ok {
					newParent["sha"] = shaVal
				}
				parentsArray = append(parentsArray, newParent)
			}
		}
	}

	// Branch name
	branchName, berr := findBranchByCommit(sdk, owner, repo, commitSha)
	if berr != nil {
		branchName = ""
	}

	// comment_count
	commentCount := 0
	if cc, ok := commitSection["comment_count"].(float64); ok {
		commentCount = int(cc)
	}

	// tree (only sha)
	var treeObj map[string]interface{}
	if tree, ok := commitSection["tree"].(map[string]interface{}); ok {
		if tsha, ok := tree["sha"].(string); ok {
			treeObj = map[string]interface{}{
				"sha": tsha,
			}
		}
	}

	// verification details
	var isVerified bool
	var verificationDetails map[string]interface{}
	if verification, ok := commitSection["verification"].(map[string]interface{}); ok {
		if verified, ok := verification["verified"].(bool); ok {
			isVerified = verified
		}
		reason, _ := verification["reason"].(string)
		signature, _ := verification["signature"].(string)
		verifiedAt, _ := verification["verified_at"].(string)
		verificationDetails = map[string]interface{}{
			"reason":      reason,
			"signature":   signature,
			"verified_at": verifiedAt,
		}
	}

	// metadata
	nodeID, _ := commitData["node_id"].(string)
	metadataObj := map[string]interface{}{
		"node_id":              nodeID,
		"parents":              parentsArray,
		"tree":                 treeObj,
		"verification_details": verificationDetails,
	}

	// target
	targetObj := map[string]interface{}{
		"branch":       branchName,
		"organization": owner,
		"repository":   repo,
	}

	// Fetch associated pull requests
	prs, err := fetchPullRequestsForCommit(sdk, owner, repo, commitSha)
	if err != nil {
		// If there's an error, we just have an empty array
		prs = []int{}
	}

	// Construct final output with exact order of fields:
	output := map[string]interface{}{
		"id":          commitSha,
		"date":        date,
		"message":     message,
		"html_url":    htmlURL,
		"target":      targetObj,
		"is_verified": isVerified,
		"author":      authorObj,
		"changes": map[string]interface{}{
			"additions": int(additions),
			"deletions": int(deletions),
			"total":     int(total),
		},
		"comment_count": commentCount,
		"metadata":      metadataObj,
		"files":         filesArray,
		"pull_requests": prs,
	}

	modifiedData, err := json.MarshalIndent(output, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("error encoding final commit details: %w", err)
	}

	return modifiedData, nil
}

func fetchPullRequestsForCommit(sdk *resilientbridge.ResilientBridge, owner, repo, sha string) ([]int, error) {
	req := &resilientbridge.NormalizedRequest{
		Method:   "GET",
		Endpoint: fmt.Sprintf("/repos/%s/%s/commits/%s/pulls", owner, repo, sha),
		Headers:  map[string]string{"Accept": "application/vnd.github+json"},
	}

	resp, err := sdk.Request("github", req)
	if err != nil {
		return nil, fmt.Errorf("error fetching pull requests for commit: %w", err)
	}

	// If not found or conflict, return empty array
	if resp.StatusCode == 409 || resp.StatusCode == 404 {
		return []int{}, nil
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("HTTP error %d: %s", resp.StatusCode, string(resp.Data))
	}

	var pulls []map[string]interface{}
	if err := json.Unmarshal(resp.Data, &pulls); err != nil {
		return nil, fmt.Errorf("error decoding pull requests: %w", err)
	}

	var prNumbers []int
	for _, pr := range pulls {
		if num, ok := pr["number"].(float64); ok {
			prNumbers = append(prNumbers, int(num))
		}
	}

	return prNumbers, nil
}

// findBranchByCommit queries the branches endpoint to find which branches contain the given commit.
// Returns the name of the first branch found, or an empty string if none.
func findBranchByCommit(sdk *resilientbridge.ResilientBridge, owner, repo, sha string) (string, error) {
	req := &resilientbridge.NormalizedRequest{
		Method:   "GET",
		Endpoint: fmt.Sprintf("/repos/%s/%s/branches?sha=%s", owner, repo, sha),
		Headers:  map[string]string{"Accept": "application/vnd.github+json"},
	}
	resp, err := sdk.Request("github", req)
	if err != nil {
		return "", fmt.Errorf("error fetching branches for commit: %w", err)
	}
	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("HTTP error %d: %s", resp.StatusCode, string(resp.Data))
	}

	var branches []map[string]interface{}
	if err := json.Unmarshal(resp.Data, &branches); err != nil {
		return "", fmt.Errorf("error decoding branches: %w", err)
	}

	if len(branches) == 0 {
		// No branches found containing this commit
		return "", nil
	}

	// Return the name of the first branch
	if name, ok := branches[0]["name"].(string); ok && name != "" {
		return name, nil
	}

	return "", nil
}
