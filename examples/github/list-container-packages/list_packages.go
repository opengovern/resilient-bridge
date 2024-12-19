package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/url"
	"os"
	"time"

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

type OutputVersion struct {
	ID             int               `json:"id"`
	Digest         string            `json:"digest"`
	PackageURI     string            `json:"package_uri"`
	PackageHTMLURL string            `json:"package_html_url"`
	CreatedAt      string            `json:"created_at"`
	UpdatedAt      string            `json:"updated_at"`
	HTMLURL        string            `json:"html_url"`
	Name           string            `json:"name"`
	MediaType      string            `json:"media_type"`
	TotalSize      int64             `json:"total_size"`
	Metadata       ContainerMetadata `json:"metadata"`
	Manifest       interface{}       `json:"manifest"`
}

var (
	startTime *time.Time
	endTime   *time.Time
)

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

	packages := fetchPackages(sdk, org, "container")
	packages = filterPackagesByTime(packages)

	for _, p := range packages {
		packageName := p.Name
		versions := fetchVersions(sdk, org, "container", packageName)
		versions = filterVersionsByTime(versions)
		if *maxVersionsFlag > 0 && len(versions) > *maxVersionsFlag {
			versions = versions[:*maxVersionsFlag]
		}
		for _, v := range versions {
			results := getVersionOutput(apiToken, org, packageName, v)
			for _, ov := range results {
				printJSON(ov)
			}
		}
	}
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

func getVersionOutput(apiToken, org, packageName string, version PackageVersion) []OutputVersion {
	var results []OutputVersion
	for _, tag := range version.Metadata.Container.Tags {
		imageRef := fmt.Sprintf("ghcr.io/%s/%s:%s", org, packageName, tag)
		ov := fetchAndAssembleOutput(apiToken, org, packageName, version, imageRef)
		results = append(results, ov)
	}
	return results
}

func fetchAndAssembleOutput(apiToken, org, packageName string, version PackageVersion, imageRef string) OutputVersion {
	authOption := remote.WithAuth(&authn.Basic{
		Username: "github",
		Password: apiToken,
	})

	ref, err := name.ParseReference(imageRef)
	if err != nil {
		log.Fatalf("Error parsing reference %s: %v", imageRef, err)
	}

	desc, err := remote.Get(ref, authOption)
	if err != nil {
		log.Fatalf("Error fetching manifest for %s: %v", imageRef, err)
	}

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
		log.Fatalf("Error unmarshaling manifest JSON: %v", err)
	}

	totalSize := manifestStruct.Config.Size
	for _, layer := range manifestStruct.Layers {
		totalSize += layer.Size
	}

	var manifestInterface interface{}
	if err := json.Unmarshal(desc.Manifest, &manifestInterface); err != nil {
		log.Fatalf("Error unmarshaling manifest for output: %v", err)
	}

	return OutputVersion{
		ID:             version.ID,
		Digest:         version.Name,
		PackageURI:     imageRef,
		PackageHTMLURL: version.PackageHTMLURL,
		CreatedAt:      version.CreatedAt,
		UpdatedAt:      version.UpdatedAt,
		HTMLURL:        version.HTMLURL,
		Name:           imageRef,
		MediaType:      string(desc.Descriptor.MediaType),
		TotalSize:      totalSize,
		Metadata:       version.Metadata,
		Manifest:       manifestInterface,
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
	packageNameEncoded := url.PathEscape(packageName)
	versionsReq := &resilientbridge.NormalizedRequest{
		Method:   "GET",
		Endpoint: fmt.Sprintf("/orgs/%s/packages/%s/%s/versions", org, packageType, packageNameEncoded),
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
