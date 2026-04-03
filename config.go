package main

import (
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
)

// GetConfig reads and validates environment variables
func GetConfig() (*Config, error) {
	cfg := &Config{
		Store:         os.Getenv("SHOPIFY_STORE"),
		AccessToken:   os.Getenv("SHOPIFY_ACCESS_TOKEN"),
		APIVersion:    getEnvDefault("SHOPIFY_API_VERSION", APIVersion),
		BackupDir:     getEnvDefault("BACKUP_DIR", "/backups/shopify"),
		Force:         os.Getenv("FORCE") == "true",
		RetentionDays: 30,
		PollTimeout:   PollTimeout,
	}

	// Parse retention days
	if rd := os.Getenv("RETENTION_DAYS"); rd != "" {
		days, err := strconv.Atoi(rd)
		if err != nil {
			return nil, fmt.Errorf("RETENTION_DAYS must be an integer, got: %s", rd)
		}
		cfg.RetentionDays = days
	}

	if err := ValidateConfig(cfg); err != nil {
		return nil, err
	}

	return cfg, nil
}

// ValidateConfig performs startup validation of all config values
func ValidateConfig(cfg *Config) error {
	// Validate SHOPIFY_STORE
	if cfg.Store == "" {
		return fmt.Errorf("SHOPIFY_STORE is required")
	}
	if !strings.HasPrefix(cfg.Store, "https://") {
		return fmt.Errorf("SHOPIFY_STORE must be HTTPS URL, got: %s", cfg.Store)
	}
	// Remove trailing slash
	cfg.Store = strings.TrimSuffix(cfg.Store, "/")

	// Validate SHOPIFY_ACCESS_TOKEN
	if cfg.AccessToken == "" {
		return fmt.Errorf("SHOPIFY_ACCESS_TOKEN is required")
	}

	// Validate SHOPIFY_API_VERSION
	apiVersionRegex := regexp.MustCompile(`^\d{4}-\d{2}$`)
	if !apiVersionRegex.MatchString(cfg.APIVersion) {
		return fmt.Errorf("SHOPIFY_API_VERSION must match YYYY-MM format, got: %s", cfg.APIVersion)
	}

	// Validate RETENTION_DAYS
	if cfg.RetentionDays < 1 {
		cfg.RetentionDays = 1
		fmt.Fprintf(os.Stderr, "Warning: RETENTION_DAYS clamped to minimum 1\n")
	}
	if cfg.RetentionDays > MaxRetentionDays {
		cfg.RetentionDays = MaxRetentionDays
		fmt.Fprintf(os.Stderr, "Warning: RETENTION_DAYS clamped to maximum %d\n", MaxRetentionDays)
	}

	// Validate BACKUP_DIR
	if cfg.BackupDir == "" {
		return fmt.Errorf("BACKUP_DIR is required")
	}

	// Ensure backup directory exists
	if err := os.MkdirAll(cfg.BackupDir, 0755); err != nil {
		return fmt.Errorf("failed to create backup directory %s: %w", cfg.BackupDir, err)
	}

	// Test write permission
	testFile := cfg.BackupDir + "/.write_test"
	if err := os.WriteFile(testFile, []byte("test"), 0644); err != nil {
		return fmt.Errorf("BACKUP_DIR is not writable: %w", err)
	}
	os.Remove(testFile)

	return nil
}

func getEnvDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
