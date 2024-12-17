package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	resilientbridge "github.com/opengovern/resilient-bridge"
	"github.com/opengovern/resilient-bridge/adapters"
)

type Owner struct {
	Login string `json:"login"`
}

type Package struct {
	ID          int    `json:"id"`
	Name        string `json:"name"`
	PackageType string `json:"package_type"`
	Visibility  string `json:"visibility"`
	HTMLURL     string `json:"html_url"`
	CreatedAt   string `json:"created_at"`
	UpdatedAt   string `json:"updated_at"`
	Owner       Owner  `json:"owner"`
	URL         string `json:"url"`
}

type PackageVersion struct {
	ID             int    `json:"id"`
	Name           string `json:"name"`
	URL            string `json:"url"`
	PackageHTMLURL string `json:"package_html_url"`
	CreatedAt      string `json:"created_at"`
	UpdatedAt      string `json:"updated_at"`
	HTMLURL        string `json:"html_url"`
	Metadata       struct {
		Container struct {
			Tags []string `json:"tags"`
		} `json:"container"`
	} `json:"metadata"`
}

// ManifestOutput is the output format for a single image manifest request.
type ManifestOutput struct {
	Name             string          `json:"name"`
	MediaType        string          `json:"MediaType"`
	TotalSize        int64           `json:"Total Size"`
	Digest           string          `json:"Digest"`
	VersionID        int             `json:"Version ID"`
	VersionCreatedAt string          `json:"CreatedAt"`
	VersionUpdatedAt string          `json:"UpdatedAt"`
	VersionHTML      string          `json:"HTML"`
	CompleteManifest json.RawMessage `json:"complete manifest"`
}

func main() {
	scopeFlag := flag.String("scope", "", "Scope: ghcr.io/<org>/, ghcr.io/<org>/<package>, or ghcr.io/<org>/<package>:<tag>")
	flag.Parse()

	if *scopeFlag == "" {
		log.Fatal("You must provide a -scope parameter")
	}

	apiToken := os.Getenv("GITHUB_API_TOKEN")
	if apiToken == "" {
		log.Fatal("GITHUB_API_TOKEN environment variable not set or missing read:packages scope")
	}

	// Register provider for GitHub
	sdk := resilientbridge.NewResilientBridge()
	sdk.RegisterProvider("github", adapters.NewGitHubAdapter(apiToken), &resilientbridge.ProviderConfig{
		UseProviderLimits: true,
		MaxRetries:        3,
		BaseBackoff:       0,
	})

	scope := *scopeFlag
	if !strings.HasPrefix(scope, "ghcr.io/") {
		log.Fatal("Scope must start with ghcr.io/")
	}

	parts := strings.Split(strings.TrimPrefix(scope, "ghcr.io/"), "/")
	org := parts[0]

	// Check if we have a trailing slash (namespace)
	if strings.HasSuffix(scope, "/") {
		// Case 1: ghcr.io/opengovern/
		// List all container packages in the org
		listPackages(sdk, org, apiToken)

	} else {
		// Not a trailing slash. Check for a tag or not
		lastPart := parts[len(parts)-1]
		refParts := strings.SplitN(lastPart, ":", 2)
		if len(refParts) == 2 {
			// Case 3: Has a tag
			// Package name is everything after org, excluding the tag
			packagePathParts := parts[1 : len(parts)-1]
			packageName := strings.Join(append(packagePathParts, refParts[0]), "/")
			tag := refParts[1]
			getManifestForVersion(sdk, org, packageName, tag, apiToken)

		} else {
			// Case 2: No tag, means list all versions of that package
			packageName := strings.Join(parts[1:], "/")
			listVersions(sdk, org, packageName, apiToken)
		}
	}
}

// listPackages lists all container packages in the given org and prints as JSON.
func listPackages(sdk *resilientbridge.ResilientBridge, org string, apiToken string) {
	packages := fetchPackages(sdk, org, "container")
	output, err := json.MarshalIndent(packages, "", "  ")
	if err != nil {
		log.Fatalf("Error marshalling packages: %v", err)
	}
	fmt.Println(string(output))
}

// listVersions lists all versions for a given package and prints as JSON.
func listVersions(sdk *resilientbridge.ResilientBridge, org, packageName, apiToken string) {
	versions := fetchVersions(sdk, org, "container", packageName)
	output, err := json.MarshalIndent(versions, "", "  ")
	if err != nil {
		log.Fatalf("Error marshalling versions: %v", err)
	}
	fmt.Println(string(output))
}

