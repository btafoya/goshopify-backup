package main

import (
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// cleanupOldBackups removes backups older than the retention period
func cleanupOldBackups(cfg *Config) error {
	if cfg.RetentionDays <= 0 {
		return nil // Retention disabled
	}

	// Get all subdirectories in backup dir
	entries, err := os.ReadDir(cfg.BackupDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // Backup directory doesn't exist, nothing to clean
		}
		return fmt.Errorf("failed to read backup directory: %w", err)
	}

	cutoffDate := time.Now().UTC().AddDate(0, 0, -cfg.RetentionDays)
	removed := 0

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		// Parse date from directory name (YYYY-MM-DD format)
		dirName := entry.Name()
		dirDate, err := time.Parse(DateFormat, dirName)
		if err != nil {
			// Not a date directory, skip
			continue
		}

		// Check if directory is older than retention period
		if dirDate.Before(cutoffDate) || dirDate.Equal(cutoffDate) {
			dirPath := filepath.Join(cfg.BackupDir, dirName)
			if err := os.RemoveAll(dirPath); err != nil {
				fmt.Fprintf(os.Stderr, "Warning: failed to remove old backup %s: %v\n", dirName, err)
			} else {
				removed++
				fmt.Printf("Removed old backup: %s\n", dirName)
			}
		}
	}

	if removed > 0 {
		fmt.Printf("Cleanup: removed %d old backup(s)\n", removed)
	}

	return nil
}

// getBackupDirForDate returns the backup directory for a specific date
func getBackupDirForDate(cfg *Config, date string) string {
	return filepath.Join(cfg.BackupDir, date)
}

// createBackupDir creates the backup directory for a specific date
func createBackupDir(cfg *Config, date string) error {
	dir := getBackupDirForDate(cfg, date)
	return os.MkdirAll(dir, 0755)
}
