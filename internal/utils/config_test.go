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

func TestGetMainDirectoryUsesPM2GoHome(t *testing.T) {
	pm2Home := filepath.Join(t.TempDir(), "runtime")
	t.Setenv("PM2_GO_HOME", pm2Home)

	if got := GetMainDirectory(); got != pm2Home {
		t.Fatalf("expected PM2_GO_HOME directory %q, got %q", pm2Home, got)
	}

	for _, dir := range []string{
		pm2Home,
		filepath.Join(pm2Home, "pids"),
		filepath.Join(pm2Home, "logs"),
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

func TestMergeStringMaps(t *testing.T) {
	merged := MergeStringMaps(
		map[string]string{
			"KEEP":     "base",
			"OVERRIDE": "base",
		},
		map[string]string{
			"OVERRIDE": "override",
			"ADD":      "override",
		},
	)

	if merged["KEEP"] != "base" {
		t.Fatalf("expected KEEP from base, got %q", merged["KEEP"])
	}
	if merged["OVERRIDE"] != "override" {
		t.Fatalf("expected OVERRIDE from override, got %q", merged["OVERRIDE"])
	}
	if merged["ADD"] != "override" {
		t.Fatalf("expected ADD from override, got %q", merged["ADD"])
	}
}
