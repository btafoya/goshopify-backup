package main

import (
	"os"
	"testing"
)

func TestValidateConfig_StoreRequired(t *testing.T) {
	tests := []struct {
		name    string
		store   string
		wantErr bool
	}{
		{"empty store", "", true},
		{"not https", "http://test.myshopify.com", true},
		{"no protocol", "test.myshopify.com", true},
		{"valid https", "https://test.myshopify.com", false},
		{"https with trailing slash", "https://test.myshopify.com/", false},
	}

	tmpDir := "/tmp/shopify-test-store"
	os.RemoveAll(tmpDir)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{
				Store:       tt.store,
				AccessToken: "test_token",
				APIVersion:  "2025-01",
				BackupDir:   tmpDir,
			}
			err := ValidateConfig(cfg)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateConfig() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}

	os.RemoveAll(tmpDir)
}

func TestValidateConfig_AccessTokenRequired(t *testing.T) {
	cfg := &Config{
		Store:       "https://test.myshopify.com",
		AccessToken: "",
		BackupDir:   "/tmp/shopify-test",
	}

	err := ValidateConfig(cfg)
	if err == nil {
		t.Error("ValidateConfig() expected error for empty access token")
	}
}

func TestValidateConfig_APIVersionFormat(t *testing.T) {
	tests := []struct {
		name    string
		version string
		wantErr bool
	}{
		{"valid", "2025-01", false},
		{"invalid format", "2025/01", true},
		{"missing month", "2025", true},
		{"string", "latest", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{
				Store:       "https://test.myshopify.com",
				AccessToken: "test_token",
				APIVersion:  tt.version,
				BackupDir:   "/tmp/shopify-test",
			}
			err := ValidateConfig(cfg)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateConfig() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidateConfig_RetentionDaysClamped(t *testing.T) {
	tmpDir := "/tmp/shopify-retention-test"
	os.RemoveAll(tmpDir)

	tests := []struct {
		name  string
		input int
		want  int
	}{
		{"below minimum", 0, 1},
		{"negative", -5, 1},
		{"valid", 30, 30},
		{"above maximum", 4000, 3650},
		{"at maximum", 3650, 3650},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{
				Store:         "https://test.myshopify.com",
				AccessToken:   "test_token",
				APIVersion:    "2025-01",
				BackupDir:     tmpDir,
				RetentionDays: tt.input,
			}
			_ = ValidateConfig(cfg)
			if cfg.RetentionDays != tt.want {
				t.Errorf("RetentionDays = %v, want %v", cfg.RetentionDays, tt.want)
			}
		})
	}

	os.RemoveAll(tmpDir)
}

func TestGetConfig(t *testing.T) {
	// Save original env vars
	origStore := os.Getenv("SHOPIFY_STORE")
	origToken := os.Getenv("SHOPIFY_ACCESS_TOKEN")
	origDir := os.Getenv("BACKUP_DIR")

	defer func() {
		os.Setenv("SHOPIFY_STORE", origStore)
		os.Setenv("SHOPIFY_ACCESS_TOKEN", origToken)
		os.Setenv("BACKUP_DIR", origDir)
	}()

	// Test with valid env vars
	os.Setenv("SHOPIFY_STORE", "https://test.myshopify.com")
	os.Setenv("SHOPIFY_ACCESS_TOKEN", "test_token")
	os.Setenv("BACKUP_DIR", "/tmp/shopify-test")

	cfg, err := GetConfig()
	if err != nil {
		t.Fatalf("GetConfig() error = %v", err)
	}

	if cfg.Store != "https://test.myshopify.com" {
		t.Errorf("Store = %v, want https://test.myshopify.com", cfg.Store)
	}
	if cfg.AccessToken != "test_token" {
		t.Errorf("AccessToken = %v, want test_token", cfg.AccessToken)
	}
	if cfg.BackupDir != "/tmp/shopify-test" {
		t.Errorf("BackupDir = %v, want /tmp/shopify-test", cfg.BackupDir)
	}
	if cfg.APIVersion != "2025-01" {
		t.Errorf("APIVersion = %v, want 2025-01", cfg.APIVersion)
	}
}

func TestAccessDeniedError(t *testing.T) {
	err := &AccessDeniedError{Message: "bulk operation not allowed"}
	got := err.Error()
	want := "ACCESS_DENIED: bulk operation not allowed"
	if got != want {
		t.Errorf("Error() = %v, want %v", got, want)
	}
}
