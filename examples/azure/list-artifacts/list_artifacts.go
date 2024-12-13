// list_artifacts.go
//
// This example retrieves Azure Container Registry (ACR) tokens using a Service Principal (SPN)
// and then uses those tokens to list repositories and artifacts (tags) from an ACR.
// It uses utilities from the "github.com/opengovern/resilient-bridge/utils" package for SPN authentication
// and token exchanges.

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"

	"github.com/opengovern/resilient-bridge/utils" // Updated import path
)

func main() {
	tenantID := os.Getenv("AZURE_TENANT_ID")
	clientID := os.Getenv("AZURE_CLIENT_ID")
	clientSecret := os.Getenv("AZURE_CLIENT_SECRET")
	registry := os.Getenv("ACR_REGISTRY_LOGIN_URI") // e.g. "myregistry.azurecr.io"
	repository := os.Getenv("ACR_REPOSITORY_NAME")  // e.g. "myrepo"

	if tenantID == "" || clientID == "" || clientSecret == "" || registry == "" || repository == "" {
		fmt.Println("Please set AZURE_TENANT_ID, AZURE_CLIENT_ID, AZURE_CLIENT_SECRET, ACR_REGISTRY_LOGIN_URI, and ACR_REPOSITORY_NAME environment variables.")
		return
	}

	// 1. Create an AzureSPN instance for getting AAD tokens
	spnCfg := &utils.AzureSPNConfig{
		TenantID:     tenantID,
		ClientID:     clientID,
		ClientSecret: clientSecret,
	}
	spn, err := utils.NewAzureSPN(spnCfg)
	if err != nil {
		fmt.Printf("Error creating AzureSPN: %v\n", err)
		return
	}

	ctx := context.Background()

	// 2. Create an SPNToACR instance to convert SPN credentials to ACR tokens
	// For listing repos: scope = "registry:catalog:*"
	spnToACRForCatalog := utils.NewSPNToACR(spn, registry, "registry:catalog:*")

	// Get Docker credentials (ACR access token) for catalog operations
	catalogCreds, err := spnToACRForCatalog.GetACRDockerCredentials(ctx)
	if err != nil {
		fmt.Printf("Error getting ACR docker credentials for catalog: %v\n", err)
		return
	}

	// List repositories
	repos, err := listRepositories(registry, catalogCreds.Password)
	if err != nil {
		fmt.Printf("Error listing repositories: %v\n", err)
		return
	}
	fmt.Println("Repositories in registry:")
	for _, r := range repos {
		fmt.Println(" -", r)
	}

	// For listing artifacts in a given repository: scope = "repository:<repository>:*"
	spnToACRForRepo := utils.NewSPNToACR(spn, registry, fmt.Sprintf("repository:%s:*", repository))
	repoCreds, err := spnToACRForRepo.GetACRDockerCredentials(ctx)
	if err != nil {
		fmt.Printf("Error getting ACR docker credentials for repository: %v\n", err)
		return
	}

	// 4. List artifacts (tags) in the given repository
	artifacts, err := listArtifacts(registry, repository, repoCreds.Password)
	if err != nil {
		fmt.Printf("Error listing artifacts: %v\n", err)
		return
	}
	fmt.Printf("Artifacts (tags) in repository %s:\n", repository)
	for _, a := range artifacts {
		fmt.Println(" -", a)
	}
}

// listRepositories lists repositories from ACR
func listRepositories(registry, acrToken string) ([]string, error) {
	url := fmt.Sprintf("https://%s/acr/v1/_catalog", registry)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+acrToken)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		body, _ := ioutil.ReadAll(resp.Body)
		return nil, fmt.Errorf("failed to list repositories: status %d, body: %s", resp.StatusCode, body)
	}

	var reposResp struct {
		Repositories []string `json:"repositories"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&reposResp); err != nil {
		return nil, err
	}
	return reposResp.Repositories, nil
}

// listArtifacts lists tags in a repository (artifacts) using the _tags endpoint
func listArtifacts(registry, repository, acrToken string) ([]string, error) {
	url := fmt.Sprintf("https://%s/acr/v1/%s/_tags", registry, repository)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+acrToken)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		body, _ := ioutil.ReadAll(resp.Body)
		return nil, fmt.Errorf("failed to list artifacts: status %d, body: %s", resp.StatusCode, body)
	}

	var tagsResp struct {
		Tags []struct {
			Name string `json:"name"`
		} `json:"tags"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&tagsResp); err != nil {
		return nil, err
	}

	var artifacts []string
	for _, t := range tagsResp.Tags {
		artifacts = append(artifacts, t.Name)
	}
	return artifacts, nil
}
