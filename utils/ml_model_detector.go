// File: github.com/opengovern/resilient-bridge/utils/ml_model_detector.go
package utils

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"net/url"
	"path/filepath"
	"strings"
	"sync"

	resilientbridge "github.com/opengovern/resilient-bridge"
)

// -----------------------------------------------------------------------------
// Constants & Data Structures
// -----------------------------------------------------------------------------

const (
	DEFAULT_CHUNK_SIZE  = 5
	MAX_FILES_PER_REPO  = 500

	// INITIAL_SAMPLE_SIZE: number of files to sample quickly in each directory.
	// If any binary found, we'll check all remaining files to avoid missing others.
	INITIAL_SAMPLE_SIZE = 3
)

// SearchResult models GitHub's /search/code JSON response.
type SearchResult struct {
	TotalCount        int    `json:"total_count"`
	IncompleteResults bool   `json:"incomplete_results"`
	Items             []Item `json:"items"`
}

// Item represents a single code search result (part of the /search/code response).
type Item struct {
	Name       string     `json:"name"`
	Path       string     `json:"path"`
	HTMLURL    string     `json:"html_url"`
	Repository Repository `json:"repository"`
}

// Repository holds basic repository information.
type Repository struct {
	ID       int64  `json:"id,omitempty"`        // May or may not be present in /search responses
	Name     string `json:"name,omitempty"`      // e.g. "platform-configuration"
	FullName string `json:"full_name,omitempty"` // e.g. "opengovern/platform-configuration"
}

// FileExtensions is a list of file extensions we typically search for.
var FileExtensions = []string{
	"h5",
	"hdf5",
	"keras",
	"pb",
	"ckpt",
	"pt",
	"pth",
	"mar",
	"safetensors",
	"joblib",
	"pkl",
	"pickle",
	"sav",
	"onnx",
	"tflite",
	"model",
	"cbm",
	"caffemodel",
	"prototxt",
	"params",
	"mlmodel",
	"pmml",
}

// ExpectedBinaryExt defines which file extensions are expected to be binary
// ML model files. (We often skip "prototxt" or "pmml" since they're text-based.)
var ExpectedBinaryExt = map[string]bool{
	"h5":          true,
	"hdf5":        true,
	"keras":       true,
	"pb":          true,
	"ckpt":        true,
	"pt":          true,
	"pth":         true,
	"mar":         true,
	"safetensors": true,
	"joblib":      true,
	"pkl":         true,
	"pickle":      true,
	"sav":         true,
	"onnx":        true,
	"tflite":      true,
	"model":       true,
	"cbm":         true,
	"caffemodel":  true,
	"params":      true,
	"mlmodel":     true,
}

// DirGroup represents a collection of Items under (repo + directory).
type DirGroup struct {
	Key   string // e.g. "opengovern/platform-configuration|models/"
	Items []Item
}

// RepoOutput is used for generating final JSON output (per repository).
type RepoOutput struct {
	RepositoryID       int64             `json:"repository_id"`
	RepositoryName     string            `json:"repository_name"`
	RepositoryFullName string            `json:"repository_full_name"`
	Extensions         map[string][]string `json:"extensions"`
}

// -----------------------------------------------------------------------------
// Helper & Utility Functions
// -----------------------------------------------------------------------------

// ChunkBySize splits a slice of strings into chunks, each with at most 'size' elements.
func ChunkBySize(slice []string, size int) [][]string {
	var chunks [][]string
	for i := 0; i < len(slice); i += size {
		end := i + size
		if end > len(slice) {
			end = len(slice)
		}
		chunks = append(chunks, slice[i:end])
	}
	return chunks
}

// EscapePath escapes each segment of the file path while preserving slashes.
func EscapePath(p string) string {
	segments := strings.Split(p, "/")
	for i, seg := range segments {
		segments[i] = url.PathEscape(seg)
	}
	return strings.Join(segments, "/")
}

// IsBinaryData returns true if the data appears binary (i.e. contains at least one null byte).
func IsBinaryData(data []byte) bool {
	for _, b := range data {
		if b == 0 {
			return true
		}
	}
	return false
}

// -----------------------------------------------------------------------------
// GitHub Search & File Checking
// -----------------------------------------------------------------------------

