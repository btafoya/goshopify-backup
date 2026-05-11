package auth

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// newOAuthServer returns an httptest server that responds to
// /admin/oauth/access_token with the given token + scope + expiry.
func newOAuthServer(t *testing.T, token, scope string, expiresIn int) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/admin/oauth/access_token", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if err := r.ParseForm(); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if r.FormValue("grant_type") != "client_credentials" {
			http.Error(w, "bad grant_type", http.StatusBadRequest)
			return
		}
		if r.FormValue("client_id") == "" || r.FormValue("client_secret") == "" {
			http.Error(w, "missing creds", http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"access_token":%q,"scope":%q,"expires_in":%d}`, token, scope, expiresIn)
	})
	return httptest.NewServer(mux)
}

func TestAuthenticate_DirectTokenNoop(t *testing.T) {
	a := New(Config{AccessToken: "static-token"})
	if err := a.Authenticate(context.Background()); err != nil {
		t.Fatalf("direct token authenticate should be no-op, got: %v", err)
	}
	tok, err := a.EnsureToken(context.Background())
	if err != nil {
		t.Fatalf("EnsureToken: %v", err)
	}
	if tok != "static-token" {
		t.Errorf("want static-token, got %q", tok)
	}
}

func TestAuthenticate_MissingCredentials(t *testing.T) {
	a := New(Config{StoreURL: "https://example.myshopify.com"})
	err := a.Authenticate(context.Background())
	if !errors.Is(err, ErrMissingCredentials) {
		t.Fatalf("want ErrMissingCredentials, got: %v", err)
	}
}

func TestAuthenticate_ClientCredentialsSuccess(t *testing.T) {
	srv := newOAuthServer(t, "tok-abc", "read_products,read_orders", 3600)
	defer srv.Close()

	a := New(Config{
		StoreURL:       srv.URL,
		ClientID:       "id",
		ClientSecret:   "secret",
		RequiredScopes: []string{"read_products"},
	})
	if err := a.Authenticate(context.Background()); err != nil {
		t.Fatalf("Authenticate: %v", err)
	}
	tok, _ := a.EnsureToken(context.Background())
	if tok != "tok-abc" {
		t.Errorf("want tok-abc, got %q", tok)
	}
}

func TestAuthenticate_MissingScopes(t *testing.T) {
	srv := newOAuthServer(t, "tok-abc", "read_products", 3600)
	defer srv.Close()

	a := New(Config{
		StoreURL:       srv.URL,
		ClientID:       "id",
		ClientSecret:   "secret",
		RequiredScopes: []string{"read_products", "read_customers"},
	})
	err := a.Authenticate(context.Background())
	var scopeErr *MissingScopesError
	if !errors.As(err, &scopeErr) {
		t.Fatalf("want MissingScopesError, got %v", err)
	}
	if len(scopeErr.Missing) != 1 || scopeErr.Missing[0] != "read_customers" {
		t.Errorf("missing scopes mismatch: %v", scopeErr.Missing)
	}
}

func TestAuthenticate_WriteScopeSatisfiesReadRequirement(t *testing.T) {
	// Shopify returns only write_X in scope when both read_X + write_X are granted.
	srv := newOAuthServer(t, "tok-x", "write_products,write_themes,read_orders", 3600)
	defer srv.Close()

	a := New(Config{
		StoreURL:       srv.URL,
		ClientID:       "id",
		ClientSecret:   "secret",
		RequiredScopes: []string{"read_products", "read_themes", "read_orders"},
	})
	if err := a.Authenticate(context.Background()); err != nil {
		t.Fatalf("write_X should satisfy read_X requirement, got: %v", err)
	}
}

func TestAuthenticate_WriteScopeDoesNotSatisfyDifferentResource(t *testing.T) {
	// write_themes should NOT satisfy read_products.
	srv := newOAuthServer(t, "tok-x", "write_themes", 3600)
	defer srv.Close()

	a := New(Config{
		StoreURL:       srv.URL,
		ClientID:       "id",
		ClientSecret:   "secret",
		RequiredScopes: []string{"read_products"},
	})
	err := a.Authenticate(context.Background())
	var scopeErr *MissingScopesError
	if !errors.As(err, &scopeErr) {
		t.Fatalf("want MissingScopesError, got: %v", err)
	}
	if len(scopeErr.Missing) != 1 || scopeErr.Missing[0] != "read_products" {
		t.Errorf("missing mismatch: %v", scopeErr.Missing)
	}
}

func TestEnsureToken_RefreshesOnExpiry(t *testing.T) {
	var calls atomic.Int32
	mux := http.NewServeMux()
	mux.HandleFunc("/admin/oauth/access_token", func(w http.ResponseWriter, r *http.Request) {
		n := calls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"access_token":"tok-%d","scope":"read_products","expires_in":61}`, n)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	a := New(Config{
		StoreURL:     srv.URL,
		ClientID:     "id",
		ClientSecret: "secret",
	})
	if err := a.Authenticate(context.Background()); err != nil {
		t.Fatal(err)
	}

	// Force expiry near refresh buffer (61s lifetime, 60s buffer = next call refreshes).
	a.mu.Lock()
	a.expiry = time.Now().Add(30 * time.Second)
	a.mu.Unlock()

	tok, err := a.EnsureToken(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if tok != "tok-2" {
		t.Errorf("want refreshed tok-2, got %q (calls=%d)", tok, calls.Load())
	}
}

