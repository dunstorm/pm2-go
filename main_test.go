package main

import (
	"testing"

	"github.com/dunstorm/pm2-go/app"
	"github.com/dunstorm/pm2-go/grpc/client"
	pb "github.com/dunstorm/pm2-go/proto"
	"github.com/dunstorm/pm2-go/shared"
	"github.com/dunstorm/pm2-go/utils"
	"github.com/rs/zerolog"
)

func isServerRunning() bool {
	// check if 50051 is open
	return utils.IsPortOpen(50051)
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
		t.Fatal("process is nil")
	}

	processFound, running := utils.IsProcessRunning(process.Pid)
	if !running {
		t.Fatal("process is not running")
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
	pythonTestPid := master.FindProcess("python-test").Pid
	celeryWorkerPid := master.FindProcess("celery-worker").Pid
	err := master.StopFile("examples/ecosystem.json")
	if err != nil {
		t.Error(err)
	}
	if isProcessRunning(master, "python-test") {
		t.Errorf("python-test %d is still running", pythonTestPid)
	}
	if isProcessRunning(master, "celery-worker") {
		t.Errorf("celery-worker %d is still running", celeryWorkerPid)
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

func TestCronRestart(t *testing.T) {
	c, err := client.New(50051)
	if err != nil {
		t.Fatal(err)
	}
	response := c.SpawnProcess(&pb.SpawnProcessRequest{
		ExecutablePath: "python3",
		Args:           []string{"examples/test.py"},
		Name:           "python-test",
		CronRestart:    "* * * * *",
	})
	if !response.Success {
		t.Fatal("failed to spawn process")
	}
	process := c.FindProcess("python-test")
	if process == nil {
		t.Fatal("process not found")
	}
	if process.NextStartAt == nil {
		t.Error("Cron expression failed, NextStartAt is nil")
	}
	c.StopProcess(process.Id)
	c.DeleteProcess(process.Id)
}

func TestNoCronRestart(t *testing.T) {
	c, err := client.New(50051)
	if err != nil {
		t.Fatal(err)
	}
	response := c.SpawnProcess(&pb.SpawnProcessRequest{
		ExecutablePath: "python3",
		Args:           []string{"examples/test.py"},
		Name:           "python-test",
	})
	if !response.Success {
		t.Fatal("failed to spawn process")
	}
	process := c.FindProcess("python-test")
	if process == nil {
		t.Fatal("process not found")
	}
	if process.NextStartAt != nil {
		t.Error("NextStartAt is not nil")
	}
	c.StopProcess(process.Id)
	c.DeleteProcess(process.Id)
}

func TestFailedCronRestart(t *testing.T) {
	c, err := client.New(50051)
	if err != nil {
		t.Fatal(err)
	}
	response := c.SpawnProcess(&pb.SpawnProcessRequest{
		ExecutablePath: "python3",
		Args:           []string{"examples/test.py"},
		Name:           "python-test",
		CronRestart:    "* * v * * *",
	})
	if response != nil {
		t.Error("failed cron expression went through")
	}
}
