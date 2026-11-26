package logx

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

const (
	// DefaultLogMaxBytes is the default single-file rotation threshold (10 MiB).
	DefaultLogMaxBytes int64 = 10 * 1024 * 1024
)

// RotatingFileWriter writes log output to a file with single-backup size rotation
// (e.g. service.log -> service.log.1) and restricted permissions (0600 file / 0700 dir).
type RotatingFileWriter struct {
	mu       sync.Mutex
	path     string
	maxBytes int64
	file     *os.File
	size     int64
}

// NewRotatingFileWriter opens (or creates) path for appending, rotating immediately
// if the existing file already exceeds maxBytes.
func NewRotatingFileWriter(path string, maxBytes int64) (*RotatingFileWriter, error) {
	if path == "" {
		return nil, fmt.Errorf("log file path is empty")
	}
	if maxBytes <= 0 {
		maxBytes = DefaultLogMaxBytes
	}
	absPath, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("resolve log file path: %w", err)
	}
	dir := filepath.Dir(absPath)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, fmt.Errorf("create log directory %s: %w", dir, err)
	}
	w := &RotatingFileWriter{
		path:     absPath,
		maxBytes: maxBytes,
	}
	if err := w.openLocked(); err != nil {
		return nil, err
	}
	return w, nil
}

func (w *RotatingFileWriter) openLocked() error {
	if fi, err := os.Stat(w.path); err == nil && fi.Size() >= w.maxBytes {
		_ = rotateBackupFile(w.path)
	}
	f, err := os.OpenFile(w.path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return fmt.Errorf("open log file %s: %w", w.path, err)
	}
	_ = os.Chmod(w.path, 0600)
	var size int64
	if fi, err := f.Stat(); err == nil {
		size = fi.Size()
	}
	w.file = f
	w.size = size
	return nil
}

func rotateBackupFile(path string) error {
	backup := path + ".1"
	_ = os.Remove(backup)
	return os.Rename(path, backup)
}

// Write implements io.Writer with thread-safe size-based rotation.
func (w *RotatingFileWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.file == nil {
		if err := w.openLocked(); err != nil {
			return 0, err
		}
	}
	if w.size+int64(len(p)) > w.maxBytes && w.size > 0 {
		_ = w.file.Close()
		w.file = nil
		_ = rotateBackupFile(w.path)
		if err := w.openLocked(); err != nil {
			return 0, err
		}
	}
	n, err := w.file.Write(p)
	w.size += int64(n)
	return n, err
}

// Close closes the underlying log file.
func (w *RotatingFileWriter) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.file == nil {
		return nil
	}
	err := w.file.Close()
	w.file = nil
	return err
}

// Path returns the resolved absolute path of the active log file.
func (w *RotatingFileWriter) Path() string {
	return w.path
}
