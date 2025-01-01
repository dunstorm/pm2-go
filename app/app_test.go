package app

import (
	"testing"

	pb "github.com/dunstorm/pm2-go/proto"
)

func TestNew(t *testing.T) {
	app := New()
	if app == nil {
		t.Fatal("Failed to create new App instance")
	}
	if app.GetLogger() == nil {
		t.Error("Logger is not initialized")
	}
}

func TestProcessLifecycle(t *testing.T) {
	app := New()

	// Test process creation
	process := &pb.Process{
		Name:           "test-process",
		ExecutablePath: "python3",
		Args:           []string{"examples/test.py"},
		AutoRestart:    true,
	}

	id := app.AddProcess(process)
	if id < 0 {
		t.Fatal("Failed to add process")
	}

	// Test process listing
	processes := app.ListProcess()
	if len(processes) == 0 {
		t.Error("Process list is empty after adding a process")
	}

	// Test process finding
	foundProcess := app.FindProcess("test-process")
	if foundProcess == nil {
		t.Error("Failed to find added process")
	}
	if foundProcess.Name != "test-process" {
		t.Errorf("Expected process name 'test-process', got '%s'", foundProcess.Name)
	}

	// Test process stopping
	if !app.StopProcess(foundProcess.Id) {
		t.Error("Failed to stop process")
	}

	// Test process deletion
	if !app.DeleteProcess(foundProcess) {
		t.Error("Failed to delete process")
	}

	// Verify process is deleted
	deletedProcess := app.FindProcess("test-process")
	if deletedProcess != nil {
		t.Error("Process still exists after deletion")
	}
}

func TestProcessRestart(t *testing.T) {
	app := New()

	process := &pb.Process{
		Name:           "restart-test",
		ExecutablePath: "python3",
		Args:           []string{"examples/test.py"},
		AutoRestart:    true,
	}

	id := app.AddProcess(process)
	if id < 0 {
		t.Fatal("Failed to add process")
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

	// Cleanup
	app.DeleteProcess(restartedProcess)
}