// SearchGitHub uses resilient-bridge to execute the GitHub Code Search API call.
func SearchGitHub(sdk *resilientbridge.ResilientBridge, query string, page int) (SearchResult, error) {
	var result SearchResult

	encodedQuery := url.QueryEscape(query)
	endpoint := fmt.Sprintf("/search/code?q=%s&per_page=100&page=%d", encodedQuery, page)

	req := &resilientbridge.NormalizedRequest{
		Method:   "GET",
		Endpoint: endpoint,
		Headers: map[string]string{
			"Accept":     "application/vnd.github.v3+json",
			"User-Agent": "opencomply fetcher",
		},
	}
	resp, err := sdk.Request("github", req)
	if err != nil {
		return result, err
	}
	if resp.StatusCode != 200 {
		// If rate-limited or some other issue, handle here if needed
		return result, fmt.Errorf("non-OK HTTP status: %d", resp.StatusCode)
	}
	if err := json.Unmarshal(resp.Data, &result); err != nil {
		return result, err
	}
	return result, nil
}

// IsBinaryFileForItem uses resilient-bridge to fetch only the first 10 KB
// of a file and checks if it is binary.
func IsBinaryFileForItem(sdk *resilientbridge.ResilientBridge, item Item, verbose bool) (bool, error) {
	if verbose {
		log.Printf("[verbose] Checking file content for %s (%s)", item.Path, item.Repository.FullName)
	}

	endpoint := fmt.Sprintf("/repos/%s/contents/%s", item.Repository.FullName, EscapePath(item.Path))
	req := &resilientbridge.NormalizedRequest{
		Method:   "GET",
		Endpoint: endpoint,
		Headers: map[string]string{
			"Accept":     "application/vnd.github.v3+json",
			"User-Agent": "opencomply fetcher",
		},
	}
	resp, err := sdk.Request("github", req)
	if err != nil {
		return false, err
	}
	if resp.StatusCode != 200 {
		return false, fmt.Errorf("non-OK HTTP status when fetching file content: %d", resp.StatusCode)
	}
	var contentResp struct {
		Content     string `json:"content"`
		Encoding    string `json:"encoding"`
		DownloadURL string `json:"download_url"`
	}
	if err := json.Unmarshal(resp.Data, &contentResp); err != nil {
		return false, err
	}

	var data []byte
	if contentResp.Content != "" && contentResp.Encoding == "base64" {
		cleanContent := strings.ReplaceAll(contentResp.Content, "\n", "")
		data, err = base64.StdEncoding.DecodeString(cleanContent)
		if err != nil {
			return false, err
		}
	} else if contentResp.DownloadURL != "" {
		req2 := &resilientbridge.NormalizedRequest{
			Method:   "GET",
			Endpoint: contentResp.DownloadURL, // full URL
			Headers: map[string]string{
				"Range":      "bytes=0-10239", // first 10 KB
				"User-Agent": "opencomply fetcher",
			},
		}
		resp2, err := sdk.Request("github", req2)
		if err != nil {
			return false, err
		}
		if resp2.StatusCode != 206 && resp2.StatusCode != 200 {
			return false, fmt.Errorf("non-OK HTTP status when fetching partial file content: %d", resp2.StatusCode)
		}
		data = resp2.Data
	} else {
		return false, fmt.Errorf("no content or download_url available")
	}

	if len(data) > 10240 {
		data = data[:10240]
	}
	isBin := IsBinaryData(data)
	if verbose {
		if isBin {
			log.Printf("[verbose] %s => BINARY", item.Path)
		} else {
			log.Printf("[verbose] %s => NOT binary", item.Path)
		}
	}
	return isBin, nil
}

// -----------------------------------------------------------------------------
// Dynamic (Two-Phase) Sampling Logic
// -----------------------------------------------------------------------------

// GatherDirectories groups items by (repo + directory) if extension is in ExpectedBinaryExt.
func GatherDirectories(items []Item) []DirGroup {
	tmp := make(map[string][]Item)
	for _, it := range items {
		ext := strings.TrimPrefix(filepath.Ext(it.Path), ".")
		if !ExpectedBinaryExt[ext] {
			continue
		}
		dir := filepath.Dir(it.Path)
		key := it.Repository.FullName + "|" + dir
		tmp[key] = append(tmp[key], it)
	}
	var groups []DirGroup
	for k, v := range tmp {
		groups = append(groups, DirGroup{Key: k, Items: v})
	}
	return groups
}

