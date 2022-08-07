package server

import (
	"context"
	"os"
	"time"

	pb "github.com/dunstorm/pm2-go/proto"
	"github.com/dunstorm/pm2-go/shared"
	"github.com/dunstorm/pm2-go/utils"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// spawn process
func (api *Handler) SpawnProcess(ctx context.Context, in *pb.SpawnProcessRequest) (*pb.SpawnProcessResponse, error) {
	api.mu.Lock()
	defer api.mu.Unlock()

	// shared: spawn new process
	process, err := shared.SpawnNewProcess(shared.SpawnParams{
		Name:           in.Name,
		Args:           in.Args,
		ExecutablePath: in.ExecutablePath,
		AutoRestart:    in.AutoRestart,
		Logger:         api.logger,
		Cwd:            in.Cwd,
		Scripts:        in.Scripts,
	})

	if err != nil {
		return &pb.SpawnProcessResponse{
			Success: false,
		}, nil
	}

	api.logger.Info().Msgf("spawned process: %d", process.Pid)

	process.Id = api.nextId
	process.ProcStatus = &pb.ProcStatus{
		Status:    "online",
		StartedAt: timestamppb.New(time.Now()),
		Uptime:    durationpb.New(0),
		Cpu:       "0.0%",
		Memory:    "0.0MB",
		ParentPid: int32(os.Getpid()),
	}

	osProcess, running := utils.GetProcess(process.Pid)
	if !running {
		return nil, status.Error(400, "failed to spawn process")
	}

	api.databaseById[api.nextId] = process
	api.processes[process.Id] = osProcess
	api.nextId++

	go osProcess.Wait()

	return &pb.SpawnProcessResponse{
		Success: true,
	}, nil
}
