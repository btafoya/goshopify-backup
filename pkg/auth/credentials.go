package auth

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// DefaultCredentialsDir is the default directory for persisted credentials.
// Override with GOSHOPIFY_CREDS_FILE env var or by constructing CredentialStore
// with a custom path.
const (
	DefaultCredentialsDir  = "~/.config/goshopify"
	DefaultCredentialsFile = "credentials.json"
	credentialsDirPerm     = 0o700
	credentialsFilePerm    = 0o600
)

// Credential represents persisted credentials and the most recent cached token.
type Credential struct {
	Store        string    `json:"store"`
	AccessToken  string    `json:"access_token,omitempty"`
	ClientID     string    `json:"client_id,omitempty"`
	ClientSecret string    `json:"client_secret,omitempty"`
	APIVersion   string    `json:"api_version,omitempty"`
	Scope        string    `json:"scope,omitempty"`
	TokenExpiry  time.Time `json:"token_expiry,omitzero"`
	LastUsed     time.Time `json:"last_used"`
	Nickname     string    `json:"nickname,omitempty"`
}

// CredentialStore persists credentials to disk in a single JSON file keyed
// by normalized store URL.
type CredentialStore struct {
	path string
	mu   sync.Mutex
}

// NewCredentialStore returns a store at the default path
// ($GOSHOPIFY_CREDS_FILE or ~/.config/goshopify/credentials.json).
func NewCredentialStore() *CredentialStore {
	if p := os.Getenv("GOSHOPIFY_CREDS_FILE"); p != "" {
		return &CredentialStore{path: p}
	}
	return &CredentialStore{
		path: filepath.Join(expandHome(DefaultCredentialsDir), DefaultCredentialsFile),
	}
}

// NewCredentialStoreAt returns a store at the given path.
func NewCredentialStoreAt(path string) *CredentialStore {
	return &CredentialStore{path: path}
}

// Path returns the on-disk credentials file path.
func (s *CredentialStore) Path() string {
	return s.path
}

// Save writes the credential to disk, merging with any existing entries.
func (s *CredentialStore) Save(c Credential) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := os.MkdirAll(filepath.Dir(s.path), credentialsDirPerm); err != nil {
		return fmt.Errorf("create credentials dir: %w", err)
	}

	creds, _ := s.loadAllLocked()
	if creds == nil {
		creds = make(map[string]Credential)
	}
	c.LastUsed = time.Now()
	creds[credKey(c.Store)] = c

	data, err := json.MarshalIndent(creds, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal credentials: %w", err)
	}
	if err := os.WriteFile(s.path, data, credentialsFilePerm); err != nil {
		return fmt.Errorf("write credentials: %w", err)
	}
	return nil
}

// Load returns the credential for the given store URL.
func (s *CredentialStore) Load(storeURL string) (*Credential, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	creds, err := s.loadAllLocked()
	if err != nil {
		return nil, err
	}
	c, ok := creds[credKey(storeURL)]
	if !ok {
		return nil, fmt.Errorf("no saved credentials for %s", storeURL)
	}
	return &c, nil
}

// List returns all stored credentials.
func (s *CredentialStore) List() ([]Credential, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	creds, err := s.loadAllLocked()
	if err != nil {
		return nil, err
	}
	out := make([]Credential, 0, len(creds))
	for _, c := range creds {
		out = append(out, c)
	}
	return out, nil
}

// Delete removes the credential for the given store URL.
func (s *CredentialStore) Delete(storeURL string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	creds, err := s.loadAllLocked()
	if err != nil {
		return err
	}
	delete(creds, credKey(storeURL))

	data, err := json.MarshalIndent(creds, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal credentials: %w", err)
	}
	return os.WriteFile(s.path, data, credentialsFilePerm)
}

func (s *CredentialStore) loadAllLocked() (map[string]Credential, error) {
	data, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return make(map[string]Credential), nil
		}
		return nil, fmt.Errorf("read credentials: %w", err)
	}
	var creds map[string]Credential
	if err := json.Unmarshal(data, &creds); err != nil {
		return nil, fmt.Errorf("parse credentials: %w", err)
	}
	return creds, nil
}

func credKey(storeURL string) string {
	k := strings.TrimPrefix(storeURL, "https://")
	k = strings.TrimPrefix(k, "http://")
	k = strings.TrimSuffix(k, "/")
	return k
}

func expandHome(path string) string {
	if strings.HasPrefix(path, "~/") || path == "~" {
		home, err := os.UserHomeDir()
		if err == nil {
			return filepath.Join(home, strings.TrimPrefix(path, "~"))
		}
	}
	return path
}
