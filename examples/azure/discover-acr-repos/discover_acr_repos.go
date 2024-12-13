// main.go
//
// This example retrieves Azure Container Registry (ACR) tokens using a Service Principal (SPN)
// and then lists repositories from that ACR using utilities from the utils package.

package main

import (
	"context"
	"fmt"
	"os"

	"encoding/json"
	"io/ioutil"
	"net/http"

	"github.com/opengovern/resilient-bridge/utils" // Adjust the path if needed
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

	// Create SPN config
	spnCfg := &utils.AzureSPNConfig{
		TenantID:     tenantID,
		ClientID:     clientID,
		ClientSecret: clientSecret,
	}

	// Initialize Azure SPN object
	spn, err := utils.NewAzureSPN(spnCfg)
	if err != nil {
		fmt.Printf("Error creating AzureSPN: %v\n", err)
		return
	}

	ctx := context.Background()

	// Create SPNToACR with "registry:catalog:*" scope to list repositories
	spnToACR := utils.NewSPNToACR(spn, registry, "registry:catalog:*")

	// Acquire ACR Docker credentials (token)
	dockerCreds, err := spnToACR.GetACRDockerCredentials(ctx)
	if err != nil {
		fmt.Printf("Error getting ACR Docker credentials: %v\n", err)
		return
	}

	// Now list repositories using the acquired token
	repos, err := listRepositories(registry, dockerCreds.Password) // dockerCreds.Password should be the ACR access token
	if err != nil {
		fmt.Printf("Error listing repositories: %v\n", err)
		return
	}

	fmt.Println("Repositories in registry:")
	for _, r := range repos {
		fmt.Println("-", r)
	}
}

// listRepositories lists repositories from ACR using the provided ACR token.
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
