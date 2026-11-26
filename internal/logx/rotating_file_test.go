package logx

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRotatingFileWriterRotatesOnSizeLimit(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "logs", "service.log")

	w, err := NewRotatingFileWriter(logPath, 64)
	if err != nil {
		t.Fatalf("NewRotatingFileWriter: %v", err)
	}
	defer w.Close()

	line1 := strings.Repeat("A", 50) + "\n"
	line2 := strings.Repeat("B", 30) + "\n"

	if _, err := w.Write([]byte(line1)); err != nil {
		t.Fatalf("write line1: %v", err)
	}
	if _, err := w.Write([]byte(line2)); err != nil {
		t.Fatalf("write line2: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	active, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read active log: %v", err)
	}
	backup, err := os.ReadFile(logPath + ".1")
	if err != nil {
		t.Fatalf("read rotated backup log: %v", err)
	}

	if string(backup) != line1 {
		t.Fatalf("backup = %q, want %q", string(backup), line1)
	}
	if string(active) != line2 {
		t.Fatalf("active = %q, want %q", string(active), line2)
	}
}
