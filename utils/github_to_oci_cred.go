// github_to_oci_cred.go
//
// Package utils provides utility functions and helpers for authenticating and interacting with various services.
//
// This file provides a helper function to generate OCI registry credentials for GitHub Container Registry (GHCR)
// using a GitHub username and Personal Access Token (PAT).
//
// The function returns a map of registry hostnames to a base64-encoded "username:password" string suitable for inclusion
// in a Docker config.json file or other OCI-compatible credential stores.
//
// Example usage:
//   creds, err := GetGHCRCredentials("my-username", "my-gh-pat")
//   if err != nil {
//       // handle error
//   }
//   // creds["ghcr.io"] will contain a base64-encoded auth string

package utils

import (
	"encoding/base64"
	"fmt"
)

// GetGHCRCredentials generates OCI credentials for GHCR from a username and PAT.
//
// username: Your GitHub username.
// token:    A Personal Access Token (PAT) with appropriate scopes for accessing GHCR.
//
// Returns a map[string]string where the key is the hostname (e.g. "ghcr.io") and the value
// is the base64-encoded "username:password" auth string.
//
// If username or token is empty, it returns an error.
func GetGHCRCredentials(username, token string) (map[string]string, error) {
	if username == "" || token == "" {
		return nil, fmt.Errorf("both username and token are required for GHCR credentials")
	}

	authStr := username + ":" + token
	encoded := base64.StdEncoding.EncodeToString([]byte(authStr))

	// GHCR uses "ghcr.io" as its registry hostname
	return map[string]string{
		"ghcr.io": encoded,
	}, nil
}
