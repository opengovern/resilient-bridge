// Package utils provides a utility function that accepts a JSON input containing one or more credential configurations
// for different container registries (Azure ACR via SPN Password or SPN Certificate, GitHub Container Registry (GHCR),
// and DockerHub), and returns OCI-compatible (Docker) credentials.
//
// JSON Input Structure (example):
//
//	{
//	  "azure_spn_password": {
//	    "tenant_id": "your-tenant-id",
//	    "client_id": "your-client-id",
//	    "client_secret": "your-client-secret",
//	    "registry": "yourregistry.azurecr.io"
//	  },
//	  "azure_spn_certificate": {
//	    "tenant_id": "your-tenant-id",
//	    "client_id": "your-client-id",
//	    "cert_path": "/path/to/cert.pfx",
//	    "cert_password": "cert-pass",
//	    "registry": "yourregistry.azurecr.io"
//	  },
//	  "github": {
//	    "username": "your-github-username",
//	    "token": "your-github-pat"
//	  },
//	  "dockerhub": {
//	    "username": "your-dockerhub-username",
//	    "token": "your-dockerhub-token"
//	  }
//	}
//
// Each field is optional, but if present, must provide the required sub-fields as noted above.
// The returned map is a set of registry hostnames to base64-encoded "username:password" strings suitable
// for inclusion in a Docker config.json or other OCI-compatible credential store.
//
// Example returned map keys/values:
//
//	"ghcr.io" -> base64("username:token")
//	"index.docker.io" -> base64("username:token")
//	"<yourregistry>.azurecr.io" -> base64("00000000-0000-0000-0000-000000000000:<access_token>")
//
// This utility replaces older files such as spn_to_acr.go and github_to_oci_cred.go by consolidating
// all credential acquisition logic into a single entry point.
//
// Usage:
//
//	import "github.com/opengovern/resilient-bridge/utils"
//
//	jsonData := []byte(`{...}`) // JSON as described above
//	creds, err := utils.GetAllCredentials(jsonData)
//	if err != nil {
//	  panic(err)
//	}
//
//	// creds can now be used to populate Docker config.json or similar.
//
// Azure notes:
// To use azure_spn_password or azure_spn_certificate, ensure the Service Principal (SPN) has been granted
// appropriate access (e.g., acrpull or acrpush roles) to the ACR. Authentication uses the standard
// Azure OAuth2 flow:
//   - For password: Uses client_id/client_secret
//   - For certificate: Uses client assertion (JWT signed by SPN’s certificate)
//
// GitHub (GHCR) and DockerHub are straightforward username/token pairs.
// Package utils provides a utility function that accepts a JSON input containing one or more credential configurations
// for different container registries (Azure ACR via SPN Password or SPN Certificate, GitHub Container Registry (GHCR),
// and DockerHub), and returns OCI-compatible (Docker) credentials.
//
// JSON Input Structure (example):
//
//	{
//	  "azure_spn_password": {
//	    "tenant_id": "your-tenant-id",
//	    "client_id": "your-client-id",
//	    "client_secret": "your-client-secret",
//	    "registry": "yourregistry.azurecr.io"
//	  },
//	  "azure_spn_certificate": {
//	    "tenant_id": "your-tenant-id",
//	    "client_id": "your-client-id",
//	    "cert_path": "/path/to/cert.pfx",
//	    "cert_password": "cert-pass",
//	    "registry": "yourregistry.azurecr.io"
//	  },
//	  "github": {
//	    "username": "your-github-username",
//	    "token": "your-github-pat"
//	  },
//	  "dockerhub": {
//	    "username": "your-dockerhub-username",
//	    "token": "your-dockerhub-token"
//	  }
//	}
//
// Each field is optional, but if present, must provide the required sub-fields as noted above.
// The returned map is a set of registry hostnames to base64-encoded "username:password" strings suitable
// for inclusion in a Docker config.json or other OCI-compatible credential store.
//
// Example returned map keys/values:
//
//	"ghcr.io" -> base64("username:token")
//	"index.docker.io" -> base64("username:token")
//	"<yourregistry>.azurecr.io" -> base64("00000000-0000-0000-0000-000000000000:<access_token>")
//
// This utility replaces older files such as spn_to_acr.go and github_to_oci_cred.go by consolidating
// all credential acquisition logic into a single entry point.
//
// Usage:
//
//	import "github.com/opengovern/resilient-bridge/utils"
//
//	jsonData := []byte(`{...}`) // JSON as described above
//	creds, err := utils.GetAllCredentials(jsonData, "repository:myrepo:pull")
//	if err != nil {
//	  panic(err)
//	}
//
//	// creds can now be used to populate Docker config.json or similar.
//
// Azure notes:
// To use azure_spn_password or azure_spn_certificate, ensure the Service Principal (SPN) has been granted
// appropriate access (e.g., acrpull or acrpush roles) to the ACR. The flow is now:
//   - Acquire a general AAD token for https://management.azure.com/.default
//   - Exchange that AAD token for ACR refresh token
//   - Exchange refresh token for ACR access token with desired scope
//
// GitHub (GHCR) and DockerHub are straightforward username/token pairs.
package utils

