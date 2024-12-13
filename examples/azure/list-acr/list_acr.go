package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
)

func main() {
	tenantID := os.Getenv("AZURE_TENANT_ID")         // or your tenant id
	clientID := os.Getenv("AZURE_CLIENT_ID")         // or your client/application id
	clientSecret := os.Getenv("AZURE_CLIENT_SECRET") // or your client secret
	subscriptionID := os.Getenv("AZURE_SUBSCRIPTION_ID")

	// 1. Acquire a token from Azure AD using client credentials
	token, err := getAzureToken(tenantID, clientID, clientSecret)
	if err != nil {
		fmt.Printf("Error getting token: %v\n", err)
		return
	}

	// 2. Use the token to call Azure Container Registry Management REST API
	registries, err := listContainerRegistries(subscriptionID, token)
	if err != nil {
		fmt.Printf("Error listing registries: %v\n", err)
		return
	}

	fmt.Println("Container Registries:")
	for _, reg := range registries.Value {
		fmt.Printf("- Name: %s, Location: %s, ID: %s\n", reg.Name, reg.Location, reg.ID)
	}
}

// getAzureToken acquires an OAuth2 token from Azure AD using client credentials.
func getAzureToken(tenantID, clientID, clientSecret string) (string, error) {
	tokenURL := "https://login.microsoftonline.com/" + tenantID + "/oauth2/v2.0/token"

	data := url.Values{}
	data.Set("grant_type", "client_credentials")
	data.Set("client_id", clientID)
	data.Set("client_secret", clientSecret)
	// Resource or scope for ARM
	// For Azure Resource Manager, you often use scope as "https://management.azure.com/.default"
	data.Set("scope", "https://management.azure.com/.default")

	resp, err := http.PostForm(tokenURL, data)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		bodyBytes, _ := ioutil.ReadAll(resp.Body)
		return "", fmt.Errorf("failed to get token, status: %d, body: %s", resp.StatusCode, string(bodyBytes))
	}

	var tokenResp struct {
		AccessToken string `json:"access_token"`
	}
	err = json.NewDecoder(resp.Body).Decode(&tokenResp)
	if err != nil {
		return "", err
	}

	return tokenResp.AccessToken, nil
}

// listContainerRegistries lists the container registries for a subscription.
func listContainerRegistries(subscriptionID, accessToken string) (*RegistryListResult, error) {
	url := "https://management.azure.com/subscriptions/" + subscriptionID + "/providers/Microsoft.ContainerRegistry/registries?api-version=2023-01-01-preview"

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Accept", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		bodyBytes, _ := ioutil.ReadAll(resp.Body)
		return nil, fmt.Errorf("failed to list registries, status: %d, body: %s", resp.StatusCode, string(bodyBytes))
	}

	var result RegistryListResult
	err = json.NewDecoder(resp.Body).Decode(&result)
	if err != nil {
		return nil, err
	}

	return &result, nil
}

// RegistryListResult represents the result of a request to list container registries.
type RegistryListResult struct {
	Value []Registry `json:"value"`
}

// Registry represents a container registry.
type Registry struct {
	ID         string            `json:"id"`
	Name       string            `json:"name"`
	Type       string            `json:"type"`
	Location   string            `json:"location"`
	Tags       map[string]string `json:"tags"`
	Properties struct {
		LoginServer string `json:"loginServer"`
		// ... Add other fields if needed
	} `json:"properties"`
	Sku struct {
		Name string `json:"name"`
		Tier string `json:"tier"`
	} `json:"sku"`
}

// Example usage (in terminal):
// export AZURE_TENANT_ID="your-tenant-id"
// export AZURE_CLIENT_ID="your-client-id"
// export AZURE_CLIENT_SECRET="your-client-secret"
// export AZURE_SUBSCRIPTION_ID="your-subscription-id"
// go run main.go
