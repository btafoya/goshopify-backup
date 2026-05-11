package status

import (
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"time"
)

// Writer is a buffered channel-based status file writer
// Batches updates and flushes every 5 seconds or on module completion
type Writer struct {
	status        *BackupStatus
	mu            sync.Mutex
	outputDir     string
	flushInterval time.Duration
	done          chan struct{}
	initialized   bool
}

// NewWriter creates a buffered status writer
func NewWriter(outputDir string, flushInterval time.Duration) *Writer {
	return &Writer{
		status:        NewBackupStatus(),
		outputDir:     outputDir,
		flushInterval: flushInterval,
		done:          make(chan struct{}),
		initialized:   false,
	}
}

// Initialize creates the initial status file
func (w *Writer) Initialize(expectedModules []string) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.initialized {
		return nil
	}

	// Initialize all modules as pending
	for _, mod := range expectedModules {
		w.status.Modules[mod] = ModuleStatus{
			Status: "pending",
		}
	}

	w.initialized = true
	return w.Write()
}

// Update sends a status update (blocking, applies directly)
func (w *Writer) Update(update StatusUpdate) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	w.status.ApplyUpdate(update)
	return w.Write()
}

// Write writes the current status to disk
func (w *Writer) Write() error {
	if !w.initialized {
		return fmt.Errorf("writer not initialized")
	}

	data, err := json.MarshalIndent(w.status, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal status: %w", err)
	}

	path := w.statusFilePath()
	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("failed to write status file: %w", err)
	}

	return nil
}

// statusFilePath returns the path to the status.json file
func (w *Writer) statusFilePath() string {
	return w.outputDir + "/status.json"
}

// MarkBackupComplete marks the entire backup as complete
func (w *Writer) MarkBackupComplete() error {
	w.mu.Lock()
	defer w.mu.Unlock()

	w.status.CompletedAt = time.Now().UTC()
	w.status.Duration = w.status.CompletedAt.Sub(w.status.StartedAt).String()
	return w.Write()
}

// GetStatus returns a copy of the current status
func (w *Writer) GetStatus() BackupStatus {
	w.mu.Lock()
	defer w.mu.Unlock()

	data, _ := json.Marshal(w.status)
	var copy BackupStatus
	json.Unmarshal(data, &copy)
	return copy
}

// Close stops the writer (no-op for this simple implementation)
func (w *Writer) Close() error {
	close(w.done)
	return nil
}

// LoadStatus loads an existing status from disk
func LoadStatus(outputDir string) (*BackupStatus, error) {
	path := outputDir + "/status.json"
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil // No existing status
		}
		return nil, fmt.Errorf("failed to read status file: %w", err)
	}

	var status BackupStatus
	if err := json.Unmarshal(data, &status); err != nil {
		return nil, fmt.Errorf("failed to unmarshal status: %w", err)
	}

	return &status, nil
}
