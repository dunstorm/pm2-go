package main

import (
	"context"
	"testing"
	"time"

	"github.com/dunstorm/pm2-go/internal/app"
	"github.com/dunstorm/pm2-go/internal/grpc/client"
	processrunner "github.com/dunstorm/pm2-go/internal/process"
	"github.com/dunstorm/pm2-go/internal/testutil"
	"github.com/dunstorm/pm2-go/internal/utils"
	pb "github.com/dunstorm/pm2-go/proto"
	"github.com/rs/zerolog"
)

func isServerRunning(port int) bool {
	return utils.IsPortOpen(port)
}

func isProcessAdded(master *app.App, name string) bool {
	process := master.FindProcess(name)
	return process != nil
}

func isProcessRunning(master *app.App, name string) bool {
	process := master.FindProcess(name)
	return process != nil && process.Pid != 0
}

func TestSpawn(t *testing.T) {
	zerolog.SetGlobalLevel(zerolog.Disabled)

	spawnedProcess, err := processrunner.SpawnNewProcess(processrunner.SpawnParams{
		ExecutablePath: "python3",
		Args:           []string{"examples/test.py"},
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

func TestStartEcosystem(t *testing.T) {
	port := testutil.StartGRPCServer(t)
	master := app.NewWithPort(port)
	t.Cleanup(func() {
		if process := master.FindProcess("python-test"); process != nil {
			master.StopProcess(process.Id)
			master.DeleteProcess(process)
		}
	})

	err := master.StartFile("examples/ecosystem.json")
	if err != nil {
		t.Error(err)
	}
	if !isProcessAdded(master, "python-test") {
		t.Error("python-test is not running")
	}
	running := isServerRunning(port)
	if !running {
		t.Error()
	}
}

func TestStopEcosystem(t *testing.T) {
	port := testutil.StartGRPCServer(t)
	master := app.NewWithPort(port)
	if err := master.StartFile("examples/ecosystem.json"); err != nil {
		t.Fatal(err)
	}

	pythonTestPid := master.FindProcess("python-test").Pid
	err := master.StopFile("examples/ecosystem.json")
	if err != nil {
		t.Error(err)
	}
	if isProcessRunning(master, "python-test") {
		t.Errorf("python-test %d is still running", pythonTestPid)
	}
	if process := master.FindProcess("python-test"); process != nil {
		master.DeleteProcess(process)
	}
	running := isServerRunning(port)
	if !running {
		t.Error()
	}
}

func TestDeleteEcosystem(t *testing.T) {
	port := testutil.StartGRPCServer(t)
	master := app.NewWithPort(port)
	if err := master.StartFile("examples/ecosystem.json"); err != nil {
		t.Fatal(err)
	}

	err := master.DeleteFile("examples/ecosystem.json")
	if err != nil {
		t.Error(err)
	}
	if isProcessAdded(master, "python-test") {
		t.Error("python-test exists")
	}
	running := isServerRunning(port)
	if !running {
		t.Error()
	}
}

func TestCronRestart(t *testing.T) {
	c, err := client.New(testutil.StartGRPCServer(t))
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
	c, err := client.New(testutil.StartGRPCServer(t))
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
	c, err := client.New(testutil.StartGRPCServer(t))
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	conn, manager := c.Dial()
	defer conn.Close()

	response, err := (*manager).SpawnProcess(ctx, &pb.SpawnProcessRequest{
		ExecutablePath: "python3",
		Args:           []string{"examples/test.py"},
		Name:           "python-test",
		CronRestart:    "* * v * * *",
	})
	if err == nil || response != nil {
		t.Error("failed cron expression went through")
	}
}
