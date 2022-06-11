package main

import (
	"os/exec"
	"strings"
	"testing"

	"github.com/dunstorm/pm2-go/app"
	"github.com/rs/zerolog"
)

func isServerRunning() bool {
	out, err := exec.Command("./bin/main", "status").CombinedOutput()
	if err != nil {
		return false
	}
	return strings.Contains(string(out), "PM2 Daemon Running")
}

func isProcessAdded(master *app.App, name string) bool {
	process := master.FindProcess(name)
	return process.ProcStatus != nil
}

func isProcessRunning(master *app.App, name string) bool {
	process := master.FindProcess(name)
	return process.Pid != 0
}

func TestStartEcosystem(t *testing.T) {
	zerolog.SetGlobalLevel(zerolog.Disabled)

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
	zerolog.SetGlobalLevel(zerolog.Disabled)

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
	zerolog.SetGlobalLevel(zerolog.Disabled)

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
