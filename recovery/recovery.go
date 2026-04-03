package recovery

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/btafoya/goshopify-backup/status"
)

// Manager handles resuming from incomplete backups
type Manager struct {
	backupDir string
}

// NewManager creates a new recovery manager
func NewManager(backupDir string) *Manager {
	return &Manager{
		backupDir: backupDir,
	}
}

// LoadStatus reads the existing status.json if it exists
func (m *Manager) LoadStatus(date string) (*status.BackupStatus, error) {
	dateDir := m.dateDir(date)
	return status.LoadStatus(dateDir)
}

// GetCompletedModules returns list of modules that completed successfully
func (m *Manager) GetCompletedModules(status *status.BackupStatus) []string {
	if status == nil {
		return nil
	}

	completed := make([]string, 0)
	for module, modStatus := range status.Modules {
		if modStatus.Status == "completed" {
			completed = append(completed, module)
		}
	}
	return completed
}

// GetFailedModules returns list of modules that failed
func (m *Manager) GetFailedModules(status *status.BackupStatus) []string {
	if status == nil {
		return nil
	}

	failed := make([]string, 0)
	for module, modStatus := range status.Modules {
		if modStatus.Status == "failed" {
			failed = append(failed, module)
		}
	}
	return failed
}

// GetPendingModules returns list of modules that haven't run yet
func (m *Manager) GetPendingModules(status *status.BackupStatus, expectedModules []string) []string {
	if status == nil {
		return expectedModules
	}

	pending := make([]string, 0)
	for _, module := range expectedModules {
		if modStatus, ok := status.Modules[module]; !ok {
			pending = append(pending, module)
		} else if modStatus.Status == "pending" {
			pending = append(pending, module)
		}
	}
	return pending
}

// ShouldResume returns true if backup should resume (not start fresh)
// Returns true if status.json exists and is from today
func (m *Manager) ShouldResume(date string) (bool, *status.BackupStatus, error) {
	backupStatus, err := m.LoadStatus(date)
	if err != nil {
		return false, nil, err
	}
	if backupStatus == nil {
		return false, nil, nil // No existing status
	}

	// Check if backup is from today (same date)
	backupDate := backupStatus.StartedAt.Format("2006-01-02")
	if backupDate != date {
		// Backup is from a different date, don't resume
		return false, nil, nil
	}

	return true, backupStatus, nil
}

// GetModulesToRun returns the list of modules that should be run
// Skips completed modules unless force is true
func (m *Manager) GetModulesToRun(date string, expectedModules []string, force bool) ([]string, error) {
	shouldResume, backupStatus, err := m.ShouldResume(date)
	if err != nil {
		return nil, err
	}

	if !shouldResume || force {
		return expectedModules, nil
	}

	modulesToRun := make([]string, 0)
	for _, module := range expectedModules {
		modStatus, ok := backupStatus.Modules[module]
		if !ok {
			// Module not in status, run it
			modulesToRun = append(modulesToRun, module)
			continue
		}

		// If module failed, retry it
		if modStatus.Status == "failed" {
			modulesToRun = append(modulesToRun, module)
			continue
		}

		// If module is running, skip it (assume still running)
		if modStatus.Status == "running" {
			continue
		}

		// If module is pending, run it
		if modStatus.Status == "pending" {
			modulesToRun = append(modulesToRun, module)
			continue
		}

		// Module completed, skip unless force (handled above)
	}

	return modulesToRun, nil
}

// DateDir returns the backup directory for a given date
func (m *Manager) dateDir(date string) string {
	return filepath.Join(m.backupDir, date)
}

// Exists returns true if the backup directory exists for the given date
func (m *Manager) Exists(date string) bool {
	dateDir := m.dateDir(date)
	info, err := os.Stat(dateDir)
	if err != nil {
		return false
	}
	return info.IsDir()
}

// CleanOldLock removes stale lock files from old backup directories
func (m *Manager) CleanOldLock(date string) error {
	dateDir := m.dateDir(date)
	lockPath := filepath.Join(dateDir, ".lock")

	// Check if lock exists and is stale
	lockFile := lockPath
	if info, err := os.Stat(lockFile); err == nil {
		// Lock exists, check if it's > 24 hours old
		if time.Since(info.ModTime()) > 24*time.Hour {
			if err := os.Remove(lockFile); err != nil {
				return fmt.Errorf("failed to remove old lock: %w", err)
			}
		}
	}

	return nil
}