// dynamicSampleDirectory does a two-phase sampling approach:
//   1) Sample up to INITIAL_SAMPLE_SIZE files. If none are binary => skip directory.
//   2) If at least 1 is binary => check all remaining files in directory.
func dynamicSampleDirectory(
	sdk *resilientbridge.ResilientBridge,
	group DirGroup,
	verbose bool,
) (bool, []Item) {
	n := len(group.Items)
	if n == 0 {
		return false, nil
	}

	// Shuffle for random sampling
	rand.Shuffle(n, func(i, j int) {
		group.Items[i], group.Items[j] = group.Items[j], group.Items[i]
	})

	if n <= INITIAL_SAMPLE_SIZE {
		var binItems []Item
		foundAny := false
		for _, it := range group.Items {
			ok, err := IsBinaryFileForItem(sdk, it, verbose)
			if err == nil && ok {
				foundAny = true
				binItems = append(binItems, it)
			}
		}
		return foundAny, binItems
	}

	// Phase 1: small initial sample
	sample1 := group.Items[:INITIAL_SAMPLE_SIZE]
	foundBinary := false
	var binItems []Item
	for _, it := range sample1 {
		ok, err := IsBinaryFileForItem(sdk, it, verbose)
		if err == nil && ok {
			foundBinary = true
			binItems = append(binItems, it)
		}
	}
	if !foundBinary {
		// skip entire directory
		return false, nil
	}

	// Phase 2: check all remaining
	remaining := group.Items[INITIAL_SAMPLE_SIZE:]
	for _, it := range remaining {
		ok, err := IsBinaryFileForItem(sdk, it, verbose)
		if err == nil && ok {
			binItems = append(binItems, it)
		}
	}
	return (len(binItems) > 0), binItems
}

// SampleAndFilterDirectories runs dynamic sampling across directories in parallel.
// If a directory's initial sample yields no binaries, we skip it entirely; if it yields
// some, we check all remaining files to ensure we don't miss others.
func SampleAndFilterDirectories(
	sdk *resilientbridge.ResilientBridge,
	groups []DirGroup,
	maxParallel int,
	verbose bool,
) []Item {

	var wg sync.WaitGroup
	sem := make(chan struct{}, maxParallel)

	var mu sync.Mutex
	var kept []Item

	for _, g := range groups {
		group := g
		wg.Add(1)
		sem <- struct{}{} // acquire concurrency slot

		go func() {
			defer wg.Done()
			defer func() { <-sem }() // release slot

			found, binItems := dynamicSampleDirectory(sdk, group, verbose)
			if found {
				mu.Lock()
				kept = append(kept, binItems...)
				mu.Unlock()
				if verbose {
					log.Printf("[verbose] Directory %s => found %d binary file(s).",
						group.Key, len(binItems))
				}
			} else if verbose {
				log.Printf("[verbose] Directory %s => no binaries, skipping.", group.Key)
			}
		}()
	}
	wg.Wait()
	return kept
}

// -----------------------------------------------------------------------------
// Final JSON-Building for Output
// -----------------------------------------------------------------------------

// CreateDetailedRepoExtensionMap organizes items per repository, including
// repository_id, repository_name, repository_full_name, and an "extensions" map
// of ext -> []file paths.
//
// Returns a map keyed by repoFullName => *RepoOutput.
func CreateDetailedRepoExtensionMap(items []Item) map[string]*RepoOutput {
	result := make(map[string]*RepoOutput)

	for _, item := range items {
		repo := item.Repository

		// If we have "ownerName/repoName", let's parse the "repoName"
		repoName := repo.Name
		// If "Name" wasn't populated by GitHub, try to parse from FullName
		if repoName == "" && repo.FullName != "" {
			parts := strings.SplitN(repo.FullName, "/", 2)
			if len(parts) == 2 {
				repoName = parts[1] // e.g. "platform-configuration"
			} else {
				repoName = repo.FullName // fallback if no slash
			}
		}

		if repo.FullName == "" {
			// If we somehow don't have a full_name, skip or handle it
			continue
		}

		if _, ok := result[repo.FullName]; !ok {
			result[repo.FullName] = &RepoOutput{
				RepositoryID:       repo.ID,
				RepositoryName:     repoName,
				RepositoryFullName: repo.FullName,
				Extensions:         make(map[string][]string),
			}
		}

		out := result[repo.FullName]
		ext := strings.TrimPrefix(filepath.Ext(item.Path), ".")
		if ext == "" {
			ext = "unknown"
		}

		// Optionally ensure we don't exceed MAX_FILES_PER_REPO
		if len(out.Extensions[ext]) >= MAX_FILES_PER_REPO {
			continue
		}

		out.Extensions[ext] = append(out.Extensions[ext], item.Path)
	}

	return result
}
