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

func TestStopProcessReleasesLockBeforeWaitingForExit(t *testing.T) {
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
		Id:   1,
		Name: "slow-stop",
		Pid:  int32(command.Process.Pid),
		ProcStatus: &pb.ProcStatus{
			Status: "online",
			Cpu:    "2.0%",
			Memory: "8.0MB",
		},
	}
	handler := &Handler{
		logger:           &logger,
		databaseById:     map[int32]*pb.Process{process.Id: process},
		processes:        map[int32]*os.Process{process.Id: command.Process},
		metricsUpdatedAt: map[int32]time.Time{process.Id: time.Now()},
	}

	previousWaitForStopProcessExit := waitForStopProcessExit
	waitStarted := make(chan struct{})
	releaseWait := make(chan struct{})
	var waitStartedOnce sync.Once
	var releaseWaitOnce sync.Once
	waitForStopProcessExit = func(pid int32, timeout time.Duration) bool {
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
		waitForStopProcessExit = previousWaitForStopProcessExit
	})

	stopDone := make(chan *pb.StopProcessResponse, 1)
	stopErr := make(chan error, 1)
	go func() {
		response, err := handler.StopProcess(context.Background(), &pb.StopProcessRequest{Id: process.Id})
		stopDone <- response
		stopErr <- err
	}()

	select {
	case <-waitStarted:
	case <-time.After(time.Second):
		t.Fatal("expected stop process to enter exit wait")
	}

	listDone := make(chan error, 1)
	go func() {
		_, err := handler.ListProcess(context.Background(), &pb.ListProcessRequest{})
		listDone <- err
	}()

	select {
	case err := <-listDone:
		if err != nil {
			t.Fatalf("list process while stop waits: %v", err)
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatal("expected list process not to block while stop waits for child exit")
	}

	releaseWaitOnce.Do(func() {
		close(releaseWait)
	})
	select {
	case err := <-stopErr:
		if err != nil {
			t.Fatalf("stop process: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for stop process to return")
	}
	response := <-stopDone
	if !response.GetSuccess() {
		t.Fatal("expected stop process to succeed")
	}
}
