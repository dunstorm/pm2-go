package cli

import (
	"os"
	"path/filepath"
	"testing"
)

func TestIsJSONFilePath(t *testing.T) {
	tmp := t.TempDir()

	shortFile := filepath.Join(tmp, "x")
	if err := os.WriteFile(shortFile, []byte("{}"), 0600); err != nil {
		t.Fatalf("write short file: %v", err)
	}
	if isJSONFilePath(shortFile) {
		t.Fatal("expected short non-json file to be ignored")
	}

	jsonFile := filepath.Join(tmp, "app.JSON")
	if err := os.WriteFile(jsonFile, []byte("{}"), 0600); err != nil {
		t.Fatalf("write json file: %v", err)
	}
	if !isJSONFilePath(jsonFile) {
		t.Fatal("expected json file to be detected")
	}

	jsonDir := filepath.Join(tmp, "dir.json")
	if err := os.Mkdir(jsonDir, 0700); err != nil {
		t.Fatalf("create json directory: %v", err)
	}
	if isJSONFilePath(jsonDir) {
		t.Fatal("expected json directory to be ignored")
	}
}
