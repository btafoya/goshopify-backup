package status

import "time"

// ModuleStatus represents the status of a single backup module
type ModuleStatus struct {
	Status      string    `json:"status"`      // "pending", "running", "completed", "failed"
	StartedAt   time.Time `json:"startedAt"`
	CompletedAt time.Time `json:"completedAt,omitempty"`
	Count       int       `json:"count"`
	Error       string    `json:"error,omitempty"`
	FileSize    int64     `json:"fileSize,omitempty"`
	Fallback    string    `json:"fallback,omitempty"` // "REST" if bulk operation fell back
}

// BackupStatus is written to status.json
type BackupStatus struct {
	StartedAt    time.Time                 `json:"startedAt"`
	CompletedAt  time.Time                 `json:"completedAt,omitempty"`
	Duration     string                    `json:"duration,omitempty"`
	Modules      map[string]ModuleStatus  `json:"modules"`
	TotalSize    int64                     `json:"totalSize,omitempty"`
}

// StatusUpdate represents a single status change
type StatusUpdate struct {
	Module    string
	Status    string
	Count     int
	Error     string
	FileSize  int64
	Fallback  string
	Timestamp time.Time
}

// NewBackupStatus creates a new backup status
func NewBackupStatus() *BackupStatus {
	return &BackupStatus{
		StartedAt:   time.Now().UTC(),
		Modules:     make(map[string]ModuleStatus),
	}
}

// MarkRunning marks a module as running
func (s *BackupStatus) MarkRunning(module string) {
	s.Modules[module] = ModuleStatus{
		Status:    "running",
		StartedAt: time.Now().UTC(),
	}
}

// MarkCompleted marks a module as completed
func (s *BackupStatus) MarkCompleted(module string, count int, fileSize int64) {
	if existing, ok := s.Modules[module]; ok {
		s.Modules[module] = ModuleStatus{
			Status:      "completed",
			StartedAt:   existing.StartedAt,
			CompletedAt: time.Now().UTC(),
			Count:       count,
			FileSize:    fileSize,
			Fallback:    existing.Fallback,
		}
	} else {
		s.Modules[module] = ModuleStatus{
			Status:      "completed",
			StartedAt:   time.Now().UTC(),
			CompletedAt: time.Now().UTC(),
			Count:       count,
			FileSize:    fileSize,
		}
	}
	s.TotalSize += fileSize
}

// MarkFailed marks a module as failed
func (s *BackupStatus) MarkFailed(module string, err string, fallback string) {
	if existing, ok := s.Modules[module]; ok {
		s.Modules[module] = ModuleStatus{
			Status:      "failed",
			StartedAt:   existing.StartedAt,
			CompletedAt: time.Now().UTC(),
			Error:       err,
			Fallback:    fallback,
		}
	} else {
		s.Modules[module] = ModuleStatus{
			Status:      "failed",
			StartedAt:   time.Now().UTC(),
			CompletedAt: time.Now().UTC(),
			Error:       err,
			Fallback:    fallback,
		}
	}
}

// IsComplete checks if all modules are complete or failed
func (s *BackupStatus) IsComplete(expectedModules []string) bool {
	for _, mod := range expectedModules {
		if status, ok := s.Modules[mod]; !ok {
			return false
		} else if status.Status == "pending" || status.Status == "running" {
			return false
		}
	}
	return true
}

// IsModuleCompleted checks if a specific module is completed
func (s *BackupStatus) IsModuleCompleted(module string) bool {
	status, ok := s.Modules[module]
	return ok && status.Status == "completed"
}

// IsModuleFailed checks if a specific module failed
func (s *BackupStatus) IsModuleFailed(module string) bool {
	status, ok := s.Modules[module]
	return ok && status.Status == "failed"
}

// ApplyUpdate applies a status update to the backup status
func (s *BackupStatus) ApplyUpdate(update StatusUpdate) {
	switch update.Status {
	case "running":
		s.MarkRunning(update.Module)
	case "completed":
		s.MarkCompleted(update.Module, update.Count, update.FileSize)
	case "failed":
		s.MarkFailed(update.Module, update.Error, update.Fallback)
	}
}