import (
	"context"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v4"
	"golang.org/x/crypto/pkcs12"
)

// AzureSPNPasswordCredentials holds fields required for SPN password-based credential retrieval.
type AzureSPNPasswordCredentials struct {
	TenantID     string `json:"tenant_id"`
	ClientID     string `json:"client_id"`
	ClientSecret string `json:"client_secret"`
	Registry     string `json:"registry"`
}

// AzureSPNCertificateCredentials holds fields required for SPN certificate-based credential retrieval.
type AzureSPNCertificateCredentials struct {
	TenantID     string `json:"tenant_id"`
	ClientID     string `json:"client_id"`
	CertPath     string `json:"cert_path"`
	CertPassword string `json:"cert_password"`
	Registry     string `json:"registry"`
}

// GitHubCredentials for GitHub Container Registry (GHCR).
type GitHubCredentials struct {
	Username string `json:"username"`
	Token    string `json:"token"`
}

// DockerHubCredentials for DockerHub registry.
type DockerHubCredentials struct {
	Username string `json:"username"`
	Token    string `json:"token"`
}

// CredentialsInput is the combined structure for all supported credential types.
type CredentialsInput struct {
	AzureSPNPassword    *AzureSPNPasswordCredentials    `json:"azure_spn_password,omitempty"`
	AzureSPNCertificate *AzureSPNCertificateCredentials `json:"azure_spn_certificate,omitempty"`
	GitHub              *GitHubCredentials              `json:"github,omitempty"`
	DockerHub           *DockerHubCredentials           `json:"dockerhub,omitempty"`
}

// GetAllCredentials takes a JSON byte slice and a scope (e.g., `repository:myrepo:pull`).
// If scope is empty and we have Azure credentials, it defaults to "registry:catalog:*".
// Returns a map of registry -> base64("username:password") credentials.
func GetAllCredentials(jsonData []byte, scope string) (map[string]string, error) {
	var input CredentialsInput
	if err := json.Unmarshal(jsonData, &input); err != nil {
		return nil, fmt.Errorf("failed to decode input JSON: %w", err)
	}

	creds := make(map[string]string)

	// GitHub (GHCR)
	if input.GitHub != nil {
		if input.GitHub.Username == "" || input.GitHub.Token == "" {
			return nil, fmt.Errorf("GitHub username and token are required")
		}
		authStr := input.GitHub.Username + ":" + input.GitHub.Token
		encoded := base64.StdEncoding.EncodeToString([]byte(authStr))
		creds["ghcr.io"] = encoded
	}

	// DockerHub
	if input.DockerHub != nil {
		if input.DockerHub.Username == "" || input.DockerHub.Token == "" {
			return nil, fmt.Errorf("DockerHub username and token are required")
		}
		authStr := input.DockerHub.Username + ":" + input.DockerHub.Token
		encoded := base64.StdEncoding.EncodeToString([]byte(authStr))
		creds["index.docker.io"] = encoded
	}

	// Default scope for Azure if none provided
	if scope == "" {
		scope = "registry:catalog:*"
	}

	// Azure SPN (Password)
	if input.AzureSPNPassword != nil {
		spnCreds, err := getAzureACRResourceScopedToken(
			input.AzureSPNPassword.TenantID,
			input.AzureSPNPassword.ClientID,
			input.AzureSPNPassword.ClientSecret,
			"", // no cert path
			"", // no cert password
			input.AzureSPNPassword.Registry,
			scope,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to get ACR token with SPN password: %w", err)
		}
		creds[input.AzureSPNPassword.Registry] = spnCreds
	}

	// Azure SPN (Certificate)
	if input.AzureSPNCertificate != nil {
		spnCreds, err := getAzureACRResourceScopedToken(
			input.AzureSPNCertificate.TenantID,
			input.AzureSPNCertificate.ClientID,
			"", // no client secret
			input.AzureSPNCertificate.CertPath,
			input.AzureSPNCertificate.CertPassword,
			input.AzureSPNCertificate.Registry,
			scope,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to get ACR token with SPN certificate: %w", err)
		}
		creds[input.AzureSPNCertificate.Registry] = spnCreds
	}

	return creds, nil
}

