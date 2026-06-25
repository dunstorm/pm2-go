package process

import (
	"path/filepath"
	"testing"

	"github.com/dunstorm/pm2-go/internal/utils"
	"github.com/rs/zerolog"
)

func TestSpawnNewProcess(t *testing.T) {
	zerolog.SetGlobalLevel(zerolog.Disabled)
	t.Setenv("HOME", t.TempDir())

	spawnedProcess, err := SpawnNewProcess(SpawnParams{
		ExecutablePath: "python3",
		Args:           []string{"../../examples/test.py"},
	})
	if err != nil {
		t.Error(err)
		return
	}

	if spawnedProcess == nil {
		t.Fatal("process is nil")
	}

	processFound, running := utils.IsProcessRunning(spawnedProcess.Pid)
	if !running {
		t.Fatal("process is not running")
	}
	processFound.Kill()
}

func TestFillDefaultsUsesExecutableBaseNameForAbsolutePath(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	params := SpawnParams{
		ExecutablePath: "/opt/miniconda3/envs/test/bin/python",
	}
	if err := params.fillDefaults(); err != nil {
		t.Fatal(err)
	}

	if params.Name != "python" {
		t.Fatalf("expected process name python, got %q", params.Name)
	}

	expectedPidFile := filepath.Join(home, ".pm2-go", "pids", "python.pid")
	if params.PidPilePath != expectedPidFile {
		t.Fatalf("expected pid file %q, got %q", expectedPidFile, params.PidPilePath)
	}

	expectedLogFile := filepath.Join(home, ".pm2-go", "logs", "python-out.log")
	if params.LogFilePath != expectedLogFile {
		t.Fatalf("expected log file %q, got %q", expectedLogFile, params.LogFilePath)
	}
}

func TestFillDefaultsSanitizesFileNames(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	params := SpawnParams{
		Name:           "api/service",
		ExecutablePath: "python3",
	}
	if err := params.fillDefaults(); err != nil {
		t.Fatal(err)
	}

	if params.Name != "api/service" {
		t.Fatalf("expected process name to stay unchanged, got %q", params.Name)
	}

	expectedPidFile := filepath.Join(home, ".pm2-go", "pids", "api-service.pid")
	if params.PidPilePath != expectedPidFile {
		t.Fatalf("expected pid file %q, got %q", expectedPidFile, params.PidPilePath)
	}
}
