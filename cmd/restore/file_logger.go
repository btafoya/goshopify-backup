package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// FileLogger writes structured JSON log entries to a file (O3)
type FileLogger struct {
	mu      sync.Mutex
	file    *os.File
	logDir  string
	started time.Time
}

// FileLogEntry represents a structured log entry for the file logger
type FileLogEntry struct {
	Timestamp string `json:"timestamp"`
	Level     string `json:"level"`
	Message   string `json:"message"`
	Entity    string `json:"entity,omitempty"`
	ItemID    string `json:"itemId,omitempty"`
	Action    string `json:"action,omitempty"`
	Duration  string `json:"duration,omitempty"`
	Error     string `json:"error,omitempty"`
}

// NewFileLogger creates a new file logger
func NewFileLogger(logDir string) (*FileLogger, error) {
	if err := os.MkdirAll(logDir, 0755); err != nil {
		return nil, fmt.Errorf("create log directory: %w", err)
	}

	filename := fmt.Sprintf(LogFile, time.Now().Format(DateFormat))
	filePath := filepath.Join(logDir, filename)

	f, err := os.OpenFile(filePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return nil, fmt.Errorf("open log file: %w", err)
	}

	return &FileLogger{
		file:    f,
		logDir:  logDir,
		started: time.Now(),
	}, nil
}

// Info logs an info-level entry
func (l *FileLogger) Info(msg string, fields ...FileLogEntry) {
	entry := FileLogEntry{
		Timestamp: time.Now().Format(time.RFC3339Nano),
		Level:     "info",
		Message:   msg,
	}
	if len(fields) > 0 {
		l.mergeFields(&entry, fields[0])
	}
	l.write(entry)
}

// Warn logs a warning-level entry
func (l *FileLogger) Warn(msg string, fields ...FileLogEntry) {
	entry := FileLogEntry{
		Timestamp: time.Now().Format(time.RFC3339Nano),
		Level:     "warn",
		Message:   msg,
	}
	if len(fields) > 0 {
		l.mergeFields(&entry, fields[0])
	}
	l.write(entry)
}

// Error logs an error-level entry
func (l *FileLogger) Error(msg string, fields ...FileLogEntry) {
	entry := FileLogEntry{
		Timestamp: time.Now().Format(time.RFC3339Nano),
		Level:     "error",
		Message:   msg,
	}
	if len(fields) > 0 {
		l.mergeFields(&entry, fields[0])
	}
	l.write(entry)
}

// RestoreItem logs a restore item action
func (l *FileLogger) RestoreItem(entityType EntityType, itemID, action, restoredID string, duration time.Duration, err error) {
	entry := FileLogEntry{
		Timestamp: time.Now().Format(time.RFC3339Nano),
		Level:     "info",
		Message:   fmt.Sprintf("Restore %s: %s", action, itemID),
		Entity:    string(entityType),
		ItemID:    itemID,
		Action:    action,
		Duration:  duration.String(),
	}
	if err != nil {
		entry.Level = "error"
		entry.Error = err.Error()
	}
	if restoredID != "" {
		entry.Message = fmt.Sprintf("Restore %s: %s -> %s", action, itemID, restoredID)
	}
	l.write(entry)
}

func (l *FileLogger) mergeFields(target *FileLogEntry, source FileLogEntry) {
	if source.Entity != "" {
		target.Entity = source.Entity
	}
	if source.ItemID != "" {
		target.ItemID = source.ItemID
	}
	if source.Action != "" {
		target.Action = source.Action
	}
	if source.Duration != "" {
		target.Duration = source.Duration
	}
	if source.Error != "" {
		target.Error = source.Error
	}
}

func (l *FileLogger) write(entry FileLogEntry) {
	data, err := json.Marshal(entry)
	if err != nil {
		return
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	l.file.Write(data)
	l.file.Write([]byte("\n"))
}

// Close closes the log file
func (l *FileLogger) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.file != nil {
		return l.file.Close()
	}
	return nil
}

// FilePath returns the path to the log file
func (l *FileLogger) FilePath() string {
	if l.file != nil {
		return l.file.Name()
	}
	return ""
}
