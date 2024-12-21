package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	resilientbridge "github.com/opengovern/resilient-bridge"
	"github.com/opengovern/resilient-bridge/adapters"
)

// -------------------------------------------------------------------
// Data Structures
// -------------------------------------------------------------------

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
}

type ContainerMetadata struct {
	Container struct {
		Tags []string `json:"tags"`
	} `json:"container"`
}

type PackageVersion struct {
	ID             int               `json:"id"`
	Name           string            `json:"name"`
	PackageHTMLURL string            `json:"package_html_url"`
	CreatedAt      string            `json:"created_at"`
	UpdatedAt      string            `json:"updated_at"`
	HTMLURL        string            `json:"html_url"`
	Metadata       ContainerMetadata `json:"metadata"`
}

// OutputVersion now includes a list of additional package URIs
type OutputVersion struct {
	ID                    int               `json:"id"`
	Digest                string            `json:"digest"`
	PackageURI            string            `json:"package_uri"`
	AdditionalPackageURIs []string          `json:"additional_package_uris,omitempty"`
	PackageHTMLURL        string            `json:"package_html_url"`
	CreatedAt             string            `json:"created_at"`
	UpdatedAt             string            `json:"updated_at"`
	HTMLURL               string            `json:"html_url"`
	Name                  string            `json:"name"`
	MediaType             string            `json:"media_type"`
	TotalSize             int64             `json:"total_size"`
	Metadata              ContainerMetadata `json:"metadata"`
	Manifest              interface{}       `json:"manifest"`
}

// -------------------------------------------------------------------
// Global flags
// -------------------------------------------------------------------

var (
	startTime *time.Time
	endTime   *time.Time
)

// -------------------------------------------------------------------
// main()
// -------------------------------------------------------------------

