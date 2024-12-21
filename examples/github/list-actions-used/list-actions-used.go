package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

// GitHubSearchResponse models the response from the GitHub code search API.
type GitHubSearchResponse struct {
	Items []struct {
		Name       string `json:"name"`
		Path       string `json:"path"`
		HTMLURL    string `json:"html_url"`
		Repository struct {
			FullName string `json:"full_name"`
		} `json:"repository"`
	} `json:"items"`
}

// GitHubFileContent models the response from the GitHub contents API.
type GitHubFileContent struct {
	Content  string `json:"content"`
	Encoding string `json:"encoding"`
}

// Workflow represents a simplified GitHub Actions workflow structure for parsing.
type Workflow struct {
	Jobs map[string]struct {
		Steps []map[string]interface{} `yaml:"steps"`
	} `yaml:"jobs"`
}

func main() {
	if len(os.Args) < 3 {
		fmt.Fprintf(os.Stderr, "Usage: %s <repo (owner/repo)> <username>\n", os.Args[0])
		os.Exit(1)
	}

	repo := os.Args[1]     // e.g. "facebook/react"
	username := os.Args[2] // arbitrary user agent identifier

	token := os.Getenv("CR_PAT")
	if token == "" {
		fmt.Fprintln(os.Stderr, "CR_PAT environment variable is not set")
		os.Exit(1)
	}

	ctx := context.Background()

	// 1. Search for workflow files containing `uses:`
	searchURL := fmt.Sprintf("https://api.github.com/search/code?q=repo:%s+path:.github/workflows+uses:", repo)
	respBytes, err := doGitHubRequest(ctx, searchURL, token, username)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error searching code: %v\n", err)
		os.Exit(1)
	}

	var searchResp GitHubSearchResponse
	if err := json.Unmarshal(respBytes, &searchResp); err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing search response: %v\n", err)
		os.Exit(1)
	}

	// 2. For each file, fetch content and parse actions.
	for _, item := range searchResp.Items {
		fileContentURL := fmt.Sprintf("https://api.github.com/repos/%s/contents/%s", item.Repository.FullName, item.Path)

		contentBytes, err := doGitHubRequest(ctx, fileContentURL, token, username)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error fetching file content for %s: %v\n", item.Path, err)
			continue
		}

		var fileContent GitHubFileContent
		if err := json.Unmarshal(contentBytes, &fileContent); err != nil {
			fmt.Fprintf(os.Stderr, "Error unmarshalling file content for %s: %v\n", item.Path, err)
			continue
		}

		var decoded []byte
		if fileContent.Encoding == "base64" {
			decoded, err = base64.StdEncoding.DecodeString(fileContent.Content)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error base64 decoding content for %s: %v\n", item.Path, err)
				continue
			}
		} else {
			decoded = []byte(fileContent.Content)
		}

		// Parse YAML
		var workflow Workflow
		if err := yaml.Unmarshal(decoded, &workflow); err != nil {
			fmt.Fprintf(os.Stderr, "Error parsing YAML for %s: %v\n", item.Path, err)
			continue
		}

		// Extract actions from steps
		actions := extractActions(workflow)

		// Print the results
		fmt.Printf("File: %s\n", item.Path)
		if len(actions) == 0 {
			fmt.Println("  No actions found.")
		} else {
			for _, a := range actions {
				fmt.Printf("  - %s\n", a)
			}
		}
	}
}

func doGitHubRequest(ctx context.Context, url, token, username string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Add("Authorization", "token "+token)
	req.Header.Add("User-Agent", username)
	req.Header.Add("Accept", "application/vnd.github.v3+json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		body, _ := ioutil.ReadAll(resp.Body)
		return nil, fmt.Errorf("non-2xx status code: %d, body: %s", resp.StatusCode, string(body))
	}

	return ioutil.ReadAll(resp.Body)
}

func extractActions(workflow Workflow) []string {
	var actions []string
	for _, job := range workflow.Jobs {
		for _, step := range job.Steps {
			if usesVal, ok := step["uses"]; ok {
				if usesStr, ok := usesVal.(string); ok {
					actions = append(actions, strings.TrimSpace(usesStr))
				}
			}
		}
	}
	return actions
}
