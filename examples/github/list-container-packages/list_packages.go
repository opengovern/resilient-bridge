package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
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

type ContainerMetadata struct {
	Container struct {
		Tags []string `json:"tags"`
	} `json:"container"`
}

type PackageVersion struct {
	ID             int               `json:"id"`
	Name           string            `json:"name"`
	URL            string            `json:"url"`
	PackageHTMLURL string            `json:"package_html_url"`
	CreatedAt      string            `json:"created_at"`
	UpdatedAt      string            `json:"updated_at"`
	HTMLURL        string            `json:"html_url"`
	Metadata       ContainerMetadata `json:"metadata"`
}

type OutputVersion struct {
	ID             int               `json:"id"`
	Digest         string            `json:"digest"`
	URL            string            `json:"url"`
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
	Attestation    interface{}       `json:"attestation"`
}

func main() {
	scopeFlag := flag.String("scope", "", "Scope: ghcr.io/<org>/, ghcr.io/<org>/<package>, or ghcr.io/<org>/<package>:<tag>")
	maxVersionsFlag := flag.Int("max_versions", 1, "Maximum number of versions to retrieve (0 = no limit)")
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

	// Org-level scope (e.g. ghcr.io/org/)
	if strings.HasSuffix(scope, "/") {
		packages := fetchPackages(sdk, org, "container")
		for _, p := range packages {
			packageName := p.Name
			versions := fetchVersions(sdk, org, "container", packageName)
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
		return
	}

	// Check if we have a tag (single version)
	lastPart := parts[len(parts)-1]
	refParts := strings.SplitN(lastPart, ":", 2)
	if len(refParts) == 2 {
		// Single version case: ghcr.io/org/package:tag
		packagePathParts := parts[1 : len(parts)-1]
		packageName := strings.Join(append(packagePathParts, refParts[0]), "/")
		tag := refParts[1]

		versions := fetchVersions(sdk, org, "container", packageName)
		if *maxVersionsFlag > 0 && len(versions) > *maxVersionsFlag {
			versions = versions[:*maxVersionsFlag]
		}
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

		results := getVersionOutput(apiToken, org, packageName, *matchedVersion)
		if len(results) == 0 {
			log.Fatalf("No output for matched version %s:%s", packageName, tag)
		}
		printJSON(results[0])
	} else {
		// Package-level scope: ghcr.io/org/package
		packageName := strings.Join(parts[1:], "/")
		versions := fetchVersions(sdk, org, "container", packageName)
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
		ov := fetchAndAssembleOutput(apiToken, version, imageRef)
		results = append(results, ov)
	}
	return results
}

func fetchAndAssembleOutput(apiToken string, version PackageVersion, imageRef string) OutputVersion {
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

	// Attempt to fetch attestation
	attestation := fetchAttestation(apiToken, ref.Context().Name(), desc.Descriptor.Digest.String())

	return OutputVersion{
		ID:             version.ID,
		Digest:         version.Name,
		URL:            version.URL,
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
		Attestation:    attestation,
	}
}

func fetchAttestation(apiToken, repoName, digest string) interface{} {
	authOption := remote.WithAuth(&authn.Basic{
		Username: "github",
		Password: apiToken,
	})

	refByDigest, err := name.NewDigest(fmt.Sprintf("%s@%s", repoName, digest))
	if err != nil {
		// If we cannot parse, return nil
		return nil
	}

	idx, err := remote.Referrers(refByDigest, authOption, remote.WithContext(context.Background()))
	if err != nil {
		// If the registry doesn't support or no referrers found, return nil
		return nil
	}

	indexManifest, err := idx.IndexManifest()
	if err != nil {
		return nil
	}

	// Known attestation media types
	knownAttestationMediaTypes := map[string]bool{
		"application/vnd.dev.cosign.attestation.v1+json": true,
	}

	// Search through index for an attestation artifact
	for _, artifact := range indexManifest.Manifests {
		mt := string(artifact.MediaType)
		if knownAttestationMediaTypes[mt] {
			// The artifact here is an OCI Artifact Manifest. We need to fetch it.
			attRef, err := name.NewDigest(fmt.Sprintf("%s@%s", repoName, artifact.Digest.String()))
			if err != nil {
				continue
			}
			attDesc, err := remote.Get(attRef, authOption)
			if err != nil {
				continue
			}

			var artifactManifest struct {
				MediaType string `json:"mediaType"`
				Blobs     []struct {
					Digest    string `json:"digest"`
					MediaType string `json:"mediaType"`
				} `json:"blobs"`
			}
			if err := json.Unmarshal(attDesc.Manifest, &artifactManifest); err != nil {
				continue
			}

			for _, blob := range artifactManifest.Blobs {
				blobMT := string(blob.MediaType)
				if knownAttestationMediaTypes[blobMT] {
					blobRef, err := name.NewDigest(fmt.Sprintf("%s@%s", repoName, blob.Digest))
					if err != nil {
						continue
					}
					layer, err := remote.Layer(blobRef, authOption)
					if err != nil {
						continue
					}
					rc, err := layer.Uncompressed()
					if err != nil {
						continue
					}
					defer rc.Close()
					blobBytes, err := io.ReadAll(rc)
					if err != nil {
						continue
					}

					var attPayload interface{}
					if err := json.Unmarshal(blobBytes, &attPayload); err != nil {
						continue
					}
					return attPayload
				}
			}
		}
	}

	// If no attestation found
	return nil
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
