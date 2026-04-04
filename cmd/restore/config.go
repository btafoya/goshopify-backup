package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// GetConfig reads configuration from flags, environment, and saved credentials
func GetConfig() (*Config, error) {
	cfg := &Config{
		BackupDir:     getEnvDefault("BACKUP_DIR", "/backups/shopify"),
		Store:         os.Getenv("SHOPIFY_STORE"),
		AccessToken:   os.Getenv("SHOPIFY_ACCESS_TOKEN"),
		APIVersion:    getEnvDefault("SHOPIFY_API_VERSION", APIVersion),
		LogDir:        getEnvDefault("LOG_DIR", "/var/log/goshopify"),
		RollbackDir:   getEnvDefault("ROLLBACK_DIR", "/var/log/goshopify"),
		RestoreImages: ImageInteractive,
	}

	// Parse flags (simple parsing for now)
	for i, arg := range os.Args[1:] {
		switch arg {
		case "--dry-run", "-n":
			cfg.DryRun = true
		case "--force", "-f":
			cfg.Force = true
		case "--resume":
			cfg.Resume = true
		case "--verbose", "-v":
			cfg.Verbose = true
		case "--backup-dir":
			if i+1 < len(os.Args)-1 {
				cfg.BackupDir = os.Args[i+2]
			}
		case "--backup-date":
			if i+1 < len(os.Args)-1 {
				cfg.BackupDate = os.Args[i+2]
			}
		case "--store":
			if i+1 < len(os.Args)-1 {
				cfg.Store = os.Args[i+2]
			}
		case "--token":
			if i+1 < len(os.Args)-1 {
				cfg.AccessToken = os.Args[i+2]
			}
		case "--images-restore":
			cfg.RestoreImages = ImageRestore
		case "--images-skip":
			cfg.RestoreImages = ImageSkip
		}
	}

	// Expand ~ in paths
	cfg.BackupDir = expandPath(cfg.BackupDir)
	cfg.LogDir = expandPath(cfg.LogDir)
	cfg.RollbackDir = expandPath(cfg.RollbackDir)

	if err := ValidateConfig(cfg); err != nil {
		return nil, err
	}

	return cfg, nil
}

// ValidateConfig validates configuration
func ValidateConfig(cfg *Config) error {
	// Validate BACKUP_DIR
	if cfg.BackupDir == "" {
		return fmt.Errorf("BACKUP_DIR is required")
	}

	// Check if backup directory exists and is readable
	if _, err := os.Stat(cfg.BackupDir); os.IsNotExist(err) {
		return fmt.Errorf("backup directory does not exist: %s", cfg.BackupDir)
	}

	// Validate Store URL if provided
	if cfg.Store != "" {
		if !strings.HasPrefix(cfg.Store, "https://") {
			return fmt.Errorf("SHOPIFY_STORE must be HTTPS URL, got: %s", cfg.Store)
		}
		// Remove trailing slash
		cfg.Store = strings.TrimSuffix(cfg.Store, "/")

		// Validate store domain pattern
		storeRegex := regexp.MustCompile(StoreDomainPattern)
		if !storeRegex.MatchString(cfg.Store) {
			return fmt.Errorf("SHOPIFY_STORE must be a valid Shopify store URL (https://*.myshopify.com), got: %s", cfg.Store)
		}
	}

	// Validate SHOPIFY_ACCESS_TOKEN if store is provided
	if cfg.Store != "" && cfg.AccessToken == "" {
		return fmt.Errorf("SHOPIFY_ACCESS_TOKEN is required when store is specified")
	}

	// Validate SHOPIFY_API_VERSION
	apiVersionRegex := regexp.MustCompile(`^\d{4}-\d{2}$`)
	if !apiVersionRegex.MatchString(cfg.APIVersion) {
		return fmt.Errorf("SHOPIFY_API_VERSION must match YYYY-MM format, got: %s", cfg.APIVersion)
	}

	// Ensure log directory exists
	if err := os.MkdirAll(cfg.LogDir, 0755); err != nil {
		return fmt.Errorf("failed to create log directory %s: %w", cfg.LogDir, err)
	}

	// Ensure rollback directory exists
	if err := os.MkdirAll(cfg.RollbackDir, 0755); err != nil {
		return fmt.Errorf("failed to create rollback directory %s: %w", cfg.RollbackDir, err)
	}

	return nil
}

// getEnvDefault returns environment variable value or default
func getEnvDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// expandPath expands ~ to user's home directory
func expandPath(path string) string {
	if strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err == nil {
			return filepath.Join(home, path[2:])
		}
	}
	return path
}

// ParseInt parses an integer with a default value
func ParseInt(s string, defaultValue int) int {
	i, err := strconv.Atoi(s)
	if err != nil {
		return defaultValue
	}
	return i
}