// getManifestForVersion fetches the manifest for a given package:tag and prints the JSON as specified.
func getManifestForVersion(sdk *resilientbridge.ResilientBridge, org, packageName, tag, apiToken string) {
	imageRef := fmt.Sprintf("ghcr.io/%s/%s:%s", org, packageName, tag)

	// Fetch versions and find the one that matches this tag
	versions := fetchVersions(sdk, org, "container", packageName)
	var matchedVersion *PackageVersion
	for i, v := range versions {
		for _, t := range v.Metadata.Container.Tags {
			if t == tag {
				matchedVersion = &versions[i]
				break
			}
		}
		if matchedVersion != nil {
			break
		}
	}

	if matchedVersion == nil {
		log.Fatalf("No version found with tag %s for package %s/%s", tag, org, packageName)
	}

	ref, err := name.ParseReference(imageRef)
	if err != nil {
		log.Fatalf("Error parsing reference %s: %v", imageRef, err)
	}

	// Auth option using the provided token
	authOption := remote.WithAuth(&authn.Basic{
		Username: "github",
		Password: apiToken,
	})

	desc, err := remote.Get(ref, authOption)
	if err != nil {
		log.Fatalf("Error fetching manifest for %s: %v", imageRef, err)
	}

	// Calculate total size (config + layers)
	var manifest struct {
		SchemaVersion int    `json:"schemaVersion"`
		MediaType     string `json:"mediaType"`
		Config        struct {
			Size      int64  `json:"size"`
			Digest    string `json:"digest"`
			MediaType string `json:"mediaType"`
		} `json:"config"`
		Layers []struct {
			Size      int64  `json:"size"`
			Digest    string `json:"digest"`
			MediaType string `json:"mediaType"`
		} `json:"layers"`
	}
	if err := json.Unmarshal(desc.Manifest, &manifest); err != nil {
		log.Fatalf("Error unmarshaling manifest JSON: %v", err)
	}

	totalSize := manifest.Config.Size
	for _, layer := range manifest.Layers {
		totalSize += layer.Size
	}

	output := ManifestOutput{
		Name:             imageRef,
		MediaType:        string(desc.Descriptor.MediaType), // convert MediaType to string
		TotalSize:        totalSize,
		Digest:           desc.Descriptor.Digest.String(),
		VersionID:        matchedVersion.ID,
		VersionCreatedAt: matchedVersion.CreatedAt,
		VersionUpdatedAt: matchedVersion.UpdatedAt,
		VersionHTML:      matchedVersion.HTMLURL,
		CompleteManifest: desc.Manifest,
	}

	outBytes, err := json.MarshalIndent(output, "", "  ")
	if err != nil {
		log.Fatalf("Error marshalling output: %v", err)
	}
	fmt.Println(string(outBytes))
}

func fetchPackages(sdk *resilientbridge.ResilientBridge, org, packageType string) []Package {
	listReq := &resilientbridge.NormalizedRequest{
		Method:   "GET",
		Endpoint: fmt.Sprintf("/orgs/%s/packages?package_type=%s", org, packageType),
		Headers:  map[string]string{"Accept": "application/vnd.github+json"},
	}
	listResp, err := sdk.Request("github", listReq)
	if err != nil {
		log.Fatalf("Error listing packages: %v", err)
	}
	if listResp.StatusCode >= 400 {
		log.Fatalf("HTTP error %d: %s", listResp.StatusCode, string(listResp.Data))
	}
	var packages []Package
	if err := json.Unmarshal(listResp.Data, &packages); err != nil {
		log.Fatalf("Error parsing packages list response: %v", err)
	}
	return packages
}

func fetchVersions(sdk *resilientbridge.ResilientBridge, org, packageType, packageName string) []PackageVersion {
	versionsReq := &resilientbridge.NormalizedRequest{
		Method:   "GET",
		Endpoint: fmt.Sprintf("/orgs/%s/packages/%s/%s/versions", org, packageType, packageName),
		Headers:  map[string]string{"Accept": "application/vnd.github+json"},
	}

	versionsResp, err := sdk.Request("github", versionsReq)
	if err != nil {
		log.Fatalf("Error listing package versions: %v", err)
	}
	if versionsResp.StatusCode >= 400 {
		log.Fatalf("HTTP error %d: %s", versionsResp.StatusCode, string(versionsResp.Data))
	}

	var versions []PackageVersion
	if err := json.Unmarshal(versionsResp.Data, &versions); err != nil {
		log.Fatalf("Error parsing package versions response: %v", err)
	}
	return versions
}
