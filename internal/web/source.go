package web

import (
	"context"
	"fmt"
	"strconv"
	"time"

	pb "github.com/dunstorm/pm2-go/proto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type ProcessSource interface {
	ListProcesses(ctx context.Context) ([]*pb.Process, error)
	FindProcess(ctx context.Context, id int32) (*pb.Process, error)
	StopProcess(ctx context.Context, id int32) (bool, error)
	RestartProcess(ctx context.Context, process *pb.Process, graceful bool) (*pb.Process, error)
	DeleteProcess(ctx context.Context, id int32) (bool, error)
}

type GRPCProcessSource struct {
	Port    int
	Timeout time.Duration
}

func NewGRPCProcessSource(port int) GRPCProcessSource {
	if port == 0 {
		port = DefaultDaemonPort
	}
	return GRPCProcessSource{
		Port:    port,
		Timeout: 2 * time.Second,
	}
}

func (source GRPCProcessSource) ListProcesses(ctx context.Context) ([]*pb.Process, error) {
	ctx, manager, closeConn, err := source.manager(ctx)
	if err != nil {
		return nil, err
	}
	defer closeConn()

	resp, err := manager.ListProcess(ctx, &pb.ListProcessRequest{})
	if err != nil {
		return nil, err
	}
	return resp.GetProcesses(), nil
}

func (source GRPCProcessSource) FindProcess(ctx context.Context, id int32) (*pb.Process, error) {
	ctx, manager, closeConn, err := source.manager(ctx)
	if err != nil {
		return nil, err
	}
	defer closeConn()

	return manager.FindProcess(ctx, &pb.FindProcessRequest{Name: strconv.Itoa(int(id))})
}

func (source GRPCProcessSource) StopProcess(ctx context.Context, id int32) (bool, error) {
	ctx, manager, closeConn, err := source.manager(ctx)
	if err != nil {
		return false, err
	}
	defer closeConn()

	resp, err := manager.StopProcess(ctx, &pb.StopProcessRequest{Id: id})
	if err != nil {
		return false, err
	}
	return resp.GetSuccess(), nil
}

func (source GRPCProcessSource) RestartProcess(ctx context.Context, process *pb.Process, graceful bool) (*pb.Process, error) {
	ctx, manager, closeConn, err := source.manager(ctx)
	if err != nil {
		return nil, err
	}
	defer closeConn()

	return manager.RestartProcess(ctx, restartRequest(process, graceful))
}

func (source GRPCProcessSource) DeleteProcess(ctx context.Context, id int32) (bool, error) {
	ctx, manager, closeConn, err := source.manager(ctx)
	if err != nil {
		return false, err
	}
	defer closeConn()

	resp, err := manager.DeleteProcess(ctx, &pb.DeleteProcessRequest{Id: id})
	if err != nil {
		return false, err
	}
	return resp.GetSuccess(), nil
}

func (source GRPCProcessSource) manager(ctx context.Context) (context.Context, pb.ProcessManagerClient, func(), error) {
	timeout := source.Timeout
	if timeout <= 0 {
		timeout = 2 * time.Second
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)

	conn, err := grpc.NewClient(
		fmt.Sprintf("127.0.0.1:%d", source.Port),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		cancel()
		return nil, nil, nil, err
	}
	closeConn := func() {
		_ = conn.Close()
		cancel()
	}
	return ctx, pb.NewProcessManagerClient(conn), closeConn, nil
}

func restartRequest(process *pb.Process, graceful bool) *pb.RestartProcessRequest {
	return &pb.RestartProcessRequest{
		Id:                       process.Id,
		Name:                     process.Name,
		Args:                     process.Args,
		ExecutablePath:           process.ExecutablePath,
		AutoRestart:              process.AutoRestart,
		Cwd:                      process.Cwd,
		CronRestart:              process.CronRestart,
		Env:                      process.Env,
		Graceful:                 graceful,
		Signal:                   "SIGTERM",
		KillTimeoutMs:            1600,
		MaxRestarts:              process.MaxRestarts,
		MinUptimeMs:              process.MinUptimeMs,
		RestartDelayMs:           process.RestartDelayMs,
		ExpBackoffRestartDelayMs: process.ExpBackoffRestartDelayMs,
		MaxMemoryRestart:         process.MaxMemoryRestart,
		HealthCheckUrl:           process.HealthCheckUrl,
		HealthCheckIntervalMs:    process.HealthCheckIntervalMs,
		HealthCheckTimeoutMs:     process.HealthCheckTimeoutMs,
		Watch:                    process.Watch,
		WatchPaths:               process.WatchPaths,
		WatchIntervalMs:          process.WatchIntervalMs,
	}
}
