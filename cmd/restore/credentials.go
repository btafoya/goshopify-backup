package main

import (
	"path/filepath"

	"github.com/btafoya/goshopify-backup/pkg/auth"
)

// CredentialManager manages saved store credentials.
// Backed by the shared pkg/auth credential store.
type CredentialManager struct {
	credentialsDir string
}

// NewCredentialManager creates a new credential manager using the default
// credentials directory (~/.config/goshopify).
func NewCredentialManager() *CredentialManager {
	dir := expandPath(CredentialsDir)
	return &CredentialManager{credentialsDir: dir}
}

func (cm *CredentialManager) store() *auth.CredentialStore {
	return auth.NewCredentialStoreAt(filepath.Join(cm.credentialsDir, CredentialsFile))
}

// Save persists a credential.
func (cm *CredentialManager) Save(cred Credential) error {
	return cm.store().Save(toAuthCredential(cred))
}

// Load returns the credential for the given store URL.
func (cm *CredentialManager) Load(storeURL string) (*Credential, error) {
	c, err := cm.store().Load(storeURL)
	if err != nil {
		return nil, err
	}
	out := fromAuthCredential(*c)
	return &out, nil
}

// List returns all stored credentials.
func (cm *CredentialManager) List() ([]Credential, error) {
	src, err := cm.store().List()
	if err != nil {
		return nil, err
	}
	out := make([]Credential, len(src))
	for i, c := range src {
		out[i] = fromAuthCredential(c)
	}
	return out, nil
}

func toAuthCredential(c Credential) auth.Credential {
	return auth.Credential{
		Store:        c.Store,
		AccessToken:  c.AccessToken,
		ClientID:     c.ClientID,
		ClientSecret: c.ClientSecret,
		APIVersion:   c.APIVersion,
		LastUsed:     c.LastUsed,
		Nickname:     c.Nickname,
	}
}

func fromAuthCredential(c auth.Credential) Credential {
	return Credential{
		Store:        c.Store,
		AccessToken:  c.AccessToken,
		ClientID:     c.ClientID,
		ClientSecret: c.ClientSecret,
		APIVersion:   c.APIVersion,
		LastUsed:     c.LastUsed,
		Nickname:     c.Nickname,
	}
}

// Delete removes the credential for the given store URL.
func (cm *CredentialManager) Delete(storeURL string) error {
	return cm.store().Delete(storeURL)
}

// GetOrPromptConfig checks for credentials and returns config values,
// falling back to env vars and prompting if needed.
func GetOrPromptConfig(cfg *Config) error {
	cm := NewCredentialManager()
	if cfg.Store != "" && cfg.AccessToken == "" && (cfg.ClientID == "" || cfg.ClientSecret == "") {
		if cred, err := cm.Load(cfg.Store); err == nil {
			if cred.ClientID != "" && cred.ClientSecret != "" {
				cfg.ClientID = cred.ClientID
				cfg.ClientSecret = cred.ClientSecret
			} else if cred.AccessToken != "" {
				cfg.AccessToken = cred.AccessToken
			}
			if cfg.APIVersion == "" || cfg.APIVersion == APIVersion {
				cfg.APIVersion = cred.APIVersion
			}
		}
	}
	return nil
}
