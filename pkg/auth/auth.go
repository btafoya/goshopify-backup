// Package auth provides Shopify authentication for both backup and restore tools.
//
// It supports two auth modes:
//   - Direct access token (legacy): SHOPIFY_ACCESS_TOKEN
//   - Client credentials OAuth (recommended): SHOPIFY_CLIENT_ID + SHOPIFY_SECRET
//
// The Authenticator handles token exchange, caching, scope validation,
// concurrent-safe refresh, and optional disk persistence.
package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

const (
	// TokenRefreshBuffer is how far before expiry to refresh a cached token.
	TokenRefreshBuffer = 60 * time.Second

	// RefreshMaxRetries bounds OAuth retry attempts on transient failures.
	RefreshMaxRetries = 3

	// RefreshBaseDelay is the initial backoff between OAuth retries.
	RefreshBaseDelay = 2 * time.Second

	defaultOAuthTimeout = 30 * time.Second
)

// Config configures a new Authenticator.
type Config struct {
	// StoreURL is the full Shopify store URL (https://example.myshopify.com).
	StoreURL string

	// AccessToken, if set, bypasses OAuth and is used directly.
	AccessToken string

	// ClientID + ClientSecret enable OAuth client_credentials grant.
	ClientID     string
	ClientSecret string

	// APIVersion (e.g. "2025-01"). Persisted with credentials.
	APIVersion string

	// RequiredScopes lists scopes the token must include. Validated after fetch.
	// Empty = no scope check.
	RequiredScopes []string

	// HTTPClient is used for OAuth requests. If nil a default is created.
	HTTPClient *http.Client

	// CredentialsStore, if non-nil, persists credentials + tokens to disk.
	CredentialsStore *CredentialStore
}

// Authenticator manages a Shopify access token, refreshing via OAuth as needed.
type Authenticator struct {
	storeURL       string
	accessToken    string
	clientID       string
	clientSecret   string
	apiVersion     string
	requiredScopes []string
	httpClient     *http.Client
	store          *CredentialStore

	mu          sync.Mutex
	cached      string
	cachedScope string
	expiry      time.Time
}

// New creates an Authenticator. Use Authenticate to fetch and validate
// the initial token (proactive mode), or call EnsureToken on first use (lazy).
func New(cfg Config) *Authenticator {
	httpClient := cfg.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: defaultOAuthTimeout}
	}
	return &Authenticator{
		storeURL:       strings.TrimSuffix(cfg.StoreURL, "/"),
		accessToken:    cfg.AccessToken,
		clientID:       cfg.ClientID,
		clientSecret:   cfg.ClientSecret,
		apiVersion:     cfg.APIVersion,
		requiredScopes: cfg.RequiredScopes,
		httpClient:     httpClient,
		store:          cfg.CredentialsStore,
	}
}

// Authenticate performs proactive token acquisition and scope validation.
// Call once at startup before any API requests. Returns ErrMissingCredentials
// if neither auth mode is configured, or ErrMissingScopes if the token lacks
// required scopes.
func (a *Authenticator) Authenticate(ctx context.Context) error {
	if a.accessToken != "" {
		// Direct token: no scope check possible without an extra API call.
		// Caller can issue a probe query if they need scope verification.
		return nil
	}
	if a.clientID == "" || a.clientSecret == "" {
		return ErrMissingCredentials
	}

	token, expiresIn, scope, err := a.fetchWithRetry(ctx)
	if err != nil {
		return fmt.Errorf("oauth: %w", err)
	}

	if err := a.validateScopes(scope); err != nil {
		return err
	}

	a.mu.Lock()
	a.cached = token
	a.cachedScope = scope
	a.expiry = time.Now().Add(time.Duration(expiresIn) * time.Second)
	a.mu.Unlock()

	if a.store != nil {
		_ = a.store.Save(Credential{
			Store:        a.storeURL,
			ClientID:     a.clientID,
			ClientSecret: a.clientSecret,
			APIVersion:   a.apiVersion,
			AccessToken:  token,
			Scope:        scope,
			TokenExpiry:  a.expiry,
			LastUsed:     time.Now(),
		})
	}

	return nil
}

