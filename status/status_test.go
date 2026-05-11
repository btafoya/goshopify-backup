package status

import (
	"os"
	"testing"
	"time"
)

func TestNewBackupStatus(t *testing.T) {
	status := NewBackupStatus()
	if status == nil {
		t.Fatal("NewBackupStatus() returned nil")
	}

	if status.StartedAt.IsZero() {
		t.Error("StartedAt should be set")
	}
	if !status.CompletedAt.IsZero() {
		t.Error("CompletedAt should be zero initially")
	}
	if status.Modules == nil {
		t.Error("Modules map should be initialized")
	}
}

func TestBackupStatus_MarkRunning(t *testing.T) {
	s := NewBackupStatus()
	s.MarkRunning("products")

	mod, ok := s.Modules["products"]
	if !ok {
		t.Fatal("Module not found")
	}

	if mod.Status != "running" {
		t.Errorf("Status = %v, want running", mod.Status)
	}
	if mod.StartedAt.IsZero() {
		t.Error("StartedAt should be set")
	}
}

func TestBackupStatus_MarkCompleted(t *testing.T) {
	s := NewBackupStatus()
	s.MarkRunning("products")
	time.Sleep(10 * time.Millisecond)
	s.MarkCompleted("products", 100, 1024)

	mod, ok := s.Modules["products"]
	if !ok {
		t.Fatal("Module not found")
	}

	if mod.Status != "completed" {
		t.Errorf("Status = %v, want completed", mod.Status)
	}
	if mod.Count != 100 {
		t.Errorf("Count = %v, want 100", mod.Count)
	}
	if mod.FileSize != 1024 {
		t.Errorf("FileSize = %v, want 1024", mod.FileSize)
	}
	if mod.CompletedAt.IsZero() {
		t.Error("CompletedAt should be set")
	}

	if s.TotalSize != 1024 {
		t.Errorf("TotalSize = %v, want 1024", s.TotalSize)
	}
}

func TestBackupStatus_MarkFailed(t *testing.T) {
	s := NewBackupStatus()
	s.MarkRunning("products")
	s.MarkFailed("products", "connection error", "REST")

	mod, ok := s.Modules["products"]
	if !ok {
		t.Fatal("Module not found")
	}

	if mod.Status != "failed" {
		t.Errorf("Status = %v, want failed", mod.Status)
	}
	if mod.Error != "connection error" {
		t.Errorf("Error = %v, want connection error", mod.Error)
	}
	if mod.Fallback != "REST" {
		t.Errorf("Fallback = %v, want REST", mod.Fallback)
	}

	// Failed modules should not contribute to total size
	if s.TotalSize != 0 {
		t.Errorf("TotalSize = %v, want 0", s.TotalSize)
	}
}

func TestBackupStatus_IsComplete(t *testing.T) {
	s := NewBackupStatus()

	// Initially not complete
	if s.IsComplete([]string{"products"}) {
		t.Error("Should not be complete initially")
	}

	// Mark as pending
	s.MarkRunning("products")
	if s.IsComplete([]string{"products"}) {
		t.Error("Should not be complete when running")
	}

	// Mark as completed
	s.MarkCompleted("products", 100, 1024)
	if !s.IsComplete([]string{"products"}) {
		t.Error("Should be complete when module is completed")
	}

	// Check with multiple modules
	s2 := NewBackupStatus()
	s2.MarkCompleted("products", 100, 1024)
	if s2.IsComplete([]string{"products", "customers"}) {
		t.Error("Should not be complete when one module is pending")
	}

	s2.MarkCompleted("customers", 50, 512)
	if !s2.IsComplete([]string{"products", "customers"}) {
		t.Error("Should be complete when all modules are completed")
	}
}

func TestBackupStatus_IsModuleCompleted(t *testing.T) {
	s := NewBackupStatus()

	if s.IsModuleCompleted("products") {
		t.Error("Should not be completed initially")
	}

	s.MarkCompleted("products", 100, 1024)
	if !s.IsModuleCompleted("products") {
		t.Error("Should be completed after marking")
	}
}

func TestBackupStatus_IsModuleFailed(t *testing.T) {
	s := NewBackupStatus()

	if s.IsModuleFailed("products") {
		t.Error("Should not be failed initially")
	}

	s.MarkFailed("products", "error", "")
	if !s.IsModuleFailed("products") {
		t.Error("Should be failed after marking")
	}
}

func TestBackupStatus_ApplyUpdate(t *testing.T) {
	s := NewBackupStatus()

	update := StatusUpdate{
		Module:    "products",
		Status:    "running",
		Timestamp: time.Now().UTC(),
	}
	s.ApplyUpdate(update)

	if s.Modules["products"].Status != "running" {
		t.Error("Status should be running after ApplyUpdate")
	}
}

func TestNewWriter(t *testing.T) {
	w := NewWriter("/tmp/test-backup", 5*time.Second)
	if w == nil {
		t.Fatal("NewWriter() returned nil")
	}

	if w.status == nil {
		t.Error("status should be initialized")
	}
	if w.outputDir != "/tmp/test-backup" {
		t.Errorf("outputDir = %v, want /tmp/test-backup", w.outputDir)
	}
	if w.flushInterval != 5*time.Second {
		t.Errorf("flushInterval = %v, want 5s", w.flushInterval)
	}
}

func TestWriter_Initialize(t *testing.T) {
	tmpDir, _ := os.MkdirTemp("", "status-test-")
	defer os.RemoveAll(tmpDir)

	w := NewWriter(tmpDir, 5*time.Second)
	modules := []string{"products", "customers", "orders"}

	err := w.Initialize(modules)
	if err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}

	if len(w.status.Modules) != 3 {
		t.Errorf("Modules count = %v, want 3", len(w.status.Modules))
	}

	if w.status.Modules["products"].Status != "pending" {
		t.Errorf("products status = %v, want pending", w.status.Modules["products"].Status)
	}
}

func TestWriter_Update(t *testing.T) {
	tmpDir, _ := os.MkdirTemp("", "status-test-")
	defer os.RemoveAll(tmpDir)

	w := NewWriter(tmpDir, 5*time.Second)
	modules := []string{"products"}
	_ = w.Initialize(modules)

	update := StatusUpdate{
		Module:    "products",
		Status:    "running",
		Timestamp: time.Now().UTC(),
	}

	err := w.Update(update)
	if err != nil {
		t.Fatalf("Update() error = %v", err)
	}

	if w.status.Modules["products"].Status != "running" {
		t.Errorf("Module status = %v, want running", w.status.Modules["products"].Status)
	}
}

func TestWriter_MarkBackupComplete(t *testing.T) {
	tmpDir, _ := os.MkdirTemp("", "status-test-")
	defer os.RemoveAll(tmpDir)

	w := NewWriter(tmpDir, 5*time.Second)
	modules := []string{"products"}
	_ = w.Initialize(modules)
	w.status.MarkCompleted("products", 100, 1024)

	err := w.MarkBackupComplete()
	if err != nil {
		t.Fatalf("MarkBackupComplete() error = %v", err)
	}

	if w.status.CompletedAt.IsZero() {
		t.Error("CompletedAt should be set")
	}
	if w.status.Duration == "" {
		t.Error("Duration should be set")
	}
}
