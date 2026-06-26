package server

import (
	"context"
	"os"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/aptible/supercronic/cronexpr"
	processrunner "github.com/dunstorm/pm2-go/internal/process"
	"github.com/dunstorm/pm2-go/internal/utils"
	pb "github.com/dunstorm/pm2-go/proto"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

const defaultReloadKillTimeout = 1600 * time.Millisecond

func reloadSignal(name string) (os.Signal, error) {
	normalized := strings.ToUpper(strings.TrimSpace(name))
	if normalized == "" {
		return syscall.SIGTERM, nil
	}
	if value, err := strconv.Atoi(normalized); err == nil {
		return syscall.Signal(value), nil
	}
	if !strings.HasPrefix(normalized, "SIG") {
		normalized = "SIG" + normalized
	}

	switch normalized {
	case "SIGHUP":
		return syscall.SIGHUP, nil
	case "SIGINT":
		return syscall.SIGINT, nil
	case "SIGQUIT":
		return syscall.SIGQUIT, nil
	case "SIGTERM":
		return syscall.SIGTERM, nil
	case "SIGUSR1":
		return syscall.SIGUSR1, nil
	case "SIGUSR2":
		return syscall.SIGUSR2, nil
	default:
		return nil, status.Errorf(400, "unsupported reload signal: %s", name)
	}
}

func waitForProcessExit(pid int32, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if _, running := utils.IsProcessRunning(pid); !running {
			return true
		}
		time.Sleep(25 * time.Millisecond)
	}
	_, running := utils.IsProcessRunning(pid)
	return !running
}

func stopProcessForRestart(found *os.Process, pid int32, in *pb.RestartProcessRequest) error {
	if !in.Graceful {
		return found.Kill()
	}

	signal, err := reloadSignal(in.Signal)
	if err != nil {
		return err
	}
	if err := found.Signal(signal); err != nil {
		return err
	}

	timeout := defaultReloadKillTimeout
	if in.KillTimeoutMs > 0 {
		timeout = time.Duration(in.KillTimeoutMs) * time.Millisecond
	}
	if waitForProcessExit(pid, timeout) {
		return nil
	}
	return found.Kill()
}

func (api *Handler) RestartProcess(ctx context.Context, in *pb.RestartProcessRequest) (*pb.Process, error) {
	api.mu.Lock()
	defer api.mu.Unlock()

	currentProcess := api.databaseById[in.Id]
	if currentProcess == nil {
		return nil, status.Error(400, "failed to find process")
	}

	var nextStartAt *timestamppb.Timestamp
	if in.CronRestart != "" {
		expr, err := cronexpr.Parse(in.CronRestart)
		if err != nil {
			return nil, status.Errorf(400, "invalid cron expression: %v", err)
		}
		nextStartAt = timestamppb.New(expr.Next(time.Now()))
	}

	if found := api.processes[in.Id]; found != nil {
		pid := currentProcess.Pid
		if err := stopProcessForRestart(found, pid, in); err != nil {
			return nil, err
		}
		currentProcess.SetStatus("stopped")
		currentProcess.ResetCPUMemory()
		currentProcess.StopSignal = true
		currentProcess.ResetPid()
		updateProcessMap(api, in.Id, nil)
	}

	newProcess, err := processrunner.SpawnNewProcess(processrunner.SpawnParams{
		Name:           in.Name,
		Args:           in.Args,
		ExecutablePath: in.ExecutablePath,
		AutoRestart:    in.AutoRestart,
		Cwd:            in.Cwd,
		Logger:         api.logger,
		CronRestart:    in.CronRestart,
		Env:            in.Env,
	})
	if err != nil {
		currentProcess.AutoRestart = false
		currentProcess.SetStopSignal(true)
		currentProcess.SetStatus("stopped")
		currentProcess.ResetPid()
		updateProcessMap(api, currentProcess.Id, nil)
		return nil, err
	}

	restarts := int32(0)
	if currentProcess.ProcStatus != nil {
		restarts = currentProcess.ProcStatus.Restarts + 1
	}

	newProcess.Id = currentProcess.Id
	newProcess.LogFileCount = currentProcess.LogFileCount
	newProcess.ProcStatus = &pb.ProcStatus{
		Status:    "online",
		StartedAt: timestamppb.New(time.Now()),
		Uptime:    durationpb.New(0),
		Restarts:  restarts,
		Cpu:       "0.0%",
		Memory:    "0.0MB",
		ParentPid: int32(os.Getpid()),
	}
	newProcess.NextStartAt = nextStartAt

	osProcess, running := utils.GetProcess(newProcess.Pid)
	if !running {
		return nil, status.Error(400, "failed to restart process")
	}

	delete(api.databaseByName, currentProcess.Name)
	api.databaseById[newProcess.Id] = newProcess
	api.databaseByName[newProcess.Name] = newProcess
	api.processes[newProcess.Id] = osProcess
	api.persistStateLocked()

	go osProcess.Wait()

	return newProcess, nil
}
