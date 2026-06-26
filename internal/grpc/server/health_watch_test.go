package server

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWatchSignatureIncludesDirectoryChildren(t *testing.T) {
	dir := t.TempDir()
	watchedFile := filepath.Join(dir, "watched.txt")
	if err := os.WriteFile(watchedFile, []byte("initial\n"), 0644); err != nil {
		t.Fatalf("write watched file: %v", err)
	}

	initial, err := watchSignature(dir)
	if err != nil {
		t.Fatalf("initial watch signature: %v", err)
	}

	if err := os.WriteFile(watchedFile, []byte("changed file contents\n"), 0644); err != nil {
		t.Fatalf("update watched file: %v", err)
	}

	changed, err := watchSignature(dir)
	if err != nil {
		t.Fatalf("changed watch signature: %v", err)
	}
	if initial == changed {
		t.Fatal("expected directory signature to change when a child file changes")
	}
}
