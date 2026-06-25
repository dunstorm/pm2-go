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

func TestIsPythonExecutable(t *testing.T) {
	tests := []struct {
		name           string
		executablePath string
		want           bool
	}{
		{name: "python", executablePath: "python", want: true},
		{name: "python3", executablePath: "python3", want: true},
		{name: "python versioned", executablePath: "/usr/local/bin/python3.12", want: true},
		{name: "python config helper", executablePath: "python3-config", want: false},
		{name: "not python", executablePath: "node", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isPythonExecutable(tt.executablePath); got != tt.want {
				t.Fatalf("expected %v, got %v", tt.want, got)
			}
		})
	}
}

func TestCommandEnvironmentAddsPythonUnbufferedDefault(t *testing.T) {
	environ := commandEnvironment([]string{"PATH=/bin"}, nil, true)
	env := utils.EnvironmentMap(environ)

	if env["PYTHONUNBUFFERED"] != "1" {
		t.Fatalf("expected PYTHONUNBUFFERED=1, got %q", env["PYTHONUNBUFFERED"])
	}
}

func TestCommandEnvironmentAppliesOverrides(t *testing.T) {
	environ := commandEnvironment(
		[]string{"PATH=/bin", "APP_ENV=base", "PYTHONUNBUFFERED=1"},
		map[string]string{
			"APP_ENV":          "override",
			"PYTHONUNBUFFERED": "0",
		},
		true,
	)
	env := utils.EnvironmentMap(environ)

	if env["APP_ENV"] != "override" {
		t.Fatalf("expected APP_ENV override, got %q", env["APP_ENV"])
	}
	if env["PYTHONUNBUFFERED"] != "0" {
		t.Fatalf("expected explicit PYTHONUNBUFFERED override, got %q", env["PYTHONUNBUFFERED"])
	}
}
