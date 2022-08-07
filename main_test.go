package main

import (
	"testing"

	"github.com/dunstorm/pm2-go/app"
	"github.com/dunstorm/pm2-go/shared"
	"github.com/dunstorm/pm2-go/utils"
	"github.com/rs/zerolog"
)

func isServerRunning() bool {
	// check if 9001 is open
	return utils.IsPortOpen(9001)
}

func isProcessAdded(master *app.App, name string) bool {
	process := master.FindProcess(name)
	return process != nil
}

func isProcessRunning(master *app.App, name string) bool {
	process := master.FindProcess(name)
	return process.Pid != 0
}

func TestSpawn(t *testing.T) {
	zerolog.SetGlobalLevel(zerolog.Disabled)

	process, err := shared.SpawnNewProcess(shared.SpawnParams{
		ExecutablePath: "python3",
		Args:           []string{"examples/test.py"},
	})
	if err != nil {
		t.Error(err)
		return
	}

	if process == nil {
		t.Error("process is nil")
		return
	}

	processFound, running := utils.IsProcessRunning(process.Pid)
	if !running {
		t.Error("process is not running")
	}
	processFound.Kill()
}

func TestStartEcosystem(t *testing.T) {
	master := app.New()
	err := master.StartFile("examples/ecosystem.json")
	if err != nil {
		t.Error(err)
	}
	if !isProcessAdded(master, "python-test") {
		t.Error("python-test is not running")
	}
	if !isProcessAdded(master, "celery-worker") {
		t.Error("celery-worker is not running")
	}
	running := isServerRunning()
	if !running {
		t.Error()
	}
}

func TestStopEcosystem(t *testing.T) {
	master := app.New()
	err := master.StopFile("examples/ecosystem.json")
	if err != nil {
		t.Error(err)
	}
	if isProcessRunning(master, "python-test") {
		t.Error("python-test is running")
	}
	if isProcessRunning(master, "celery-worker") {
		t.Error("celery-worker is running")
	}
	running := isServerRunning()
	if !running {
		t.Error()
	}
}

func TestDeleteEcosystem(t *testing.T) {
	master := app.New()
	err := master.DeleteFile("examples/ecosystem.json")
	if err != nil {
		t.Error(err)
	}
	if isProcessAdded(master, "python-test") {
		t.Error("python-test exists")
	}
	if isProcessAdded(master, "celery-worker") {
		t.Error("celery-worker exists")
	}
	running := isServerRunning()
	if !running {
		t.Error()
	}
}
