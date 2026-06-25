package server_test

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/dunstorm/pm2-go/internal/testutil"
	"github.com/dunstorm/pm2-go/internal/utils"
	pb "github.com/dunstorm/pm2-go/proto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func newProcessManager(t *testing.T) pb.ProcessManagerClient {
	t.Helper()
	t.Setenv("HOME", t.TempDir())

	port := testutil.StartGRPCServer(t)
	conn, err := grpc.Dial(fmt.Sprintf("127.0.0.1:%d", port), grpc.WithTransportCredentials(insecure.NewCredentials()))
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

func TestStopAndDeleteMissingProcessReturnFalse(t *testing.T) {
	manager := newProcessManager(t)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	stopResp, err := manager.StopProcess(ctx, &pb.StopProcessRequest{Id: 404})
	if err != nil {
		t.Fatalf("stop missing process: %v", err)
	}
	if stopResp.Success {
		t.Fatal("expected stop missing process to return false")
	}

	deleteResp, err := manager.DeleteProcess(ctx, &pb.DeleteProcessRequest{Id: 404})
	if err != nil {
		t.Fatalf("delete missing process: %v", err)
	}
	if deleteResp.Success {
		t.Fatal("expected delete missing process to return false")
	}
}

func TestSpawnMissingExecutableDoesNotRegisterProcess(t *testing.T) {
	manager := newProcessManager(t)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	resp, err := manager.SpawnProcess(ctx, &pb.SpawnProcessRequest{
		Name:           "missing-executable",
		ExecutablePath: "definitely-not-a-real-command",
	})
	if err != nil {
		t.Fatalf("spawn missing executable: %v", err)
	}
	if resp.Success {
		t.Fatal("expected missing executable spawn to fail")
	}

	if _, err := manager.FindProcess(ctx, &pb.FindProcessRequest{Name: "missing-executable"}); err == nil {
		t.Fatal("expected missing executable not to be registered")
	}
}

func TestSpawnInvalidWorkingDirectoryDoesNotRegisterProcess(t *testing.T) {
	manager := newProcessManager(t)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	resp, err := manager.SpawnProcess(ctx, &pb.SpawnProcessRequest{
		Name:           "missing-cwd",
		ExecutablePath: "python3",
		Args:           []string{"-c", "import time; time.sleep(10)"},
		Cwd:            filepath.Join(t.TempDir(), "does-not-exist"),
	})
	if err != nil {
		t.Fatalf("spawn invalid cwd: %v", err)
	}
	if resp.Success {
		t.Fatal("expected invalid cwd spawn to fail")
	}

	if _, err := manager.FindProcess(ctx, &pb.FindProcessRequest{Name: "missing-cwd"}); err == nil {
		t.Fatal("expected invalid cwd process not to be registered")
	}
}

func TestRestartInvalidCronDoesNotStopRunningProcess(t *testing.T) {
	manager := newProcessManager(t)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	resp, err := manager.SpawnProcess(ctx, &pb.SpawnProcessRequest{
		Name:           "restart-invalid-cron",
		ExecutablePath: "python3",
		Args:           []string{"-c", "import time; time.sleep(10)"},
	})
	if err != nil {
		t.Fatalf("spawn process: %v", err)
	}
	if !resp.Success {
		t.Fatal("expected spawn to succeed")
	}

	process, err := manager.FindProcess(ctx, &pb.FindProcessRequest{Name: "restart-invalid-cron"})
	if err != nil {
		t.Fatalf("find process: %v", err)
	}
	originalPid := process.Pid

	_, err = manager.RestartProcess(ctx, &pb.RestartProcessRequest{
		Id:             process.Id,
		Name:           process.Name,
		ExecutablePath: process.ExecutablePath,
		Args:           process.Args,
		Cwd:            process.Cwd,
		CronRestart:    "* * v * * *",
	})
	if err == nil {
		t.Fatal("expected invalid cron restart to fail")
	}

	process, err = manager.FindProcess(ctx, &pb.FindProcessRequest{Name: "restart-invalid-cron"})
	if err != nil {
		t.Fatalf("find process after failed restart: %v", err)
	}
	if process.Pid != originalPid {
		t.Fatalf("expected pid %d after failed restart, got %d", originalPid, process.Pid)
	}
	if _, running := utils.IsProcessRunning(originalPid); !running {
		t.Fatal("expected original process to keep running after failed restart")
	}
}

func TestDeleteRunningProcessRemovesAndStopsProcess(t *testing.T) {
	manager := newProcessManager(t)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	resp, err := manager.SpawnProcess(ctx, &pb.SpawnProcessRequest{
		Name:           "delete-running",
		ExecutablePath: "python3",
		Args:           []string{"-c", "import time; time.sleep(10)"},
	})
	if err != nil {
		t.Fatalf("spawn process: %v", err)
	}
	if !resp.Success {
		t.Fatal("expected spawn to succeed")
	}

	process, err := manager.FindProcess(ctx, &pb.FindProcessRequest{Name: "delete-running"})
	if err != nil {
		t.Fatalf("find process: %v", err)
	}

	deleteResp, err := manager.DeleteProcess(ctx, &pb.DeleteProcessRequest{Id: process.Id})
	if err != nil {
		t.Fatalf("delete running process: %v", err)
	}
	if !deleteResp.Success {
		t.Fatal("expected delete running process to succeed")
	}

	if _, err := manager.FindProcess(ctx, &pb.FindProcessRequest{Name: "delete-running"}); err == nil {
		t.Fatal("expected deleted process not to be registered")
	}
	waitForProcessExit(t, process.Pid)
}

func waitForProcessExit(t *testing.T, pid int32) {
	t.Helper()

	for i := 0; i < 40; i++ {
		if _, running := utils.IsProcessRunning(pid); !running {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}

	t.Fatalf("process %d is still running", pid)
}
