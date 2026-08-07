package server

import (
	"context"
	"io"
	"os"
	"testing"
	"time"

	pb "github.com/dunstorm/pm2-go/proto"
	"github.com/rs/zerolog"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func TestDeleteProcessSuppressesAutorestartBeforeRemoval(t *testing.T) {
	logger := zerolog.New(io.Discard)
	process := &pb.Process{
		Id:             1,
		Name:           "api",
		AutoRestart:    true,
		StopSignal:     false,
		RestartAt:      timestamppb.New(time.Now()),
		NextStartAt:    timestamppb.New(time.Now()),
		ExecutablePath: "python3",
		ProcStatus: &pb.ProcStatus{
			Status: "online",
			Cpu:    "3.1%",
			Memory: "12.0MB",
			Uptime: durationpb.New(time.Second),
		},
	}
	handler := &Handler{
		logger:         &logger,
		databaseById:   map[int32]*pb.Process{process.Id: process},
		databaseByName: map[string]*pb.Process{process.Name: process},
		processes:      make(map[int32]*os.Process),
	}

	response, err := handler.DeleteProcess(context.Background(), &pb.DeleteProcessRequest{Id: process.Id})
	if err != nil {
		t.Fatalf("delete process: %v", err)
	}
	if !response.GetSuccess() {
		t.Fatal("expected delete to succeed")
	}
	if !process.GetStopSignal() {
		t.Fatal("expected delete to set stop signal before removal")
	}
	if process.AutoRestart {
		t.Fatal("expected delete to disable autorestart on stale process snapshots")
	}
	if process.RestartAt != nil || process.NextStartAt != nil {
		t.Fatal("expected delete to clear scheduled restarts")
	}
	if process.GetProcStatus().GetStatus() != "stopped" {
		t.Fatalf("expected deleted process snapshot to be stopped, got %q", process.GetProcStatus().GetStatus())
	}
	if process.GetProcStatus().GetCpu() != "0.0%" || process.GetProcStatus().GetMemory() != "0.0MB" {
		t.Fatalf("expected deleted process metrics to reset, got cpu=%q memory=%q", process.GetProcStatus().GetCpu(), process.GetProcStatus().GetMemory())
	}
	if handler.databaseById[process.Id] != nil {
		t.Fatal("expected process to be removed from id index")
	}
	if handler.databaseByName[process.Name] != nil {
		t.Fatal("expected process to be removed from name index")
	}
}

func TestRestartProcessIgnoresDeletedSnapshot(t *testing.T) {
	logger := zerolog.New(io.Discard)
	process := &pb.Process{
		Id:             1,
		Name:           "deleted-snapshot",
		ExecutablePath: "python3",
		Args:           []string{"-c", "import time; time.sleep(30)"},
		AutoRestart:    true,
		ProcStatus: &pb.ProcStatus{
			Status: "stopped",
		},
	}
	handler := &Handler{
		logger:           &logger,
		databaseById:     make(map[int32]*pb.Process),
		processes:        make(map[int32]*os.Process),
		metricsUpdatedAt: make(map[int32]time.Time),
	}

	restartProcess(handler, process)

	if process.Pid != 0 {
		t.Fatalf("expected stale process snapshot not to spawn, got pid %d", process.Pid)
	}
	if handler.processes[process.Id] != nil {
		t.Fatal("expected stale process snapshot not to be tracked")
	}
}
