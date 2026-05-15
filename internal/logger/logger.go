// Package logger initializes the application log file at
// ~/.go-mcp/vision/vision-mcp.log and sets Go's standard log package
// to write to both stderr and the file.
package logger

import (
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"

	"github.com/cristian-guerrero/go-vision-mcp/internal/config"
)

// Logger wraps the log file for deferred closing.
type Logger struct {
	file *os.File
}

// Init opens the log file in append mode and configures the standard
// log package to write to both stderr and the file simultaneously.
func Init() (*Logger, error) {
	logDir := config.InstallDir()
	if err := os.MkdirAll(logDir, 0755); err != nil {
		return nil, fmt.Errorf("create log dir: %w", err)
	}

	logPath := filepath.Join(logDir, "vision-mcp.log")
	f, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return nil, fmt.Errorf("open log file: %w", err)
	}

	multi := io.MultiWriter(os.Stderr, f)
	log.SetOutput(multi)
	log.SetFlags(log.Ldate | log.Ltime | log.Lshortfile)

	return &Logger{file: f}, nil
}

// Close closes the underlying log file.
func (l *Logger) Close() error {
	if l.file != nil {
		return l.file.Close()
	}
	return nil
}