func main() {
	orgFlag := flag.String("organization", "", "GitHub organization name")
	maxVersionsFlag := flag.Int("max_versions", 1, "Maximum number of versions to retrieve (0 = no limit)")
	startTimeFlag := flag.String("start_time", "", "Filter results updated after this time (RFC3339)")
	endTimeFlag := flag.String("end_time", "", "Filter results updated before this time (RFC3339)")
	flag.Parse()

	if *orgFlag == "" {
		log.Fatal("You must provide a -organization parameter")
	}

	// Parse time range if provided
	if *startTimeFlag != "" {
		t, err := time.Parse(time.RFC3339, *startTimeFlag)
		if err != nil {
			log.Fatalf("Invalid start_time format: %v", err)
		}
		startTime = &t
	}
	if *endTimeFlag != "" {
		t, err := time.Parse(time.RFC3339, *endTimeFlag)
		if err != nil {
			log.Fatalf("Invalid end_time format: %v", err)
		}
		endTime = &t
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

	org := *orgFlag

	// Fetch all container packages with pagination
	packages := fetchPackages(sdk, org, "container")
	packages = filterPackagesByTime(packages)

	for _, p := range packages {
		packageName := p.Name
		// Fetch all versions for each package with pagination
		versions := fetchVersions(sdk, org, "container", packageName)
		versions = filterVersionsByTime(versions)

		// Apply user-specified limit
		if *maxVersionsFlag > 0 && len(versions) > *maxVersionsFlag {
			versions = versions[:*maxVersionsFlag]
		}

		// For each version, gather output
		for _, v := range versions {
			results := getVersionOutput(apiToken, org, packageName, v)
			for _, ov := range results {
				printJSON(ov)
			}
		}
	}
}

// -------------------------------------------------------------------
// Helpers: Pagination, Filtering, JSON Output
// -------------------------------------------------------------------

func fetchPackages(sdk *resilientbridge.ResilientBridge, org, packageType string) []Package {
	var allPackages []Package
	page := 1
	perPage := 100

	for {
		listReq := &resilientbridge.NormalizedRequest{
			Method: "GET",
			Endpoint: fmt.Sprintf(
				"/orgs/%s/packages?package_type=%s&per_page=%d&page=%d",
				org, packageType, perPage, page,
			),
			Headers: map[string]string{"Accept": "application/vnd.github+json"},
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
		if len(packages) == 0 {
			break
		}

		allPackages = append(allPackages, packages...)
		if len(packages) < perPage {
			// No more pages
			break
		}
		page++
	}
	return allPackages
}

func fetchVersions(sdk *resilientbridge.ResilientBridge, org, packageType, packageName string) []PackageVersion {
	packageNameEncoded := url.PathEscape(packageName)
	var allVersions []PackageVersion
	page := 1
	perPage := 100

	for {
		versionsReq := &resilientbridge.NormalizedRequest{
			Method: "GET",
			Endpoint: fmt.Sprintf(
				"/orgs/%s/packages/%s/%s/versions?per_page=%d&page=%d",
				org, packageType, packageNameEncoded, perPage, page,
			),
			Headers: map[string]string{"Accept": "application/vnd.github+json"},
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
		if len(versions) == 0 {
			break
		}

		allVersions = append(allVersions, versions...)
		if len(versions) < perPage {
			break
		}
		page++
	}
	return allVersions
}

func filterPackagesByTime(pkgs []Package) []Package {
	if startTime == nil && endTime == nil {
		return pkgs
	}
	var filtered []Package
	for _, p := range pkgs {
		t, err := time.Parse(time.RFC3339, p.UpdatedAt)
		if err != nil {
			continue
		}
		if (startTime == nil || t.After(*startTime)) && (endTime == nil || t.Before(*endTime)) {
			filtered = append(filtered, p)
		}
	}
	return filtered
}

func filterVersionsByTime(vers []PackageVersion) []PackageVersion {
	if startTime == nil && endTime == nil {
		return vers
	}
	var filtered []PackageVersion
	for _, v := range vers {
		t, err := time.Parse(time.RFC3339, v.UpdatedAt)
		if err != nil {
			continue
		}
		if (startTime == nil || t.After(*startTime)) && (endTime == nil || t.Before(*endTime)) {
			filtered = append(filtered, v)
		}
	}
	return filtered
}

func printJSON(obj interface{}) {
	outBytes, err := json.Marshal(obj)
	if err != nil {
		log.Fatalf("Error marshalling output: %v", err)
	}
	fmt.Println(string(outBytes))
}

// -------------------------------------------------------------------
// Core logic: Deduplicate (id,digest) and store extra tags
// -------------------------------------------------------------------

func getVersionOutput(apiToken, org, packageName string, version PackageVersion) []OutputVersion {
	authOption := remote.WithAuth(&authn.Basic{
		Username: "github",
		Password: apiToken,
	})

	// We store one OutputVersion per (ID, realDigest).
	// additional_package_uris will hold subsequent tags with same (id, digest).
	dedup := make(map[string]*OutputVersion)

	for _, tag := range version.Metadata.Container.Tags {
		imageRef := fmt.Sprintf("ghcr.io/%s/%s:%s",
			org,
			strings.ToLower(packageName),
			strings.ToLower(tag),
		)
		ref, err := name.ParseReference(imageRef)
		if err != nil {
			log.Printf("Error parsing reference %s: %v (skipping)", imageRef, err)
			continue
		}

		desc, err := remote.Get(ref, authOption)
		if err != nil {
			log.Printf("Error fetching manifest for %s: %v (skipping)", imageRef, err)
			continue
		}

		actualDigest := desc.Descriptor.Digest.String()
		// Combine version.ID + the real registry digest for dedup
		dedupKey := fmt.Sprintf("%d|%s", version.ID, actualDigest)

		// If we have an existing OutputVersion with same (id,digest), just add to AdditionalPackageURIs
		if existing, ok := dedup[dedupKey]; ok {
			existing.AdditionalPackageURIs = append(existing.AdditionalPackageURIs, imageRef)
			continue
		}

		// Otherwise, parse the manifest to find total size, etc.
		var manifestStruct struct {
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
		if err := json.Unmarshal(desc.Manifest, &manifestStruct); err != nil {
			log.Printf("Error unmarshaling manifest JSON for %s: %v", imageRef, err)
			continue
		}

		totalSize := manifestStruct.Config.Size
		for _, layer := range manifestStruct.Layers {
			totalSize += layer.Size
		}

		// Parse entire manifest into an interface{} for output
		var manifestInterface interface{}
		if err := json.Unmarshal(desc.Manifest, &manifestInterface); err != nil {
			log.Printf("Error unmarshaling manifest for output: %v", err)
			continue
		}

		// Create new OutputVersion record, storing the FIRST tag as package_uri
		ov := &OutputVersion{
			ID:                    version.ID,
			Digest:                actualDigest,
			PackageURI:            imageRef,   // "First tag" for this digest
			AdditionalPackageURIs: []string{}, // Will append more if found
			PackageHTMLURL:        version.PackageHTMLURL,
			CreatedAt:             version.CreatedAt,
			UpdatedAt:             version.UpdatedAt,
			HTMLURL:               version.HTMLURL,
			Name:                  imageRef, // or set to something else
			MediaType:             string(desc.Descriptor.MediaType),
			TotalSize:             totalSize,
			Metadata:              version.Metadata,
			Manifest:              manifestInterface,
		}

		dedup[dedupKey] = ov
	}

	// Convert map values to a slice in stable order (not guaranteed here).
	// If you need stable output, you can store them in insertion order.
	var results []OutputVersion
	for _, output := range dedup {
		results = append(results, *output)
	}
	return results
}