// getAzureACRResourceScopedToken performs a two-step approach:
// 1. Get a general AAD token (resource: https://management.azure.com/.default)
// 2. Exchange AAD token for ACR refresh token, then exchange refresh token for resource-scoped ACR access token.
func getAzureACRResourceScopedToken(tenantID, clientID, clientSecret, certPath, certPassword, registry, scope string) (string, error) {
	// 1. Acquire a general AAD token
	generalToken, err := acquireAADToken(tenantID, clientID, clientSecret, certPath, certPassword, "https://management.azure.com/.default")
	if err != nil {
		return "", fmt.Errorf("failed to get AAD token: %w", err)
	}

	// 2. Exchange AAD token for ACR Refresh Token
	refreshToken, err := exchangeAADForACRRefreshToken(registry, generalToken)
	if err != nil {
		return "", fmt.Errorf("failed to get ACR refresh token: %w", err)
	}

	// 3. Exchange ACR refresh token for ACR Access Token (with the given scope)
	acrAccessToken, err := exchangeRefreshForACRAccessToken(registry, refreshToken, scope)
	if err != nil {
		return "", fmt.Errorf("failed to get ACR access token: %w", err)
	}

	// Return base64("username:password")
	username := "00000000-0000-0000-0000-000000000000"
	authStr := username + ":" + acrAccessToken
	return base64.StdEncoding.EncodeToString([]byte(authStr)), nil
}

// acquireAADToken gets a token from AAD using either client_secret or certificate-based auth.
// The resource should be something like "https://management.azure.com/.default"
func acquireAADToken(tenantID, clientID, clientSecret, certPath, certPassword, resource string) (string, error) {
	var form url.Values
	tokenEndpoint := "https://login.microsoftonline.com/" + tenantID + "/oauth2/v2.0/token"

	if certPath != "" {
		// Certificate-based authentication
		jwtAssertion, err := buildClientAssertionFromCert(tokenEndpoint, clientID, certPath, certPassword)
		if err != nil {
			return "", fmt.Errorf("failed to build client assertion: %w", err)
		}

		form = url.Values{}
		form.Set("grant_type", "client_credentials")
		form.Set("client_id", clientID)
		form.Set("scope", resource)
		form.Set("client_assertion_type", "urn:ietf:params:oauth:client-assertion-type:jwt-bearer")
		form.Set("client_assertion", jwtAssertion)
	} else {
		// Password-based (client secret) authentication
		if clientSecret == "" {
			return "", fmt.Errorf("client secret is required for password-based auth")
		}
		form = url.Values{}
		form.Set("grant_type", "client_credentials")
		form.Set("client_id", clientID)
		form.Set("client_secret", clientSecret)
		form.Set("scope", resource)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, "POST", tokenEndpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return "", fmt.Errorf("failed to create token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	httpClient := &http.Client{Timeout: 10 * time.Second}
	resp, err := httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to execute token request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("received non-200 response: %d - %s", resp.StatusCode, string(bodyBytes))
	}

	var tokenResp struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return "", fmt.Errorf("failed to decode token response: %w", err)
	}

	return tokenResp.AccessToken, nil
}

