package server

import (
	"context"
	"fmt"
	"net"
	"testing"
	"time"

	"github.com/dunstorm/pm2-go/internal/utils"
	pb "github.com/dunstorm/pm2-go/proto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func newStateTestProcessManager(t *testing.T) pb.ProcessManagerClient {
	t.Helper()

	grpcServer, lis, err := NewServer(0)
	if err != nil {
		t.Fatalf("start grpc test server: %v", err)
	}

	port := lis.Addr().(*net.TCPAddr).Port
	go func() {
		_ = grpcServer.Serve(lis)
	}()

	for i := 0; i < 100; i++ {
		if utils.IsPortOpen(port) {
			break
		}
		if i == 99 {
			grpcServer.Stop()
			t.Fatalf("grpc test server did not open port %d", port)
		}
		time.Sleep(10 * time.Millisecond)
	}

	t.Cleanup(grpcServer.Stop)

	conn, err := grpc.NewClient(fmt.Sprintf("127.0.0.1:%d", port), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("dial grpc server: %v", err)
	}
	t.Cleanup(func() {
		_ = conn.Close()
	})

	manager := pb.NewProcessManagerClient(conn)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		processes, err := manager.ListProcess(ctx, &pb.ListProcessRequest{})
		if err != nil {
			return
		}
		for _, process := range processes.Processes {
			if process.Pid != 0 {
				_, _ = manager.StopProcess(ctx, &pb.StopProcessRequest{Id: process.Id})
			}
			_, _ = manager.DeleteProcess(ctx, &pb.DeleteProcessRequest{Id: process.Id})
		}
	})

	return manager
}

func TestPersistStateOnSpawnStopDelete(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	manager := newStateTestProcessManager(t)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	resp, err := manager.SpawnProcess(ctx, &pb.SpawnProcessRequest{
		Name:           "persisted",
		ExecutablePath: "python3",
		Args:           []string{"-c", "import time; time.sleep(10)"},
		AutoRestart:    true,
		Env:            map[string]string{"PM2_GO_TEST_ENV": "persisted"},
	})
	if err != nil {
		t.Fatalf("spawn process: %v", err)
	}
	if !resp.Success {
		t.Fatal("expected spawn to succeed")
	}

	process, err := manager.FindProcess(ctx, &pb.FindProcessRequest{Name: "persisted"})
	if err != nil {
		t.Fatalf("find process: %v", err)
	}

	state := readPersistedState(t)
	if len(state) != 1 {
		t.Fatalf("expected 1 persisted process, got %d", len(state))
	}
	if state[0].Name != "persisted" {
		t.Fatalf("expected persisted process name, got %q", state[0].Name)
	}
	if state[0].ExecutablePath == "" {
		t.Fatal("expected persisted executable path")
	}
	if !state[0].AutoRestart {
		t.Fatal("expected autorestart to be persisted")
	}
	if state[0].Env["PM2_GO_TEST_ENV"] != "persisted" {
		t.Fatalf("expected persisted env, got %#v", state[0].Env)
	}

	stopResp, err := manager.StopProcess(ctx, &pb.StopProcessRequest{Id: process.Id})
	if err != nil {
		t.Fatalf("stop process: %v", err)
	}
	if !stopResp.Success {
		t.Fatal("expected stop to succeed")
	}

	state = readPersistedState(t)
	if len(state) != 0 {
		t.Fatalf("expected stopped process to be removed from state, got %d entries", len(state))
	}

	deleteResp, err := manager.DeleteProcess(ctx, &pb.DeleteProcessRequest{Id: process.Id})
	if err != nil {
		t.Fatalf("delete process: %v", err)
	}
	if !deleteResp.Success {
		t.Fatal("expected delete to succeed")
	}
}

func TestRestoreStateSpawnsPersistedProcesses(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	err := utils.SaveObject(utils.GetStateFilePath(), []persistedProcess{
		{
			Name:           "restored",
			ExecutablePath: "python3",
			Args:           []string{"-c", "import time; time.sleep(10)"},
			AutoRestart:    true,
			Env:            map[string]string{"PM2_GO_RESTORE_ENV": "restored"},
		},
	})
	if err != nil {
		t.Fatalf("save state: %v", err)
	}

	manager := newStateTestProcessManager(t)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	process, err := manager.FindProcess(ctx, &pb.FindProcessRequest{Name: "restored"})
	if err != nil {
		t.Fatalf("find restored process: %v", err)
	}
	if process.Pid == 0 {
		t.Fatal("expected restored process pid")
	}
	if !process.AutoRestart {
		t.Fatal("expected restored process autorestart flag")
	}
	if process.Env["PM2_GO_RESTORE_ENV"] != "restored" {
		t.Fatalf("expected restored process env, got %#v", process.Env)
	}
	if _, running := utils.IsProcessRunning(process.Pid); !running {
		t.Fatal("expected restored process to be running")
	}
}

func TestShouldPersistUnhealthyRunningProcess(t *testing.T) {
	process := &pb.Process{
		Pid: 123,
		ProcStatus: &pb.ProcStatus{
			Status: "unhealthy",
		},
	}

	if !shouldPersistProcess(process) {
		t.Fatal("expected unhealthy live process to be persisted")
	}
}

func readPersistedState(t *testing.T) []persistedProcess {
	t.Helper()

	var state []persistedProcess
	if err := utils.LoadObject(utils.GetStateFilePath(), &state); err != nil {
		t.Fatalf("load state: %v", err)
	}
	return state
}
