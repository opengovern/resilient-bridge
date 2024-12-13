// Package utils provides various utility functions for working with OCI artifacts.
//
// This file provides a library function to pull an OCI/Docker image (artifact) from a registry
// using provided credentials and store the pulled content into a specified local directory.
//
// It uses the ORAS Go library to interact with OCI registries, and supports Docker/OCI credentials.
// The credentials map is expected to have keys as registry hosts (e.g., "ghcr.io") and values as
// base64-encoded "username:password" strings.
//
// Example usage:
//
//   creds := map[string]string{
//     "ghcr.io": "base64encoded(username:token)",
//   }
//   err := PullArtifact(ctx, "ghcr.io/user/repo:tag", creds, "/path/to/output")
//   if err != nil {
//     // handle error
//   }
//
// This will connect to ghcr.io with the given credentials and pull the specified artifact,
// storing its files in the provided output directory.
//
// Note: The pulled files and manifest depend on the content of the artifact. For simple container
// images, you may get a config file, a manifest, and layers stored locally.
//
// Make sure you have ORAS libraries and appropriate imports set up:
//   go get oras.land/oras-go/v2

package utils

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"oras.land/oras-go/v2"
	"oras.land/oras-go/v2/content/file"
	"oras.land/oras-go/v2/registry"
	"oras.land/oras-go/v2/registry/remote"
	"oras.land/oras-go/v2/registry/remote/auth"
)

// PullArtifact pulls an OCI/Docker artifact from the given URI using the provided credentials,
// and stores the artifact files in the specified output directory.
//
// Parameters:
//   - ctx: A context for cancellation and deadlines.
//   - ociArtifactURI: A string representing the URI of the artifact (e.g. "ghcr.io/user/repo:tag").
//   - creds: A map of registry host to base64-encoded auth ("username:password").
//   - outputDir: The local directory where the pulled artifact's files will be stored.
//
// Returns:
//   - error: If any error occurs during the pull process.
func PullArtifact(ctx context.Context, ociArtifactURI string, creds map[string]string, outputDir string) error {
	if ociArtifactURI == "" {
		return errors.New("ociArtifactURI cannot be empty")
	}
	if outputDir == "" {
		return errors.New("outputDir cannot be empty")
	}

	ref, err := registry.ParseReference(ociArtifactURI)
	if err != nil {
		return fmt.Errorf("invalid artifact URI %q: %w", ociArtifactURI, err)
	}

	// Setup credentials function for the registry
	credentialsFunc := auth.CredentialFunc(func(ctx context.Context, host string) (auth.Credential, error) {
		if encodedAuth, ok := creds[host]; ok {
			decoded, err := base64.StdEncoding.DecodeString(encodedAuth)
			if err != nil {
				return auth.Credential{}, fmt.Errorf("failed to decode auth for %s: %w", host, err)
			}
			parts := strings.SplitN(string(decoded), ":", 2)
			if len(parts) != 2 {
				return auth.Credential{}, fmt.Errorf("invalid auth format for %s, expected username:password", host)
			}
			return auth.Credential{
				Username: parts[0],
				Password: parts[1],
			}, nil
		}
		// No credentials found for this host
		return auth.Credential{}, nil
	})

	authClient := &auth.Client{
		Credential: credentialsFunc,
	}

	repo, err := remote.NewRepository(ref.String())
	if err != nil {
		return fmt.Errorf("failed to create repository object for %s: %w", ref.String(), err)
	}
	repo.Client = authClient

	// Create a file store pointing to outputDir
	// This will store the pulled content.
	if err := ensureDir(outputDir); err != nil {
		return fmt.Errorf("failed to ensure output directory %q: %w", outputDir, err)
	}

	fs, err := file.New(outputDir)
	if err != nil {
		return fmt.Errorf("failed to create file store at %q: %w", outputDir, err)
	}
	defer fs.Close()

	// Pull the artifact using ORAS
	_, err = oras.Copy(ctx, repo, ref.Reference, fs, "", oras.DefaultCopyOptions)
	if err != nil {
		return fmt.Errorf("oras pull failed: %w", err)
	}

	return nil
}

// ensureDir makes sure the directory exists, creating it if not.
func ensureDir(dir string) error {
	if dir == "" {
		return errors.New("dir not specified")
	}
	return os.MkdirAll(filepath.Clean(dir), 0755)
}
