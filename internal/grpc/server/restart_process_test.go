package server

import (
	"context"
	"io"
	"os"
	"os/exec"
	"sync"
	"testing"
	"time"

	pb "github.com/dunstorm/pm2-go/proto"
	"github.com/rs/zerolog"
)

func TestRestartProcessReleasesLockBeforeWaitingForExit(t *testing.T) {
	command := exec.Command("sleep", "10")
	if err := command.Start(); err != nil {
		t.Fatalf("start test process: %v", err)
	}
	t.Cleanup(func() {
		_ = command.Process.Kill()
		_, _ = command.Process.Wait()
	})

	logger := zerolog.New(io.Discard)
	process := &pb.Process{
		Id:             1,
		Name:           "slow-restart",
		Pid:            int32(command.Process.Pid),
		ExecutablePath: "python3",
		ProcStatus: &pb.ProcStatus{
			Status: "online",
			Cpu:    "2.0%",
			Memory: "8.0MB",
		},
	}
	handler := &Handler{
		logger:           &logger,
		databaseById:     map[int32]*pb.Process{process.Id: process},
		databaseByName:   map[string]*pb.Process{process.Name: process},
		processes:        map[int32]*os.Process{process.Id: command.Process},
		metricsUpdatedAt: map[int32]time.Time{process.Id: time.Now()},
	}

	previousWaitForRestartProcessExit := waitForRestartProcessExit
	waitStarted := make(chan struct{})
	releaseWait := make(chan struct{})
	var waitStartedOnce sync.Once
	var releaseWaitOnce sync.Once
	waitForRestartProcessExit = func(pid int32, timeout time.Duration) bool {
		waitStartedOnce.Do(func() {
			close(waitStarted)
		})
		<-releaseWait
		return true
	}
	t.Cleanup(func() {
		releaseWaitOnce.Do(func() {
			close(releaseWait)
		})
		waitForRestartProcessExit = previousWaitForRestartProcessExit
	})

	restartErr := make(chan error, 1)
	go func() {
		_, err := handler.RestartProcess(context.Background(), &pb.RestartProcessRequest{
			Id:             process.Id,
			Name:           process.Name,
			ExecutablePath: "definitely-not-a-real-command",
		})
		restartErr <- err
	}()

	select {
	case <-waitStarted:
	case <-time.After(time.Second):
		t.Fatal("expected restart process to enter exit wait")
	}

	listDone := make(chan error, 1)
	go func() {
		_, err := handler.ListProcess(context.Background(), &pb.ListProcessRequest{})
		listDone <- err
	}()

	select {
	case err := <-listDone:
		if err != nil {
			t.Fatalf("list process while restart waits: %v", err)
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatal("expected list process not to block while restart waits for child exit")
	}

	releaseWaitOnce.Do(func() {
		close(releaseWait)
	})
	select {
	case err := <-restartErr:
		if err == nil {
			t.Fatal("expected restart to fail after wait because executable is missing")
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for restart process to return")
	}
}

func TestRestartProcessAbortsAfterConcurrentStopRequest(t *testing.T) {
	command := exec.Command("sleep", "10")
	if err := command.Start(); err != nil {
		t.Fatalf("start test process: %v", err)
	}
	t.Cleanup(func() {
		_ = command.Process.Kill()
		_, _ = command.Process.Wait()
	})

	logger := zerolog.New(io.Discard)
	process := &pb.Process{
		Id:             1,
		Name:           "restart-then-stop",
		Pid:            int32(command.Process.Pid),
		ExecutablePath: "python3",
		ProcStatus: &pb.ProcStatus{
			Status: "online",
			Cpu:    "2.0%",
			Memory: "8.0MB",
		},
	}
	handler := &Handler{
		logger:           &logger,
		databaseById:     map[int32]*pb.Process{process.Id: process},
		databaseByName:   map[string]*pb.Process{process.Name: process},
		processes:        map[int32]*os.Process{process.Id: command.Process},
		metricsUpdatedAt: map[int32]time.Time{process.Id: time.Now()},
	}

	previousWaitForRestartProcessExit := waitForRestartProcessExit
	waitStarted := make(chan struct{})
	releaseWait := make(chan struct{})
	var waitStartedOnce sync.Once
	var releaseWaitOnce sync.Once
	waitForRestartProcessExit = func(pid int32, timeout time.Duration) bool {
		waitStartedOnce.Do(func() {
			close(waitStarted)
		})
		<-releaseWait
		return true
	}
	t.Cleanup(func() {
		releaseWaitOnce.Do(func() {
			close(releaseWait)
		})
		waitForRestartProcessExit = previousWaitForRestartProcessExit
	})

	restartErr := make(chan error, 1)
	go func() {
		_, err := handler.RestartProcess(context.Background(), &pb.RestartProcessRequest{
			Id:             process.Id,
			Name:           process.Name,
			ExecutablePath: "python3",
			Args:           []string{"-c", "import time; time.sleep(30)"},
		})
		restartErr <- err
	}()

	select {
	case <-waitStarted:
	case <-time.After(time.Second):
		t.Fatal("expected restart process to enter exit wait")
	}

	stopResponse, err := handler.StopProcess(context.Background(), &pb.StopProcessRequest{Id: process.Id})
	if err != nil {
		t.Fatalf("stop process during restart wait: %v", err)
	}
	if stopResponse.GetSuccess() {
		t.Fatal("expected stop to report no tracked process while restart owns the old wait")
	}

	releaseWaitOnce.Do(func() {
		close(releaseWait)
	})
	select {
	case err := <-restartErr:
		if err == nil {
			t.Fatal("expected restart to abort after concurrent stop")
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for restart process to return")
	}

	if process.Pid != 0 {
		t.Fatalf("expected process pid to remain stopped, got %d", process.Pid)
	}
	if !process.GetStopSignal() {
		t.Fatal("expected concurrent stop intent to remain set")
	}
	if handler.processes[process.Id] != nil {
		t.Fatal("expected no replacement process to be tracked")
	}
}
