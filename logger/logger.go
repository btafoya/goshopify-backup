package logger

import (
	"os"

	"github.com/sirupsen/logrus"
)

// LogFields for structured logging
type LogFields struct {
	Module   string `json:"module"`
	Action   string `json:"action"`
	Store    string `json:"store,omitempty"`
	Count    int    `json:"count,omitempty"`
	Error    string `json:"error,omitempty"`
	Duration string `json:"duration,omitempty"`
}

// Logger provides structured JSON logging
type Logger struct {
	logger *logrus.Logger
}

// NewLogger creates a new structured logger with JSON formatter
func NewLogger(level string) *Logger {
	log := logrus.New()
	log.SetFormatter(&logrus.JSONFormatter{})
	log.SetOutput(os.Stdout)

	switch level {
	case "debug":
		log.SetLevel(logrus.DebugLevel)
	case "warn":
		log.SetLevel(logrus.WarnLevel)
	case "error":
		log.SetLevel(logrus.ErrorLevel)
	default:
		log.SetLevel(logrus.InfoLevel)
	}

	return &Logger{logger: log}
}

// Info logs at info level with fields
func (l *Logger) Info(message string, fields LogFields) {
	l.logger.WithFields(logrus.Fields{
		"module":   fields.Module,
		"action":   fields.Action,
		"store":    fields.Store,
		"count":    fields.Count,
		"error":    fields.Error,
		"duration": fields.Duration,
	}).Info(message)
}

// Warn logs at warn level with fields
func (l *Logger) Warn(message string, fields LogFields) {
	l.logger.WithFields(logrus.Fields{
		"module":   fields.Module,
		"action":   fields.Action,
		"store":    fields.Store,
		"count":    fields.Count,
		"error":    fields.Error,
		"duration": fields.Duration,
	}).Warn(message)
}

// Error logs at error level with fields
func (l *Logger) Error(message string, fields LogFields) {
	l.logger.WithFields(logrus.Fields{
		"module":   fields.Module,
		"action":   fields.Action,
		"store":    fields.Store,
		"count":    fields.Count,
		"error":    fields.Error,
		"duration": fields.Duration,
	}).Error(message)
}

// Debug logs at debug level with fields
func (l *Logger) Debug(message string, fields LogFields) {
	l.logger.WithFields(logrus.Fields{
		"module":   fields.Module,
		"action":   fields.Action,
		"store":    fields.Store,
		"count":    fields.Count,
		"error":    fields.Error,
		"duration": fields.Duration,
	}).Debug(message)
}