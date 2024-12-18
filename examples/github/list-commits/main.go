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
	maxCommitsFlag := flag.Int("maxcommits", 5, "Maximum number of commits to fetch (default 50)")
	flag.Parse()

	if *repoFlag == "" {
		log.Fatal("You must provide a -repo parameter, e.g. -repo=https://github.com/apache/cloudstack")
	}

	owner, repo, err := parseRepoURL(*repoFlag)
	if err != nil {
		log.Fatalf("Error parsing repo URL: %v", err)
	}

	maxCommits := *maxCommitsFlag
	if maxCommits <= 0 {
		maxCommits = 50
	}

	commits, err := fetchCommitList(owner, repo, maxCommits)
	if err != nil {
		log.Fatalf("Error fetching commits list: %v", err)
	}

	for _, c := range commits {
		commitJSON, err := fetchCommitDetails(owner, repo, c.SHA)
		if err != nil {
			log.Printf("Error fetching commit %s details: %v", c.SHA, err)
			continue
		}
		// Print the commit JSON directly as received
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

// fetchCommitList returns up to maxCommits commit references from the repo’s default branch (usually main).
func fetchCommitList(owner, repo string, maxCommits int) ([]commitRef, error) {
	var allCommits []commitRef
	perPage := 100
	page := 1
	client := &http.Client{}

	for len(allCommits) < maxCommits {
		remaining := maxCommits - len(allCommits)
		if remaining < perPage {
			perPage = remaining
		}

		u := fmt.Sprintf("https://api.github.com/repos/%s/%s/commits?per_page=%d&page=%d", owner, repo, perPage, page)
		req, err := http.NewRequest("GET", u, nil)
		if err != nil {
			return nil, fmt.Errorf("error creating request: %w", err)
		}
		req.Header.Set("Accept", "application/vnd.github+json")

		resp, err := client.Do(req)
		if err != nil {
			return nil, fmt.Errorf("error fetching commits: %w", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode >= 400 {
			body, _ := io.ReadAll(resp.Body)
			return nil, fmt.Errorf("HTTP error %d: %s", resp.StatusCode, string(body))
		}

		var commits []commitRef
		err = json.NewDecoder(resp.Body).Decode(&commits)
		if err != nil {
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

	// In case we got more than we needed (unlikely since we controlled perPage, but just a safety check)
	if len(allCommits) > maxCommits {
		allCommits = allCommits[:maxCommits]
	}

	return allCommits, nil
}

// fetchCommitDetails fetches the full commit JSON for a given commit SHA.
func fetchCommitDetails(owner, repo, sha string) ([]byte, error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/%s/commits/%s", owner, repo, sha)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("creating commit details request: %w", err)
	}
	req.Header.Set("Accept", "application/vnd.github+json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("error fetching commit details: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("HTTP error %d: %s", resp.StatusCode, string(body))
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("error reading commit details: %w", err)
	}

	return data, nil
}
