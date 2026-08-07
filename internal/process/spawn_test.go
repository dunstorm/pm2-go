package process

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/dunstorm/pm2-go/internal/logstore"
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

func TestWaitForSpawnedProcessTracksCmdWait(t *testing.T) {
	zerolog.SetGlobalLevel(zerolog.Disabled)
	t.Setenv("HOME", t.TempDir())

	spawnedProcess, err := SpawnNewProcess(SpawnParams{
		Name:           "wait-tracked",
		ExecutablePath: "python3",
		Args:           []string{"-c", "import time; time.sleep(30)"},
	})
	if err != nil {
		t.Fatal(err)
	}

	if exited, tracked := WaitForSpawnedProcess(spawnedProcess.Pid, 10*time.Millisecond); !tracked || exited {
		t.Fatalf("expected running process to be tracked without exiting, tracked=%v exited=%v", tracked, exited)
	}

	processFound, running := utils.IsProcessRunning(spawnedProcess.Pid)
	if !running {
		t.Fatal("process is not running")
	}
	if err := utils.KillProcessGroup(processFound); err != nil {
		t.Fatalf("kill process group: %v", err)
	}
	if exited, tracked := WaitForSpawnedProcess(spawnedProcess.Pid, 2*time.Second); !tracked || !exited {
		t.Fatalf("expected tracked process exit, tracked=%v exited=%v", tracked, exited)
	}
}

func TestDeleteSpawnedProcessWaitKeepsReusedPIDRegistration(t *testing.T) {
	const pid int32 = -4242
	oldDone := make(chan struct{})
	newDone := make(chan struct{})
	spawnedProcessWaits.Store(pid, oldDone)
	spawnedProcessWaits.Store(pid, newDone)
	t.Cleanup(func() {
		spawnedProcessWaits.Delete(pid)
	})

	deleteSpawnedProcessWait(pid, oldDone)

	value, ok := spawnedProcessWaits.Load(pid)
	if !ok {
		t.Fatal("expected reused PID registration to remain")
	}
	if value != newDone {
		t.Fatal("expected stale wait channel not to delete reused registration")
	}

	deleteSpawnedProcessWait(pid, newDone)
	if _, ok := spawnedProcessWaits.Load(pid); ok {
		t.Fatal("expected matching wait channel to be deleted")
	}
}

func TestWaitForSpawnedProcessClaimsClosedRegistration(t *testing.T) {
	const pid int32 = -4343
	done := make(chan struct{})
	spawnedProcessWaits.Store(pid, done)
	t.Cleanup(func() {
		spawnedProcessWaits.Delete(pid)
	})
	close(done)

	exited, tracked := WaitForSpawnedProcess(pid, 0)
	if !tracked || !exited {
		t.Fatalf("expected closed registration to be tracked and exited, tracked=%v exited=%v", tracked, exited)
	}
	if _, ok := spawnedProcessWaits.Load(pid); ok {
		t.Fatal("expected closed registration to be deleted after waiter claims it")
	}
}

func TestSpawnNewProcessReturnsPidFileError(t *testing.T) {
	zerolog.SetGlobalLevel(zerolog.Disabled)
	home := t.TempDir()
	t.Setenv("HOME", home)

	if err := os.MkdirAll(filepath.Join(home, ".pm2-go", "pids", "pid-file-conflict.pid"), 0700); err != nil {
		t.Fatalf("create conflicting pid path: %v", err)
	}

	process, err := SpawnNewProcess(SpawnParams{
		Name:           "pid-file-conflict",
		ExecutablePath: "python3",
		Args:           []string{"-c", "import time; time.sleep(10)"},
	})
	if err == nil {
		if process != nil {
			if found, running := utils.IsProcessRunning(process.Pid); running {
				_ = utils.KillProcessGroup(found)
			}
		}
		t.Fatal("expected pid file write error")
	}
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

	expectedCombinedLogFile := logstore.CombinedPath(expectedLogFile)
	if params.CombinedLogFilePath != expectedCombinedLogFile {
		t.Fatalf("expected combined log file %q, got %q", expectedCombinedLogFile, params.CombinedLogFilePath)
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

func TestCommandEnvironmentUsesExplicitEnvironment(t *testing.T) {
	environ := commandEnvironment(
		[]string{"PATH=/bin", "DAEMON_ONLY=leak"},
		map[string]string{
			"APP_ENV": "exact",
		},
		false,
	)
	env := utils.EnvironmentMap(environ)

	if env["APP_ENV"] != "exact" {
		t.Fatalf("expected APP_ENV exact, got %q", env["APP_ENV"])
	}
	if _, exists := env["DAEMON_ONLY"]; exists {
		t.Fatalf("expected daemon environment to be isolated, got %#v", env)
	}
	if _, exists := env["PATH"]; exists {
		t.Fatalf("expected base environment to be replaced, got %#v", env)
	}
}
