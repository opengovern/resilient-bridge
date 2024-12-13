package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"strings"
)

func main() {
	tenantID := os.Getenv("AZURE_TENANT_ID")
	clientID := os.Getenv("AZURE_CLIENT_ID")
	clientSecret := os.Getenv("AZURE_CLIENT_SECRET")
	registry := os.Getenv("ACR_REGISTRY_NAME")     // e.g. "myregistry.azurecr.io"
	repository := os.Getenv("ACR_REPOSITORY_NAME") // e.g. "myrepo"

	// 1. Get Azure AD token
	aadToken, err := getAADToken(tenantID, clientID, clientSecret)
	if err != nil {
		fmt.Printf("Error getting AAD token: %v\n", err)
		return
	}

	// 2. Exchange AAD token for ACR refresh token
	refreshToken, err := getACRRefreshToken(registry, aadToken)
	if err != nil {
		fmt.Printf("Error getting ACR refresh token: %v\n", err)
		return
	}

	// 3. Exchange ACR refresh token for ACR access token (for "registry:catalog:*" and "repository:*" actions)
	// For listing repositories: scope = "registry:catalog:*"
	// For listing artifacts in a repository: scope = "repository:{repository}:*"
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

	// Now get an access token for the specific repository
	scope := fmt.Sprintf("repository:%s:*", repository)
	acrTokenForRepo, err := getACRAccessToken(registry, refreshToken, scope)
	if err != nil {
		fmt.Printf("Error getting ACR access token for repository: %v\n", err)
		return
	}

	// 4. List artifacts (manifests) in the given repository
	// For simplicity, we'll list tags (if available) from _tags endpoint or manifests from _manifests endpoint.
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

func getAADToken(tenantID, clientID, clientSecret string) (string, error) {
	tokenURL := "https://login.microsoftonline.com/" + tenantID + "/oauth2/v2.0/token"
	data := url.Values{}
	data.Set("grant_type", "client_credentials")
	data.Set("client_id", clientID)
	data.Set("client_secret", clientSecret)
	data.Set("scope", "https://management.azure.com/.default")

	resp, err := http.PostForm(tokenURL, data)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		body, _ := ioutil.ReadAll(resp.Body)
		return "", fmt.Errorf("failed to get AAD token: status %d, body: %s", resp.StatusCode, body)
	}

	var tokenResp struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return "", err
	}
	return tokenResp.AccessToken, nil
}

// getACRRefreshToken exchanges AAD token for ACR refresh token
func getACRRefreshToken(registry, aadToken string) (string, error) {
	// POST https://{registry}/oauth2/exchange
	// Content-Type: application/x-www-form-urlencoded
	// grant_type=access_token
	// service={registry}
	// access_token={AAD token}
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
	// POST https://{registry}/oauth2/token
	// grant_type=refresh_token
	// service={registry}
	// scope={scope} e.g. "registry:catalog:*"
	// refresh_token={refresh_token}
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

// listArtifacts lists tags in a repository (artifacts) using _tags endpoint
func listArtifacts(registry, repository, acrToken string) ([]string, error) {
	// For listing artifact versions (tags), we can use the _tags endpoint:
	// GET https://{registry}/acr/v1/{repository}/_tags
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
