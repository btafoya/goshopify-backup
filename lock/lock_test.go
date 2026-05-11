package lock

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestNewLockManager(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "lock-test-")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	lm := NewManager(tmpDir, 24*time.Hour)
	if lm == nil {
		t.Fatal("NewLockManager() returned nil")
	}

	if lm.lockDir != tmpDir {
		t.Errorf("lockDir = %v, want %v", lm.lockDir, tmpDir)
	}
	if lm.staleLockDuration != 24*time.Hour {
		t.Errorf("staleLockDuration = %v, want 24h", lm.staleLockDuration)
	}
}

func TestLockManager_Acquire(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "lock-test-")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	lm := NewManager(tmpDir, 24*time.Hour)

	// First acquire should succeed
	err = lm.Acquire("2026-04-03")
	if err != nil {
		t.Fatalf("First Acquire() error = %v", err)
	}

	// Second acquire should fail
	err = lm.Acquire("2026-04-03")
	if err == nil {
		t.Error("Second Acquire() should fail")
	}

	// Clean up
	_ = lm.Release("2026-04-03")
}

func TestLockManager_Release(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "lock-test-")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	lm := NewManager(tmpDir, 24*time.Hour)

	_ = lm.Acquire("2026-04-03")

	err = lm.Release("2026-04-03")
	if err != nil {
		t.Fatalf("Release() error = %v", err)
	}

	// Verify lock file is removed
	lockPath := filepath.Join(tmpDir, ".lock")
	if _, err := os.Stat(lockPath); !os.IsNotExist(err) {
		t.Error("Lock file should be removed after Release()")
	}
}

func TestLockManager_IsStale(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "lock-test-")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	lm := NewManager(tmpDir, 1*time.Hour) // 1 hour stale duration

	_ = lm.Acquire("2026-04-03")

	// Lock should not be stale
	isStale, err := lm.IsStale("2026-04-03")
	if err != nil {
		t.Fatalf("IsStale() error = %v", err)
	}
	if isStale {
		t.Error("Lock should not be stale immediately")
	}

	// Create a lock file with old timestamp
	oldLockPath := filepath.Join(tmpDir, "old-test")
	os.MkdirAll(oldLockPath, 0755)
	lockFile := filepath.Join(oldLockPath, ".lock")
	lockContent := `{"pid": 12345, "startedAt": "2020-01-01T00:00:00Z"}`
	os.WriteFile(lockFile, []byte(lockContent), 0644)

	// Old lock should be stale
	isStale, err = lm.IsStale("old-test")
	if err != nil {
		t.Fatalf("IsStale() error = %v", err)
	}
	if !isStale {
		t.Error("Old lock should be stale")
	}

	// Clean up
	_ = lm.Release("2026-04-03")
}

func TestLockManager_Read(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "lock-test-")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	lm := NewManager(tmpDir, 24*time.Hour)

	// Read before lock exists
	lockData, err := lm.Read("2026-04-03")
	if err != nil {
		t.Fatalf("Read() error = %v", err)
	}
	if lockData != nil {
		t.Error("Read() should return nil when lock doesn't exist")
	}

	// Create lock
	_ = lm.Acquire("2026-04-03")

	// Read existing lock
	lockData, err = lm.Read("2026-04-03")
	if err != nil {
		t.Fatalf("Read() error = %v", err)
	}
	if lockData == nil {
		t.Error("Read() should return lock data when lock exists")
	}
	if lockData.PID != os.Getpid() {
		t.Errorf("Lock PID = %v, want %d", lockData.PID, os.Getpid())
	}

	// Clean up
	_ = lm.Release("2026-04-03")
}

func TestLockManager_StaleLockOverride(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "lock-test-")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create a lock with very short stale duration
	lm := NewManager(tmpDir, 10*time.Millisecond)
	_ = lm.Acquire("2026-04-03")

	// Wait for lock to become stale
	time.Sleep(20 * time.Millisecond)

	// New acquire should succeed (stale lock is removed)
	err = lm.Acquire("2026-04-03")
	if err != nil {
		t.Errorf("Acquire() should succeed with stale lock, error = %v", err)
	}

	// Clean up
	_ = lm.Release("2026-04-03")
}

func TestLockFile_MarshalUnmarshal(t *testing.T) {
	// Write lock file
	lockPath := filepath.Join(os.TempDir(), "lock_test_file")
	data := []byte(`{"pid":12345,"startedAt":"2026-04-03T12:00:00Z"}`)
	err := os.WriteFile(lockPath, data, 0644)
	if err != nil {
		t.Fatalf("Failed to write lock file: %v", err)
	}
	defer os.Remove(lockPath)

	// Verify lock file can be read back
	readData, err := os.ReadFile(lockPath)
	if err != nil {
		t.Fatalf("Failed to read lock file: %v", err)
	}
	if string(readData) != string(data) {
		t.Errorf("Lock file content mismatch")
	}
}
