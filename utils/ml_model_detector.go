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

const (
	DEFAULT_CHUNK_SIZE = 5
	MAX_FILES_PER_REPO = 500

	// INITIAL_SAMPLE_SIZE is how many "binary-likely" files
	// we quickly sample in each directory. If any pass the check,
	// we check the rest of those binary-likely files.
	INITIAL_SAMPLE_SIZE = 3
)

// SearchResult models GitHub's /search/code JSON response
type SearchResult struct {
	TotalCount        int    `json:"total_count"`
	IncompleteResults bool   `json:"incomplete_results"`
	Items             []Item `json:"items"`
}

// Item represents a single code search result (GitHub /search/code).
type Item struct {
	Name       string     `json:"name"`
	Path       string     `json:"path"`
	HTMLURL    string     `json:"html_url"`
	Repository Repository `json:"repository"`
}

// Repository holds basic repository information.
type Repository struct {
	ID       int64  `json:"id,omitempty"`
	Name     string `json:"name,omitempty"`      // e.g. "my-repo"
	FullName string `json:"full_name,omitempty"` // e.g. "ownerName/my-repo"
}

// PythonAIKeywords is our list of ML frameworks we want to find references to in .py files.

var PythonAIKeywords = []string{
    "tensorflow",
    "torch",
    "keras",
    "sklearn",
    "pytorch_lightning",
    "lightgbm",
    "catboost",
    "mxnet",
    "paddle",
    "jax",
    "xgboost",
    "fastai",
    "h2o",
    "huggingface_hub",
    "statsmodels",
    "prophet",
    "spacy",
    "nltk",
    "gensim",
    "detectron2",
}


// FileExtensions is the set of all extensions we want to find.
// This includes both "binary-likely" and "text-based" model files.
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
	"prototxt", // text-based
	"params",
	"mlmodel",
	"pmml", // text-based
}

// ExpectedBinaryExt indicates which extensions we *expect* to be binary formats
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
	// We do NOT expect "prototxt" or "pmml" to be binary => skip checks for them
}

// DirGroup represents a collection of Items under (repo + directory).
type DirGroup struct {
	Key      string // e.g. "owner/repo|models/"
	AllItems []Item // includes both binary-likely + text-based
}

// RepoOutput is used for the final JSON structure, per repository
type RepoOutput struct {
	RepositoryID       int64               `json:"repository_id"`
	RepositoryName     string              `json:"repository_name"`
	RepositoryFullName string              `json:"repository_full_name"`
	Extensions         map[string][]string `json:"extensions"`
}

// ---------------------------------------------------------------------
// Utility methods
// ---------------------------------------------------------------------

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
	parts := strings.Split(p, "/")
	for i := range parts {
		parts[i] = url.PathEscape(parts[i])
	}
	return strings.Join(parts, "/")
}

