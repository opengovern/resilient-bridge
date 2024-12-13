// utils/azure_spn.go
package utils

import (
	"context"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"strings"
	"time"

	"golang.org/x/oauth2"
)

// AzureSPNConfig holds configuration required for authenticating via SPN.
type AzureSPNConfig struct {
	TenantID      string
	ClientID      string
	ClientSecret  string // If using secret-based authentication.
	Cert          *x509.Certificate
	PrivateKey    *rsa.PrivateKey
	AuthorityHost string // e.g. "https://login.microsoftonline.com"
	Resource      string // e.g. "https://management.azure.com/.default"
	UseCertAuth   bool   // If set to true, use client certificate flow instead of secret.
}

// azureTokenResponse represents the JSON structure returned by AAD token endpoint.
type azureTokenResponse struct {
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in"`
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token,omitempty"`
}

// AzureSPN provides methods to acquire and refresh tokens for Azure using SPN credentials.
type AzureSPN struct {
	config  *AzureSPNConfig
	client  *http.Client
	token   *oauth2.Token
	baseURL string
}

// NewAzureSPN creates a new AzureSPN instance.
// It sets up defaults such as AuthorityHost and Resource if not provided.
// The returned AzureSPN can then be used to AcquireToken() or AcquireTokenSilent() as needed.
func NewAzureSPN(cfg *AzureSPNConfig) (*AzureSPN, error) {
	if cfg.AuthorityHost == "" {
		cfg.AuthorityHost = "https://login.microsoftonline.com"
	}
	if cfg.Resource == "" {
		cfg.Resource = "https://management.azure.com/.default"
	}

	return &AzureSPN{
		config:  cfg,
		client:  &http.Client{Timeout: 30 * time.Second},
		baseURL: fmt.Sprintf("%s/%s/oauth2/v2.0/token", cfg.AuthorityHost, cfg.TenantID),
	}, nil
}

// AcquireToken obtains a new token from AAD. If UseCertAuth is true, it uses client assertion (JWT).
// Otherwise, it uses client_secret.
func (s *AzureSPN) AcquireToken(ctx context.Context) (*oauth2.Token, error) {
	form := url.Values{}
	form.Set("scope", s.config.Resource)
	form.Set("client_id", s.config.ClientID)

	if s.config.UseCertAuth {
		// Using client certificate authentication
		// Generate a JWT client assertion
		assertion, err := s.createClientAssertion()
		if err != nil {
			return nil, fmt.Errorf("failed to create client assertion: %w", err)
		}
		form.Set("grant_type", "client_credentials")
		form.Set("client_assertion_type", "urn:ietf:params:oauth:client-assertion-type:jwt-bearer")
		form.Set("client_assertion", assertion)
	} else {
		// Using client secret
		form.Set("grant_type", "client_credentials")
		form.Set("client_secret", s.config.ClientSecret)
	}

	tok, err := s.doTokenRequest(ctx, form)
	if err != nil {
		return nil, err
	}
	s.token = tok
	return s.token, nil
}

// AcquireTokenSilent returns a cached token if it's still valid, or tries to refresh it if supported.
// If token is expired and cannot be refreshed (no refresh_token), it calls AcquireToken again.
func (s *AzureSPN) AcquireTokenSilent(ctx context.Context) (*oauth2.Token, error) {
	if s.token == nil {
		return s.AcquireToken(ctx)
	}

	if s.token.Valid() {
		// Token is still valid
		return s.token, nil
	}

	// If we had a refresh token (not always available in client_credentials flow), attempt refresh.
	// Generally, client_credentials flow doesn't return a refresh_token. But if it does:
	if s.token.RefreshToken != "" {
		form := url.Values{}
		form.Set("grant_type", "refresh_token")
		form.Set("refresh_token", s.token.RefreshToken)
		form.Set("client_id", s.config.ClientID)
		if !s.config.UseCertAuth && s.config.ClientSecret != "" {
			form.Set("client_secret", s.config.ClientSecret)
		} else if s.config.UseCertAuth {
			// Attempt a refresh with certificate-based assertion
			assertion, err := s.createClientAssertion()
			if err != nil {
				return nil, fmt.Errorf("failed to create client assertion: %w", err)
			}
			form.Set("client_assertion_type", "urn:ietf:params:oauth:client-assertion-type:jwt-bearer")
			form.Set("client_assertion", assertion)
		}
		form.Set("scope", s.config.Resource)

		newTok, err := s.doTokenRequest(ctx, form)
		if err == nil {
			s.token = newTok
			return s.token, nil
		}
		// If refresh failed, fallback to full AcquireToken
	}

	return s.AcquireToken(ctx)
}