// EnsureToken returns a valid access token, refreshing via OAuth if cached
// token is expired or near-expiry. Safe for concurrent use.
func (a *Authenticator) EnsureToken(ctx context.Context) (string, error) {
	if a.accessToken != "" {
		return a.accessToken, nil
	}

	a.mu.Lock()
	if a.cached != "" && time.Now().Before(a.expiry.Add(-TokenRefreshBuffer)) {
		token := a.cached
		a.mu.Unlock()
		return token, nil
	}
	a.mu.Unlock()

	if a.clientID == "" || a.clientSecret == "" {
		return "", ErrMissingCredentials
	}

	token, expiresIn, scope, err := a.fetchWithRetry(ctx)
	if err != nil {
		return "", fmt.Errorf("oauth refresh: %w", err)
	}

	if err := a.validateScopes(scope); err != nil {
		return "", err
	}

	a.mu.Lock()
	a.cached = token
	a.cachedScope = scope
	a.expiry = time.Now().Add(time.Duration(expiresIn) * time.Second)
	a.mu.Unlock()

	if a.store != nil {
		_ = a.store.Save(Credential{
			Store:        a.storeURL,
			ClientID:     a.clientID,
			ClientSecret: a.clientSecret,
			APIVersion:   a.apiVersion,
			AccessToken:  token,
			Scope:        scope,
			TokenExpiry:  a.expiry,
			LastUsed:     time.Now(),
		})
	}

	return token, nil
}

// Scopes returns scopes from the most recent token fetch (empty for direct token).
func (a *Authenticator) Scopes() []string {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.cachedScope == "" {
		return nil
	}
	return splitScopes(a.cachedScope)
}

// Expiry returns the cached token expiry. Zero for direct tokens.
func (a *Authenticator) Expiry() time.Time {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.expiry
}

// oauthTokenResponse mirrors Shopify's OAuth token endpoint payload.
type oauthTokenResponse struct {
	AccessToken string `json:"access_token"`
	Scope       string `json:"scope"`
	ExpiresIn   int    `json:"expires_in"`
}

// fetchWithRetry attempts up to RefreshMaxRetries OAuth token exchanges
// with exponential backoff. Returns the last error if all attempts fail.
func (a *Authenticator) fetchWithRetry(ctx context.Context) (token string, expiresIn int, scope string, err error) {
	delay := RefreshBaseDelay
	for attempt := 1; attempt <= RefreshMaxRetries; attempt++ {
		token, expiresIn, scope, err = a.fetchToken(ctx)
		if err == nil {
			return token, expiresIn, scope, nil
		}
		if attempt == RefreshMaxRetries {
			break
		}
		select {
		case <-ctx.Done():
			return "", 0, "", ctx.Err()
		case <-time.After(delay):
		}
		delay *= 2
	}
	return "", 0, "", err
}

// fetchToken performs a single OAuth client_credentials exchange.
func (a *Authenticator) fetchToken(ctx context.Context) (string, int, string, error) {
	tokenURL := fmt.Sprintf("%s/admin/oauth/access_token", a.storeURL)

	form := url.Values{}
	form.Set("grant_type", "client_credentials")
	form.Set("client_id", a.clientID)
	form.Set("client_secret", a.clientSecret)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return "", 0, "", fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := a.httpClient.Do(req)
	if err != nil {
		return "", 0, "", fmt.Errorf("post: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", 0, "", fmt.Errorf("read body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", 0, "", fmt.Errorf("status %d: %s", resp.StatusCode, string(body))
	}

	var tr oauthTokenResponse
	if err := json.Unmarshal(body, &tr); err != nil {
		return "", 0, "", fmt.Errorf("parse response: %w", err)
	}
	if tr.AccessToken == "" {
		return "", 0, "", fmt.Errorf("empty access_token in response")
	}
	if tr.ExpiresIn <= 0 {
		return "", 0, "", fmt.Errorf("invalid expires_in: %d", tr.ExpiresIn)
	}
	return tr.AccessToken, tr.ExpiresIn, tr.Scope, nil
}

// validateScopes returns ErrMissingScopes if any required scope is absent
// from the granted scope string.
func (a *Authenticator) validateScopes(granted string) error {
	if len(a.requiredScopes) == 0 {
		return nil
	}
	have := make(map[string]struct{}, 16)
	for _, s := range splitScopes(granted) {
		have[s] = struct{}{}
	}
	var missing []string
	for _, want := range a.requiredScopes {
		if _, ok := have[want]; !ok {
			missing = append(missing, want)
		}
	}
	if len(missing) > 0 {
		return &MissingScopesError{Required: a.requiredScopes, Missing: missing, Granted: splitScopes(granted)}
	}
	return nil
}

func splitScopes(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}
