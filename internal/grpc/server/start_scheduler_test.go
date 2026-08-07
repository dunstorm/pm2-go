package server

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRotateLogFilePreservesActivePathForAppendHandles(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "api-combined.jsonl")
	rotatedPath := logPath + ".0"

	activeFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		t.Fatalf("open active log: %v", err)
	}
	if _, err := activeFile.WriteString("before\n"); err != nil {
		t.Fatalf("write before rotate: %v", err)
	}

	rotated, err := rotateLogFile(logPath, rotatedPath)
	if err != nil {
		t.Fatalf("rotate log: %v", err)
	}
	if !rotated {
		t.Fatal("expected log file to rotate")
	}
	if _, err := activeFile.WriteString("after\n"); err != nil {
		t.Fatalf("write after rotate: %v", err)
	}
	if err := activeFile.Close(); err != nil {
		t.Fatalf("close active log: %v", err)
	}

	rotatedContents, err := os.ReadFile(rotatedPath)
	if err != nil {
		t.Fatalf("read rotated log: %v", err)
	}
	if string(rotatedContents) != "before\n" {
		t.Fatalf("expected rotated contents to preserve old log, got %q", string(rotatedContents))
	}

	activeContents, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read active log: %v", err)
	}
	if string(activeContents) != "after\n" {
		t.Fatalf("expected active path to receive new writes, got %q", string(activeContents))
	}
}