// createClientAssertion creates a JWT-based client assertion for certificate authentication.
// This involves signing a JWT with the SPN's private key.
func (s *AzureSPN) createClientAssertion() (string, error) {
	// Pseudocode for JWT creation (omitting actual JWT signing for brevity):
	//
	// header: {"alg":"RS256","typ":"JWT","x5t": base64url of certificate thumbprint}
	// payload: {
	//   "aud": s.baseURL,
	//   "iss": s.config.ClientID,
	//   "sub": s.config.ClientID,
	//   "jti": random UUID,
	//   "exp": now + 1min,
	//   "nbf": now,
	//   "iat": now
	// }
	// Sign using RSA SHA256 with s.config.PrivateKey
	// Return base64url(header) + "." + base64url(payload) + "." + base64url(signature)

	// NOTE: For brevity, this is not fully implemented. In a real scenario, you'd use a JWT library.
	return "", fmt.Errorf("createClientAssertion not implemented")
}

// doTokenRequest executes the token request with retries for transient errors.
func (s *AzureSPN) doTokenRequest(ctx context.Context, form url.Values) (*oauth2.Token, error) {
	var lastErr error
	for i := 0; i < 3; i++ {
		req, err := http.NewRequestWithContext(ctx, "POST", s.baseURL, strings.NewReader(form.Encode()))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

		resp, err := s.client.Do(req)
		if err != nil {
			lastErr = err
			time.Sleep(time.Duration(i+1) * time.Second)
			continue
		}
		defer resp.Body.Close()

		body, _ := ioutil.ReadAll(resp.Body)

		if resp.StatusCode >= 300 {
			lastErr = fmt.Errorf("token request failed: status %d, body: %s", resp.StatusCode, string(body))
			time.Sleep(time.Duration(i+1) * time.Second)
			continue
		}

		var tr azureTokenResponse
		if err := json.Unmarshal(body, &tr); err != nil {
			return nil, fmt.Errorf("failed to parse token response: %w", err)
		}

		expiry := time.Now().Add(time.Duration(tr.ExpiresIn) * time.Second)
		tok := &oauth2.Token{
			AccessToken:  tr.AccessToken,
			TokenType:    tr.TokenType,
			RefreshToken: tr.RefreshToken,
			Expiry:       expiry,
		}
		return tok, nil
	}
	return nil, lastErr
}

// Client returns an *http.Client that automatically injects the Bearer token.
// If the token is expired, it attempts to refresh before making the request.
// This can be used for long-running tasks that need seamless token refresh.
func (s *AzureSPN) Client(ctx context.Context) *http.Client {
	return &http.Client{
		Transport: &tokenTransport{
			base:   s.client.Transport,
			spn:    s,
			ctx:    ctx,
			baseTr: s.client.Transport,
		},
	}
}

type tokenTransport struct {
	base   http.RoundTripper
	spn    *AzureSPN
	ctx    context.Context
	baseTr http.RoundTripper
}

func (t *tokenTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	tok, err := t.spn.AcquireTokenSilent(t.ctx)
	if err != nil {
		return nil, err
	}
	req2 := cloneRequest(req)
	req2.Header.Set("Authorization", "Bearer "+tok.AccessToken)
	if t.base == nil {
		return http.DefaultTransport.RoundTrip(req2)
	}
	return t.base.RoundTrip(req2)
}

func cloneRequest(r *http.Request) *http.Request {
	r2 := r.Clone(r.Context())
	// shallow copy of r.Header
	r2.Header = make(http.Header, len(r.Header))
	for k, vv := range r.Header {
		vv2 := make([]string, len(vv))
		copy(vv2, vv)
		r2.Header[k] = vv2
	}
	return r2
}
