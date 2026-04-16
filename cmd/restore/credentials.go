package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// CredentialManager manages saved store credentials (O4)
type CredentialManager struct {
	credentialsDir string
}

// NewCredentialManager creates a new credential manager
func NewCredentialManager() *CredentialManager {
	dir := expandPath(CredentialsDir)
	return &CredentialManager{credentialsDir: dir}
}

// Save saves a credential to disk
func (cm *CredentialManager) Save(cred Credential) error {
	if err := os.MkdirAll(cm.credentialsDir, 0700); err != nil {
		return fmt.Errorf("create credentials directory: %w", err)
	}

	cred.LastUsed = time.Now()

	// Load existing credentials
	creds, err := cm.loadAll()
	if err != nil {
		creds = make(map[string]Credential)
	}

	// Store by store URL
	key := cm.credKey(cred.Store)
	creds[key] = cred

	data, err := json.MarshalIndent(creds, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal credentials: %w", err)
	}

	filePath := filepath.Join(cm.credentialsDir, CredentialsFile)
	return os.WriteFile(filePath, data, 0600)
}

// Load loads a credential for a store
func (cm *CredentialManager) Load(storeURL string) (*Credential, error) {
	creds, err := cm.loadAll()
	if err != nil {
		return nil, err
	}

	key := cm.credKey(storeURL)
	if cred, ok := creds[key]; ok {
		return &cred, nil
	}

	return nil, fmt.Errorf("no saved credentials for %s", storeURL)
}

// List returns all saved credentials
func (cm *CredentialManager) List() ([]Credential, error) {
	creds, err := cm.loadAll()
	if err != nil {
		return nil, err
	}

	result := make([]Credential, 0, len(creds))
	for _, cred := range creds {
		result = append(result, cred)
	}
	return result, nil
}

// Delete removes a saved credential
func (cm *CredentialManager) Delete(storeURL string) error {
	creds, err := cm.loadAll()
	if err != nil {
		return err
	}

	key := cm.credKey(storeURL)
	delete(creds, key)

	data, err := json.MarshalIndent(creds, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal credentials: %w", err)
	}

	filePath := filepath.Join(cm.credentialsDir, CredentialsFile)
	return os.WriteFile(filePath, data, 0600)
}

// loadAll reads all saved credentials
func (cm *CredentialManager) loadAll() (map[string]Credential, error) {
	filePath := filepath.Join(cm.credentialsDir, CredentialsFile)

	data, err := os.ReadFile(filePath)
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

// credKey generates a storage key from a store URL
func (cm *CredentialManager) credKey(storeURL string) string {
	// Normalize: remove https:// and trailing slash
	key := strings.TrimPrefix(storeURL, "https://")
	key = strings.TrimPrefix(key, "http://")
	key = strings.TrimSuffix(key, "/")
	return key
}

// GetOrPromptConfig checks for credentials and returns config values,
// falling back to env vars and prompting if needed
func GetOrPromptConfig(cfg *Config) error {
	cm := NewCredentialManager()

	// If store is set but auth is missing, try saved credentials
	if cfg.Store != "" && cfg.AccessToken == "" && (cfg.ClientID == "" || cfg.ClientSecret == "") {
		if cred, err := cm.Load(cfg.Store); err == nil {
			// Prefer saved client credentials over access token (they auto-refresh)
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
