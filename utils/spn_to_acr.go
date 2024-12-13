// utils/spn_to_acr.go:
// This file builds on azure_spn.go to facilitate authentication against Azure Container Registry (ACR).
// It leverages the AAD token obtained by SPN credentials to get an ACR refresh token and then an ACR access token.
// Finally, it converts these tokens into Docker/OCI-compatible credentials (username/password) that can be used to 'docker login' to ACR.
// Example usage: Generate Docker credentials from SPN credentials for pushing/pulling images from ACR.
package utils

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"strings"
)

// ACRCredentials holds the Docker/OCI credentials for ACR authentication
type ACRCredentials struct {
	Username string
	Password string // This will be the ACR Access Token
}

// SPNToACR provides methods to get ACR tokens (refresh + access) from SPN credentials
type SPNToACR struct {
	SPN        *AzureSPN
	Registry   string // e.g. "myregistry.azurecr.io"
	Scope      string // e.g. "registry:catalog:*" or "repository:<repo>:*"
	HTTPClient *http.Client
}

// NewSPNToACR creates a new SPNToACR object
func NewSPNToACR(spn *AzureSPN, registry, scope string) *SPNToACR {
	return &SPNToACR{
		SPN:        spn,
		Registry:   registry,
		Scope:      scope,
		HTTPClient: http.DefaultClient,
	}
}

// GetACRDockerCredentials gets Docker/OCI credentials for ACR from an SPN
//  1. Acquire AAD Token via SPN
//  2. Exchange AAD token for ACR Refresh token
//  3. Exchange ACR refresh token for ACR Access token (scope-based)
//  4. Return ACRCredentials with username = "00000000-0000-0000-0000-000000000000" (standard username for ACR token auth)
//     and password = ACR access token
func (a *SPNToACR) GetACRDockerCredentials(ctx context.Context) (*ACRCredentials, error) {
	// Step 1: Acquire AAD token
	token, err := a.SPN.AcquireToken(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to acquire AAD token: %w", err)
	}
	aadToken := token.AccessToken

	// Step 2: Exchange AAD token for ACR Refresh Token
	refreshToken, err := a.exchangeAADForACRRefreshToken(aadToken)
	if err != nil {
		return nil, fmt.Errorf("failed to get ACR refresh token: %w", err)
	}

	// Step 3: Exchange ACR refresh token for ACR Access Token
	accessToken, err := a.exchangeRefreshForACRAccessToken(refreshToken, a.Scope)
	if err != nil {
		return nil, fmt.Errorf("failed to get ACR access token: %w", err)
	}

	// ACR uses a dummy username for Docker login when using tokens
	// Typically "00000000-0000-0000-0000-000000000000"
	return &ACRCredentials{
		Username: "00000000-0000-0000-0000-000000000000",
		Password: accessToken,
	}, nil
}

// exchangeAADForACRRefreshToken exchanges AAD token for ACR refresh token
func (a *SPNToACR) exchangeAADForACRRefreshToken(aadToken string) (string, error) {
	exchangeURL := fmt.Sprintf("https://%s/oauth2/exchange", a.Registry)
	data := url.Values{}
	data.Set("grant_type", "access_token")
	data.Set("service", a.Registry)
	data.Set("access_token", aadToken)

	req, err := http.NewRequest("POST", exchangeURL, strings.NewReader(data.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := a.HTTPClient.Do(req)
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

// exchangeRefreshForACRAccessToken exchanges ACR refresh token for ACR access token with a specific scope
func (a *SPNToACR) exchangeRefreshForACRAccessToken(refreshToken, scope string) (string, error) {
	tokenURL := fmt.Sprintf("https://%s/oauth2/token", a.Registry)
	data := url.Values{}
	data.Set("grant_type", "refresh_token")
	data.Set("service", a.Registry)
	data.Set("scope", scope)
	data.Set("refresh_token", refreshToken)

	req, err := http.NewRequest("POST", tokenURL, strings.NewReader(data.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := a.HTTPClient.Do(req)
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