// exchangeAADForACRRefreshToken exchanges the AAD token for an ACR refresh token
func exchangeAADForACRRefreshToken(registry, aadToken string) (string, error) {
	exchangeURL := fmt.Sprintf("https://%s/oauth2/exchange", registry)
	data := url.Values{}
	data.Set("grant_type", "access_token")
	data.Set("service", registry)
	data.Set("access_token", aadToken)

	req, err := http.NewRequest("POST", exchangeURL, strings.NewReader(data.Encode()))
	if err != nil {
		return "", fmt.Errorf("failed to create request for ACR refresh token: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	httpClient := &http.Client{Timeout: 10 * time.Second}
	resp, err := httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to execute ACR refresh token request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("failed to get ACR refresh token: status %d, body: %s", resp.StatusCode, body)
	}

	var tokenResp struct {
		RefreshToken string `json:"refresh_token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return "", fmt.Errorf("failed to decode ACR refresh token response: %w", err)
	}
	return tokenResp.RefreshToken, nil
}

// exchangeRefreshForACRAccessToken exchanges the ACR refresh token for an ACR access token with a specific scope
func exchangeRefreshForACRAccessToken(registry, refreshToken, scope string) (string, error) {
	tokenURL := fmt.Sprintf("https://%s/oauth2/token", registry)
	data := url.Values{}
	data.Set("grant_type", "refresh_token")
	data.Set("service", registry)
	data.Set("scope", scope)
	data.Set("refresh_token", refreshToken)

	req, err := http.NewRequest("POST", tokenURL, strings.NewReader(data.Encode()))
	if err != nil {
		return "", fmt.Errorf("failed to create request for ACR access token: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	httpClient := &http.Client{Timeout: 10 * time.Second}
	resp, err := httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to execute ACR access token request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("failed to get ACR access token: status %d, body: %s", resp.StatusCode, body)
	}

	var tokenResp struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return "", fmt.Errorf("failed to decode ACR access token response: %w", err)
	}
	return tokenResp.AccessToken, nil
}

// buildClientAssertionFromCert creates a signed JWT to use as a client assertion with a certificate.
func buildClientAssertionFromCert(tokenEndpoint, clientID, certPath, certPassword string) (string, error) {
	pfxData, err := os.ReadFile(certPath)
	if err != nil {
		return "", fmt.Errorf("failed to read cert file: %w", err)
	}

	privateKey, cert, err := parsePfxCertificate(pfxData, certPassword)
	if err != nil {
		return "", fmt.Errorf("failed to parse PFX certificate: %w", err)
	}

	now := time.Now()
	jti := fmt.Sprintf("%d", now.UnixNano())
	claims := jwt.MapClaims{
		"aud": tokenEndpoint,
		"iss": clientID,
		"sub": clientID,
		"jti": jti,
		"exp": now.Add(time.Minute * 5).Unix(),
		"nbf": now.Unix(),
		"iat": now.Unix(),
	}

	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	x5c := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: cert.Raw})
	token.Header["x5c"] = []string{string(x5c)}

	signed, err := token.SignedString(privateKey)
	if err != nil {
		return "", fmt.Errorf("failed to sign JWT: %w", err)
	}
	return signed, nil
}

// parsePfxCertificate parses a PFX/PKCS12 file and returns the RSA private key and x509 certificate.
func parsePfxCertificate(pfxData []byte, password string) (*rsa.PrivateKey, *x509.Certificate, error) {
	privateKey, cert, err := pkcs12.Decode(pfxData, password)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to decode pkcs12: %w", err)
	}

	rsaKey, ok := privateKey.(*rsa.PrivateKey)
	if !ok {
		return nil, nil, fmt.Errorf("private key is not RSA")
	}
	return rsaKey, cert, nil
}
