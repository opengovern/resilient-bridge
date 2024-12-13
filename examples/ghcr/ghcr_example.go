// ghcr_example.go
//
// This example demonstrates how to use the utility function from github_to_oci_cred.go
// to generate OCI credentials for GitHub Container Registry (GHCR), then optionally
// pull an OCI artifact from GHCR using those credentials.
//
// Prerequisites:
// - A GitHub username and a PAT (Personal Access Token) with read permissions for GHCR.
//
// Environment Variables:
//   GH_USERNAME = GitHub username
//   CR_PAT      = GitHub Personal Access Token
//
// Example:
//   export GH_USERNAME="my-github-username"
//   export CR_PAT="my-github-token"
//   go run ghcr_example.go --oci-artifact-uri=ghcr.io/myuser/myimage:latest
//
// If --oci-artifact-uri is specified, it attempts to pull the artifact using ORAS and
// print its manifest. Without it, it just prints the generated credentials as a Docker config.
//
// Note: This example depends on a 'GetGHCRCredentials' function from utils/github_to_oci_cred.go
// and uses ORAS libraries to demonstrate pulling artifacts.

package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	// Import from your module path:
	"github.com/opengovern/resilient-bridge/utils"

	"oras.land/oras-go/v2"
	"oras.land/oras-go/v2/content/memory"
	"oras.land/oras-go/v2/registry"
	"oras.land/oras-go/v2/registry/remote"
	"oras.land/oras-go/v2/registry/remote/auth"
)

var (
	ociArtifactURIFlag string
)

func init() {
	flag.StringVar(&ociArtifactURIFlag, "oci-artifact-uri", "", "Optional OCI artifact URI to pull (e.g. ghcr.io/user/repo:tag)")
}

func main() {
	flag.Parse()

	username := os.Getenv("GH_USERNAME")
	token := os.Getenv("CR_PAT")
	if username == "" || token == "" {
		fmt.Fprintln(os.Stderr, "GH_USERNAME and CR_PAT environment variables must be set.")
		os.Exit(1)
	}

	// Acquire GHCR credentials using the utility function
	creds, err := utils.GetGHCRCredentials(username, token)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error generating GHCR credentials: %v\n", err)
		os.Exit(1)
	}

	// Convert creds map to a Docker config.json structure
	cfg := struct {
		Auths map[string]struct {
			Auth string `json:"auth"`
		} `json:"auths"`
	}{
		Auths: make(map[string]struct {
			Auth string `json:"auth"`
		}, len(creds)),
	}
	for host, encAuth := range creds {
		cfg.Auths[host] = struct {
			Auth string `json:"auth"`
		}{Auth: encAuth}
	}

	configBytes, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error marshaling config to JSON: %v\n", err)
		os.Exit(1)
	}

	// Print the generated Docker config.json to stdout
	fmt.Println("Generated Docker config.json credentials (not saved to file):")
	fmt.Println(string(configBytes))

	// If no artifact URI is provided, we're done.
	if ociArtifactURIFlag == "" {
		return
	}

	// Otherwise, try pulling the artifact using ORAS and the generated credentials
	if err := orasPull(ociArtifactURIFlag, cfg); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to pull %s: %v\n", ociArtifactURIFlag, err)
		os.Exit(1)
	} else {
		fmt.Printf("Successfully pulled %s using the provided credentials.\n", ociArtifactURIFlag)
	}
}

// orasPull pulls the given OCI artifact using the credentials in cfg.
// It uses ORAS to connect and authenticate to the registry.
func orasPull(ociArtifactURI string, cfg interface{}) error {
	ctx := context.Background()

	ref, err := registry.ParseReference(ociArtifactURI)
	if err != nil {
		return fmt.Errorf("invalid oci-artifact-uri: %w", err)
	}

	// Extract credentials from cfg (our Docker config structure)
	dockerCfg := cfg.(struct {
		Auths map[string]struct {
			Auth string `json:"auth"`
		} `json:"auths"`
	})

	credentialsFunc := auth.CredentialFunc(func(ctx context.Context, host string) (auth.Credential, error) {
		if a, ok := dockerCfg.Auths[host]; ok {
			decoded, err := base64.StdEncoding.DecodeString(a.Auth)
			if err != nil {
				return auth.Credential{}, err
			}
			parts := strings.SplitN(string(decoded), ":", 2)
			if len(parts) != 2 {
				return auth.Credential{}, fmt.Errorf("invalid auth format for %s", host)
			}
			return auth.Credential{
				Username: parts[0],
				Password: parts[1],
			}, nil
		}
		return auth.Credential{}, fmt.Errorf("no credentials for host %s", host)
	})

	authClient := &auth.Client{
		Credential: credentialsFunc,
	}

	repo, err := remote.NewRepository(ref.String())
	if err != nil {
		return fmt.Errorf("failed to create repository object: %w", err)
	}
	repo.Client = authClient

	// Pull the artifact into memory store
	memoryStore := memory.New()

	desc, err := oras.Copy(ctx, repo, ref.Reference, memoryStore, "", oras.DefaultCopyOptions)
	if err != nil {
		return fmt.Errorf("oras pull failed: %w", err)
	}

	// Fetch the manifest content.
	rc, err := memoryStore.Fetch(ctx, desc)
	if err != nil {
		return fmt.Errorf("failed to fetch manifest: %w", err)
	}
	defer rc.Close()

	manifestContent, err := io.ReadAll(rc)
	if err != nil {
		return fmt.Errorf("failed to read manifest: %w", err)
	}

	// Print the manifest JSON
	fmt.Println("Pulled artifact manifest:")
	fmt.Println(string(manifestContent))

	return nil
}
