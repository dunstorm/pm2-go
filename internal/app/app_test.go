package app

import (
	"testing"

	"github.com/dunstorm/pm2-go/internal/testutil"
)

func TestNew(t *testing.T) {
	app := NewWithPort(0)
	if app == nil {
		t.Fatal("Failed to create new App instance")
	}
	if app.GetLogger() == nil {
		t.Error("Logger is not initialized")
	}
}

func TestProcessLifecycle(t *testing.T) {
	app := NewWithPort(testutil.StartGRPCServer(t))

	if !app.SpawnProcess(SpawnParams{
		Name:           "test-process",
		ExecutablePath: "python3",
		Args:           []string{"-c", "import time; time.sleep(10)"},
	}) {
		t.Fatal("Failed to spawn process")
	}

	processes := app.ListProcess()
	if len(processes) == 0 {
		t.Error("Process list is empty after adding a process")
	}

	foundProcess := app.FindProcess("test-process")
	if foundProcess == nil {
		t.Error("Failed to find added process")
	}
	if foundProcess.Name != "test-process" {
		t.Errorf("Expected process name 'test-process', got '%s'", foundProcess.Name)
	}

	if !app.StopProcess(foundProcess.Id) {
		t.Error("Failed to stop process")
	}

	if !app.DeleteProcess(foundProcess) {
		t.Error("Failed to delete process")
	}

	deletedProcess := app.FindProcess("test-process")
	if deletedProcess != nil {
		t.Error("Process still exists after deletion")
	}
}

func TestProcessRestart(t *testing.T) {
	app := NewWithPort(testutil.StartGRPCServer(t))

	if !app.SpawnProcess(SpawnParams{
		Name:           "restart-test",
		ExecutablePath: "python3",
		Args:           []string{"-c", "import time; time.sleep(10)"},
	}) {
		t.Fatal("Failed to spawn process")
	}

	foundProcess := app.FindProcess("restart-test")
	if foundProcess == nil {
		t.Fatal("Failed to find process")
	}

	restartedProcess := app.RestartProcess(foundProcess)
	if restartedProcess == nil {
		t.Error("Failed to restart process")
	}
	if restartedProcess.Pid == 0 {
		t.Error("Restarted process has invalid PID")
	}

	app.StopProcess(restartedProcess.Id)
	app.DeleteProcess(restartedProcess)
}