func TestEnsureToken_ReusesCachedToken(t *testing.T) {
	var calls atomic.Int32
	mux := http.NewServeMux()
	mux.HandleFunc("/admin/oauth/access_token", func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"access_token":"tok-only","scope":"read_products","expires_in":3600}`)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	a := New(Config{StoreURL: srv.URL, ClientID: "id", ClientSecret: "secret"})
	if err := a.Authenticate(context.Background()); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 5; i++ {
		if _, err := a.EnsureToken(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	if got := calls.Load(); got != 1 {
		t.Errorf("expected 1 OAuth call, got %d", got)
	}
}

func TestEnsureToken_ConcurrentRefresh(t *testing.T) {
	srv := newOAuthServer(t, "tok-concurrent", "read_products", 3600)
	defer srv.Close()

	a := New(Config{StoreURL: srv.URL, ClientID: "id", ClientSecret: "secret"})
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := a.EnsureToken(context.Background()); err != nil {
				t.Errorf("EnsureToken: %v", err)
			}
		}()
	}
	wg.Wait()
}

func TestFetchToken_RetryOnTransientFailure(t *testing.T) {
	var calls atomic.Int32
	mux := http.NewServeMux()
	mux.HandleFunc("/admin/oauth/access_token", func(w http.ResponseWriter, r *http.Request) {
		n := calls.Add(1)
		if n < 2 {
			http.Error(w, "boom", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"access_token":"tok-r","scope":"read_products","expires_in":3600}`)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	a := &Authenticator{
		storeURL:     srv.URL,
		clientID:     "id",
		clientSecret: "secret",
		httpClient:   &http.Client{Timeout: 5 * time.Second},
	}
	// Bypass long backoffs by zeroing delay path via direct fetch loop:
	tok, _, _, err := a.fetchWithRetry(context.Background())
	if err != nil {
		t.Fatalf("fetchWithRetry: %v", err)
	}
	if tok != "tok-r" {
		t.Errorf("want tok-r, got %q", tok)
	}
	if calls.Load() != 2 {
		t.Errorf("want 2 calls, got %d", calls.Load())
	}
}

func TestCredentialStore_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	store := NewCredentialStoreAt(filepath.Join(dir, "credentials.json"))

	in := Credential{
		Store:        "https://example.myshopify.com",
		ClientID:     "id",
		ClientSecret: "secret",
		APIVersion:   "2025-01",
		AccessToken:  "tok-saved",
		Scope:        "read_products",
		TokenExpiry:  time.Now().Add(time.Hour).Truncate(time.Second),
	}
	if err := store.Save(in); err != nil {
		t.Fatalf("Save: %v", err)
	}

	out, err := store.Load(in.Store)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if out.AccessToken != in.AccessToken || out.ClientID != in.ClientID {
		t.Errorf("roundtrip mismatch: %+v vs %+v", in, out)
	}
}

func TestCredentialStore_DeleteAndList(t *testing.T) {
	dir := t.TempDir()
	store := NewCredentialStoreAt(filepath.Join(dir, "credentials.json"))

	c1 := Credential{Store: "https://a.myshopify.com", AccessToken: "a"}
	c2 := Credential{Store: "https://b.myshopify.com", AccessToken: "b"}
	if err := store.Save(c1); err != nil {
		t.Fatal(err)
	}
	if err := store.Save(c2); err != nil {
		t.Fatal(err)
	}
	list, err := store.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 {
		t.Errorf("want 2 creds, got %d", len(list))
	}
	if err := store.Delete(c1.Store); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Load(c1.Store); err == nil {
		t.Error("expected error loading deleted credential")
	}
}

func TestAuthenticate_PersistsToStore(t *testing.T) {
	srv := newOAuthServer(t, "tok-persist", "read_products", 3600)
	defer srv.Close()

	dir := t.TempDir()
	store := NewCredentialStoreAt(filepath.Join(dir, "credentials.json"))

	a := New(Config{
		StoreURL:         srv.URL,
		ClientID:         "id",
		ClientSecret:     "secret",
		APIVersion:       "2025-01",
		CredentialsStore: store,
	})
	if err := a.Authenticate(context.Background()); err != nil {
		t.Fatal(err)
	}
	saved, err := store.Load(srv.URL)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if saved.AccessToken != "tok-persist" {
		t.Errorf("persisted token mismatch: %q", saved.AccessToken)
	}
}
