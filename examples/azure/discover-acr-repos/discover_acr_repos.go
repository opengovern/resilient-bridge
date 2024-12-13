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
	registry := os.Getenv("ACR_REGISTRY_LOGIN_URI")

	if tenantID == "" || clientID == "" || clientSecret == "" || registry == "" {
		fmt.Println("Please set AZURE_TENANT_ID, AZURE_CLIENT_ID, AZURE_CLIENT_SECRET, and ACR_REGISTRY_LOGIN_URI environment variables.")
		return
	}

	aadToken, err := getAADToken(tenantID, clientID, clientSecret)
	if err != nil {
		fmt.Printf("Error obtaining AAD token: %v\n", err)
		return
	}

	refreshToken, err := getACRRefreshToken(registry, aadToken)
	if err != nil {
		fmt.Printf("Error obtaining ACR refresh token: %v\n", err)
		return
	}

	acrToken, err := getACRAccessToken(registry, refreshToken, "registry:catalog:*")
	if err != nil {
		fmt.Printf("Error obtaining ACR access token: %v\n", err)
		return
	}

	repos, err := listRepositories(registry, acrToken)
	if err != nil {
		fmt.Printf("Error listing repositories: %v\n", err)
		return
	}

	fmt.Println("Repositories in registry:")
	for _, r := range repos {
		fmt.Println("-", r)
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
		return "", fmt.Errorf("failed to get AAD token: status %d, body: %s", resp.StatusCode, string(body))
	}

	var tokenResp struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return "", err
	}
	return tokenResp.AccessToken, nil
}

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
		return "", fmt.Errorf("failed to get ACR refresh token: status %d, body: %s", resp.StatusCode, string(body))
	}

	var tokenResp struct {
		RefreshToken string `json:"refresh_token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return "", err
	}
	return tokenResp.RefreshToken, nil
}

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
		return "", fmt.Errorf("failed to get ACR access token: status %d, body: %s", resp.StatusCode, string(body))
	}

	var tokenResp struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return "", err
	}
	return tokenResp.AccessToken, nil
}

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
		return nil, fmt.Errorf("failed to list repositories: status %d, body: %s", resp.StatusCode, string(body))
	}

	var reposResp struct {
		Repositories []string `json:"repositories"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&reposResp); err != nil {
		return nil, err
	}
	return reposResp.Repositories, nil
}