// IsBinaryData returns true if data has at least one null byte.
func IsBinaryData(data []byte) bool {
	for _, b := range data {
		if b == 0 {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------------
// GitHub Searching
// ---------------------------------------------------------------------

func SearchGitHub(sdk *resilientbridge.ResilientBridge, query string, page int) (SearchResult, error) {
	var result SearchResult

	endpoint := fmt.Sprintf("/search/code?q=%s&per_page=100&page=%d", url.QueryEscape(query), page)
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
		return result, fmt.Errorf("non-OK HTTP status: %d", resp.StatusCode)
	}
	if err := json.Unmarshal(resp.Data, &result); err != nil {
		return result, err
	}
	return result, nil
}

// IsBinaryFileForItem fetches up to 10 KB of the file, checks if binary.
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
		return false, fmt.Errorf("non-OK HTTP status: %d", resp.StatusCode)
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
		clean := strings.ReplaceAll(contentResp.Content, "\n", "")
		data, err = base64.StdEncoding.DecodeString(clean)
		if err != nil {
			return false, err
		}
	} else if contentResp.DownloadURL != "" {
		req2 := &resilientbridge.NormalizedRequest{
			Method:   "GET",
			Endpoint: contentResp.DownloadURL,
			Headers: map[string]string{
				"Range":      "bytes=0-10239",
				"User-Agent": "opencomply fetcher",
			},
		}
		resp2, err := sdk.Request("github", req2)
		if err != nil {
			return false, err
		}
		if resp2.StatusCode != 200 && resp2.StatusCode != 206 {
			return false, fmt.Errorf("HTTP status from partial content: %d", resp2.StatusCode)
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
} // <--- closing brace for IsBinaryFileForItem

// ---------------------------------------------------------------------
// Gather Directories (Keep ALL recognized extensions, including text-based)
// ---------------------------------------------------------------------

// GatherDirectories logs each file's extension if verbose, and organizes
// them by (repo + directory). We do NOT skip text-based or "non-binary" files here.
func GatherDirectories(items []Item, verbose bool) []DirGroup {
	tmp := make(map[string][]Item)

	for _, item := range items {
		ext := strings.TrimPrefix(filepath.Ext(item.Path), ".")
		if verbose {
			log.Printf("[verbose] Found file %s (ext=%s)", item.Path, ext)
		}
		dir := filepath.Dir(item.Path)
		key := item.Repository.FullName + "|" + dir

		tmp[key] = append(tmp[key], item)
	}

	var groups []DirGroup
	for k, v := range tmp {
		groups = append(groups, DirGroup{
			Key:      k,
			AllItems: v,
		})
	}
	return groups
}

// ---------------------------------------------------------------------
// Dynamic Checking: Binary-likely vs. Text-based
// ---------------------------------------------------------------------

func dynamicSampleDirectory(
	sdk *resilientbridge.ResilientBridge,
	group DirGroup,
	verbose bool,
) (confirmedBinaries []Item, textBased []Item) {

	// We'll separate "binary-likely" items from "text-based" items
	var binLikely []Item
	for _, it := range group.AllItems {
		ext := strings.TrimPrefix(filepath.Ext(it.Path), ".")
		if ExpectedBinaryExt[ext] {
			binLikely = append(binLikely, it)
		} else {
			// This is a text-based extension (e.g. .prototxt, .pmml, etc.)
			textBased = append(textBased, it)
		}
	}

	// If no bin-likely files, we simply return empty "confirmedBinaries"
	// but keep the text-based items
	if len(binLikely) == 0 {
		return nil, textBased
	}

	// Two-phase approach for bin-likely files:
	// 1) Shuffle + sample up to INITIAL_SAMPLE_SIZE
	rand.Shuffle(len(binLikely), func(i, j int) {
		binLikely[i], binLikely[j] = binLikely[j], binLikely[i]
	})

	var foundAnyBinary bool
	var confirmed []Item

	if len(binLikely) <= INITIAL_SAMPLE_SIZE {
		// just check them all at once
		for _, it := range binLikely {
			ok, err := IsBinaryFileForItem(sdk, it, verbose)
			if err == nil && ok {
				foundAnyBinary = true
				confirmed = append(confirmed, it)
			}
		}
	} else {
		// Phase 1: sample the first INITIAL_SAMPLE_SIZE
		sample1 := binLikely[:INITIAL_SAMPLE_SIZE]
		var partialConfirmed []Item
		for _, it := range sample1 {
			ok, err := IsBinaryFileForItem(sdk, it, verbose)
			if err == nil && ok {
				foundAnyBinary = true
				partialConfirmed = append(partialConfirmed, it)
			}
		}
		if foundAnyBinary {
			// Phase 2: check the rest
			remainder := binLikely[INITIAL_SAMPLE_SIZE:]
			for _, it := range remainder {
				ok, err := IsBinaryFileForItem(sdk, it, verbose)
				if err == nil && ok {
					partialConfirmed = append(partialConfirmed, it)
				}
			}
		}
		confirmed = partialConfirmed
	}

	return confirmed, textBased
}

// SampleAndFilterDirectories:
// - We gather confirmed binaries from bin-likely files using two-phase sampling.
// - We ALWAYS keep text-based files, even if the bin-likely check fails.
//
// So the final "kept" items from each directory = `confirmedBinaries + textBased`.
func SampleAndFilterDirectories(
	sdk *resilientbridge.ResilientBridge,
	groups []DirGroup,
	maxParallel int,
	verbose bool,
) []Item {

	var wg sync.WaitGroup
	sem := make(chan struct{}, maxParallel)

	var mu sync.Mutex
	var finalKept []Item

	for _, g := range groups {
		group := g
		wg.Add(1)
		sem <- struct{}{} // Acquire concurrency slot

		go func() {
			defer wg.Done()
			defer func() { <-sem }() // Release slot

			confirmedBin, textBased := dynamicSampleDirectory(sdk, group, verbose)

			// We always keep text-based items (like .prototxt).
			// Also keep whichever binaries were truly confirmed as binary.
			kept := append(textBased, confirmedBin...)

			mu.Lock()
			finalKept = append(finalKept, kept...)
			mu.Unlock()

			if verbose {
				log.Printf("[verbose] Directory %s => kept %d items (bin=%d, text=%d)",
					group.Key, len(kept), len(confirmedBin), len(textBased))
			}
		}()
	}

	wg.Wait()
	return finalKept
}

// ---------------------------------------------------------------------
// Build final output: repository_id / name / full_name / extensions->paths
// ---------------------------------------------------------------------

func CreateDetailedRepoExtensionMap(items []Item) map[string]*RepoOutput {
	out := make(map[string]*RepoOutput)

	for _, it := range items {
		r := it.Repository
		// If "r.Name" is empty, parse from r.FullName if possible
		repoName := r.Name
		if repoName == "" && r.FullName != "" {
			parts := strings.SplitN(r.FullName, "/", 2)
			if len(parts) == 2 {
				repoName = parts[1]
			} else {
				repoName = r.FullName
			}
		}

		if r.FullName == "" {
			continue
		}
		if _, ok := out[r.FullName]; !ok {
			out[r.FullName] = &RepoOutput{
				RepositoryID:       r.ID,
				RepositoryName:     repoName,
				RepositoryFullName: r.FullName,
				Extensions:         make(map[string][]string),
			}
		}

		ext := strings.TrimPrefix(filepath.Ext(it.Path), ".")
		if ext == "" {
			ext = "unknown"
		}

		if len(out[r.FullName].Extensions[ext]) < MAX_FILES_PER_REPO {
			out[r.FullName].Extensions[ext] = append(out[r.FullName].Extensions[ext], it.Path)
		}
	}

	return out
}


// BuildPythonAIQueries turns the pythonAIKeywords into a slice of chunked queries like:
//   extension:py tensorflow OR extension:py torch ...
// so we can search them in code.
func BuildPythonAIQueries(qualifier string, keywords []string, chunkSize int) []string {
    // chunk the keywords to avoid overly long queries
    var finalQueries []string
    chunked := ChunkBySize(keywords, chunkSize)

    for _, chunk := range chunked {
        // Build a single query: "extension:py <kw1> OR extension:py <kw2> ..."
        var parts []string
        for _, kw := range chunk {
            if qualifier != "" {
                parts = append(parts, qualifier+" extension:py "+kw)
            } else {
                parts = append(parts, "extension:py "+kw)
            }
        }
        finalQueries = append(finalQueries, strings.Join(parts, " OR "))
    }
    return finalQueries
}
