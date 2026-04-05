package backup

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// Validator validates backup structure
type Validator struct{}

// NewValidator creates a new backup validator
func NewValidator() *Validator {
	return &Validator{}
}

// Validate validates backup structure
func (v *Validator) Validate(backupPath string) error {
	if _, err := os.Stat(backupPath); os.IsNotExist(err) {
		return fmt.Errorf("backup directory does not exist: %s", backupPath)
	}

	// Check for required files
	requiredFiles := []string{
		"status.json",
	}

	for _, file := range requiredFiles {
		filePath := filepath.Join(backupPath, file)
		if _, err := os.Stat(filePath); os.IsNotExist(err) {
			return fmt.Errorf("required file missing: %s", file)
		}
	}

	// Validate status.json
	status, err := v.validateStatus(backupPath)
	if err != nil {
		return fmt.Errorf("status.json validation failed: %w", err)
	}

	// Check if backup completed successfully
	if !v.isBackupComplete(status) {
		return fmt.Errorf("backup did not complete successfully")
	}

	return nil
}

// CheckRequiredFiles verifies all required files exist
func (v *Validator) CheckRequiredFiles(backupPath string) error {
	// Check directory exists
	if _, err := os.Stat(backupPath); os.IsNotExist(err) {
		return fmt.Errorf("backup directory does not exist: %s", backupPath)
	}

	// Optional entity files (don't require them to exist)
	entityFiles := []string{
		"products.json",
		"customers.json",
		"orders.json",
		"collections.json",
		"pages.json",
		"blogs.json",
		"metafields.json",
	}

	var missing []string
	for _, file := range entityFiles {
		filePath := filepath.Join(backupPath, file)
		if _, err := os.Stat(filePath); os.IsNotExist(err) {
			missing = append(missing, file)
		}
	}

	// Note: We don't return an error for missing entity files
	// They may legitimately be empty if there's no data

	return nil
}

// validateStatus validates the status.json file
func (v *Validator) validateStatus(backupPath string) (*BackupStatus, error) {
	statusPath := filepath.Join(backupPath, "status.json")

	data, err := os.ReadFile(statusPath)
	if err != nil {
		return nil, err
	}

	var status BackupStatus
	if err := json.Unmarshal(data, &status); err != nil {
		return nil, fmt.Errorf("failed to parse status.json: %w", err)
	}

	// Validate startedAt is set
	if status.StartedAt.IsZero() {
		return nil, fmt.Errorf("startedAt is not set in status.json")
	}

	return &status, nil
}

// isBackupComplete checks if the backup completed successfully
func (v *Validator) isBackupComplete(status *BackupStatus) bool {
	// Check if completedAt is set
	if status.CompletedAt.IsZero() {
		return false
	}

	// Check if all modules completed successfully (or at least have data)
	for _, moduleStatus := range status.Modules {
		if moduleStatus.Status == "failed" {
			// Failed modules don't invalidate the backup
			continue
		}
	}

	return true
}

// GetBackupFiles returns a list of all files in the backup directory
func (v *Validator) GetBackupFiles(backupPath string) ([]string, error) {
	var files []string

	err := filepath.Walk(backupPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			relPath, err := filepath.Rel(backupPath, path)
			if err != nil {
				return err
			}
			files = append(files, relPath)
		}
		return nil
	})

	return files, err
}