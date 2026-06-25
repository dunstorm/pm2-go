package utils

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFindOrCreateConfigFileCreatesHomeDirectory(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	configFile := FindOrCreateConfigFile()
	expectedConfigFile := filepath.Join(home, ".pm2-go", "config.json")
	if configFile != expectedConfigFile {
		t.Fatalf("expected config file %q, got %q", expectedConfigFile, configFile)
	}

	for _, dir := range []string{
		filepath.Join(home, ".pm2-go"),
		filepath.Join(home, ".pm2-go", "pids"),
		filepath.Join(home, ".pm2-go", "logs"),
	} {
		info, err := os.Stat(dir)
		if err != nil {
			t.Fatalf("expected directory %q to exist: %v", dir, err)
		}
		if !info.IsDir() {
			t.Fatalf("expected %q to be a directory", dir)
		}
	}
}
