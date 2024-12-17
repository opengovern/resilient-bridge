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

	// We'll add a Manifest field here to include the manifest inline when listing versions.
	Manifest interface{} `json:"manifest,omitempty"`
}

type ManifestDetail struct {
	Name             string          `json:"name"`
	MediaType        string          `json:"MediaType"`
	TotalSize        int64           `json:"total_size"`
	Digest           string          `json:"Digest"`
	CompleteManifest json.RawMessage `json:"complete_manifest"`
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

	if strings.HasSuffix(scope, "/") {
		// List all container packages in the org
		packages := fetchPackages(sdk, org, "container")
		output, err := json.MarshalIndent(packages, "", "  ")
		if err != nil {
			log.Fatalf("Error marshalling packages: %v", err)
		}
		fmt.Println(string(output))
		return
	}

	// Not a trailing slash. Check for a tag
	lastPart := parts[len(parts)-1]
	refParts := strings.SplitN(lastPart, ":", 2)
	if len(refParts) == 2 {
		// Has a tag: single version manifest
		packagePathParts := parts[1 : len(parts)-1]
		packageName := strings.Join(append(packagePathParts, refParts[0]), "/")
		tag := refParts[1]
		getManifestForSpecificVersion(sdk, org, packageName, tag, apiToken)
	} else {
		// No tag: list versions for package
		packageName := strings.Join(parts[1:], "/")
		versions := fetchVersions(sdk, org, "container", packageName)
		// For each version, if tags exist, fetch manifest of the first tag
		for i, v := range versions {
			if len(v.Metadata.Container.Tags) > 0 {
				// Fetch the manifest for the first tag
				// If you want all tags, you'd loop here, but let's do just the first for demonstration
				tag := v.Metadata.Container.Tags[0]
				imageRef := fmt.Sprintf("ghcr.io/%s/%s:%s", org, packageName, tag)
				manifestDetail := fetchManifest(apiToken, imageRef)
				versions[i].Manifest = manifestDetail
			}
		}

		output, err := json.MarshalIndent(versions, "", "  ")
		if err != nil {
			log.Fatalf("Error marshalling versions: %v", err)
		}
		fmt.Println(string(output))
	}
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

func getManifestForSpecificVersion(sdk *resilientbridge.ResilientBridge, org, packageName, tag, apiToken string) {
	imageRef := fmt.Sprintf("ghcr.io/%s/%s:%s", org, packageName, tag)

	// You may want to fetch versions if you need version details (ID, etc.)
	// If you need version details from the version's metadata, fetch versions and find the matching one:
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

	manifestDetail := fetchManifest(apiToken, imageRef)

	// If matchedVersion is found, we can augment manifestDetail with version info if required
	if matchedVersion != nil {
		// Add desired fields to manifestDetail or create a combined structure
		type FullOutput struct {
			ID             int            `json:"id"`
			ReferenceName  string         `json:"reference_name"`
			Name           string         `json:"name"`
			URL            string         `json:"url"`
			PackageHTMLURL string         `json:"package_html_url"`
			CreatedAt      string         `json:"created_at"`
			UpdatedAt      string         `json:"updated_at"`
			HTMLURL        string         `json:"html_url"`
			Metadata       interface{}    `json:"metadata"`
			Manifest       ManifestDetail `json:"manifest"`
		}

		// Name field here is a bit ambiguous—`matchedVersion.Name` is usually a digest,
		// and `manifestDetail.Name` is the imageRef. We'll keep `matchedVersion.Name` as `name`.
		full := FullOutput{
			ID:             matchedVersion.ID,
			ReferenceName:  imageRef,
			Name:           matchedVersion.Name,
			URL:            matchedVersion.URL,
			PackageHTMLURL: matchedVersion.PackageHTMLURL,
			CreatedAt:      matchedVersion.CreatedAt,
			UpdatedAt:      matchedVersion.UpdatedAt,
			HTMLURL:        matchedVersion.HTMLURL,
			Metadata:       matchedVersion.Metadata,
			Manifest:       manifestDetail,
		}

		outBytes, err := json.MarshalIndent(full, "", "  ")
		if err != nil {
			log.Fatalf("Error marshalling output: %v", err)
		}
		fmt.Println(string(outBytes))
	} else {
		// If we didn't find a matched version, just print the manifest detail
		outBytes, err := json.MarshalIndent(manifestDetail, "", "  ")
		if err != nil {
			log.Fatalf("Error marshalling output: %v", err)
		}
		fmt.Println(string(outBytes))
	}
}

func fetchManifest(apiToken, imageRef string) ManifestDetail {
	ref, err := name.ParseReference(imageRef)
	if err != nil {
		log.Fatalf("Error parsing reference %s: %v", imageRef, err)
	}

	authOption := remote.WithAuth(&authn.Basic{
		Username: "github",
		Password: apiToken,
	})

	desc, err := remote.Get(ref, authOption)
	if err != nil {
		log.Fatalf("Error fetching manifest for %s: %v", imageRef, err)
	}

	// Calculate total size
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

	return ManifestDetail{
		Name:             imageRef,
		MediaType:        string(desc.Descriptor.MediaType),
		TotalSize:        totalSize,
		Digest:           desc.Descriptor.Digest.String(),
		CompleteManifest: desc.Manifest,
	}
}
