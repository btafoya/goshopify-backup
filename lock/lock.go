package lock

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// LockFile represents the lock file content
type LockFile struct {
	PID       int       `json:"pid"`
	StartedAt time.Time `json:"startedAt"`
}

// Manager handles concurrent backup prevention
type Manager struct {
	lockDir           string
	staleLockDuration time.Duration
}

// NewManager creates a new lock manager
func NewManager(lockDir string, staleLockDuration time.Duration) *Manager {
	return &Manager{
		lockDir:           lockDir,
		staleLockDuration: staleLockDuration,
	}
}

// Acquire attempts to acquire a lock for the backup date
// Returns error if lock exists and is recent
func (m *Manager) Acquire(date string) error {
	lockPath := m.lockPath(date)

	// Check if lock exists
	if _, err := os.Stat(lockPath); err == nil {
		// Lock exists, check if it's stale
		isStale, err := m.IsStale(date)
		if err != nil {
			return fmt.Errorf("failed to check lock staleness: %w", err)
		}
		if !isStale {
			// Lock is active, return error
			lockData, _ := m.Read(date)
			pid := "unknown"
			if lockData != nil {
				pid = fmt.Sprintf("%d", lockData.PID)
			}
			return fmt.Errorf("backup in progress (PID: %s). Use --force to override", pid)
		}
		// Lock is stale, remove it
		if err := m.Release(date); err != nil {
			return fmt.Errorf("failed to remove stale lock: %w", err)
		}
	}

	// Create lock
	lockData := LockFile{
		PID:       os.Getpid(),
		StartedAt: time.Now().UTC(),
	}

	data, err := json.Marshal(lockData)
	if err != nil {
		return fmt.Errorf("failed to marshal lock file: %w", err)
	}

	// Ensure lock directory exists (including date subdirectory)
	lockDirPath := filepath.Dir(lockPath)
	if err := os.MkdirAll(lockDirPath, 0755); err != nil {
		return fmt.Errorf("failed to create lock directory: %w", err)
	}

	if err := os.WriteFile(lockPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write lock file: %w", err)
	}

	return nil
}

// Release removes the lock file
func (m *Manager) Release(date string) error {
	lockPath := m.lockPath(date)
	if err := os.Remove(lockPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove lock file: %w", err)
	}
	return nil
}

// IsStale checks if a lock file is stale (> staleLockDuration)
func (m *Manager) IsStale(date string) (bool, error) {
	lockData, err := m.Read(date)
	if err != nil {
		return false, err
	}
	if lockData == nil {
		return true, nil // No lock file exists
	}

	age := time.Since(lockData.StartedAt)
	return age >= m.staleLockDuration, nil
}

// Read reads the lock file
func (m *Manager) Read(date string) (*LockFile, error) {
	lockPath := m.lockPath(date)

	data, err := os.ReadFile(lockPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to read lock file: %w", err)
	}

	var lockData LockFile
	if err := json.Unmarshal(data, &lockData); err != nil {
		return nil, fmt.Errorf("failed to unmarshal lock file: %w", err)
	}

	return &lockData, nil
}

// lockPath returns the path to the lock file
func (m *Manager) lockPath(date string) string {
	return filepath.Join(m.lockDir, date, ".lock")
}