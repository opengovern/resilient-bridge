package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"strings"

	"myapp/utils" // Adjust import path as needed to point to your utils package
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

	// 1. Use AzureSPN to get AAD token
	cfg := &utils.AzureSPNConfig{
		TenantID:     tenantID,
		ClientID:     clientID,
		ClientSecret: clientSecret,
		// AuthorityHost and Resource can be left as default if your utils sets them by default.
	}
	spn, err := utils.NewAzureSPN(cfg)
	if err != nil {
		fmt.Printf("Error creating AzureSPN: %v\n", err)
		return
	}

	ctx := context.Background()
	token, err := spn.AcquireToken(ctx)
	if err != nil {
		fmt.Printf("Error acquiring AAD token from SPN: %v\n", err)
		return
	}
	aadToken := token.AccessToken

	// 2. Exchange AAD token for ACR refresh token
	refreshToken, err := getACRRefreshToken(registry, aadToken)
	if err != nil {
		fmt.Printf("Error getting ACR refresh token: %v\n", err)
		return
	}

	// 3. Exchange ACR refresh token for ACR access token for catalog
	acrTokenForCatalog, err := getACRAccessToken(registry, refreshToken, "registry:catalog:*")
	if err != nil {
		fmt.Printf("Error getting ACR access token for catalog: %v\n", err)
		return
	}

	// List repositories
	repos, err := listRepositories(registry, acrTokenForCatalog)
	if err != nil {
		fmt.Printf("Error listing repositories: %v\n", err)
		return
	}
	fmt.Println("Repositories in registry:")
	for _, r := range repos {
		fmt.Println(" -", r)
	}

	// Now get an access token for the specific repository to list artifacts
	scope := fmt.Sprintf("repository:%s:*", repository)
	acrTokenForRepo, err := getACRAccessToken(registry, refreshToken, scope)
	if err != nil {
		fmt.Printf("Error getting ACR access token for repository: %v\n", err)
		return
	}

	// 4. List artifacts (tags) in the given repository
	artifacts, err := listArtifacts(registry, repository, acrTokenForRepo)
	if err != nil {
		fmt.Printf("Error listing artifacts: %v\n", err)
		return
	}
	fmt.Printf("Artifacts (tags) in repository %s:\n", repository)
	for _, a := range artifacts {
		fmt.Println(" -", a)
	}
}

// getACRRefreshToken exchanges AAD token for ACR refresh token
func getACRRefreshToken(registry, aadToken string) (string, error) {
	exchangeURL := fmt.Sprintf("https://%s/oauth2/exchange", registry)
	data := url.Values{}
	data.Set("grant_type", "access_token")
	data.Set("service", registry)
	data.Set("access_token", aadToken)

	req, err := http.NewRequest("POST", exchangeURL, strings.NewReader(data.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		body, _ := ioutil.ReadAll(resp.Body)
		return "", fmt.Errorf("failed to get ACR refresh token: status %d, body: %s", resp.StatusCode, body)
	}

	var tokenResp struct {
		RefreshToken string `json:"refresh_token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return "", err
	}
	return tokenResp.RefreshToken, nil
}

// getACRAccessToken exchanges ACR refresh token for ACR access token with a specific scope
func getACRAccessToken(registry, refreshToken, scope string) (string, error) {
	tokenURL := fmt.Sprintf("https://%s/oauth2/token", registry)
	data := url.Values{}
	data.Set("grant_type", "refresh_token")
	data.Set("service", registry)
	data.Set("scope", scope)
	data.Set("refresh_token", refreshToken)

	req, err := http.NewRequest("POST", tokenURL, strings.NewReader(data.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		body, _ := ioutil.ReadAll(resp.Body)
		return "", fmt.Errorf("failed to get ACR access token: status %d, body: %s", resp.StatusCode, body)
	}

	var tokenResp struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return "", err
	}
	return tokenResp.AccessToken, nil
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
