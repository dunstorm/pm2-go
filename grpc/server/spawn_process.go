package server

import (
	"context"
	"os"
	"time"

	"github.com/aptible/supercronic/cronexpr"
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

	api.logger.Info().Msgf("Attempting to spawn process with name: %s, executable: %s", in.Name, in.ExecutablePath)

	// shared: spawn new process
	process, err := shared.SpawnNewProcess(shared.SpawnParams{
		Name:           in.Name,
		Args:           in.Args,
		ExecutablePath: in.ExecutablePath,
		AutoRestart:    in.AutoRestart,
		Logger:         api.logger,
		Cwd:            in.Cwd,
		CronRestart:    in.CronRestart,
	})

	if err != nil {
		api.logger.Error().Err(err).Msg("Failed to spawn new process")
		return &pb.SpawnProcessResponse{
			Success: false,
		}, nil
	}

	api.logger.Info().Msgf("Successfully spawned process with PID: %d", process.Pid)

	process.Id = api.nextId
	process.ProcStatus = &pb.ProcStatus{
		Status:    "online",
		StartedAt: timestamppb.New(time.Now()),
		Uptime:    durationpb.New(0),
		Cpu:       "0.0%",
		Memory:    "0.0MB",
		ParentPid: int32(os.Getpid()),
	}

	if in.CronRestart != "" {
		api.logger.Info().Msgf("Parsing cron expression: %s", in.CronRestart)
		expr, err := cronexpr.Parse(in.CronRestart)
		if err != nil {
			api.logger.Error().Err(err).Msg("Invalid cron expression")
			return nil, status.Errorf(400, "Invalid cron expression: %v", err)
		}
		process.NextStartAt = timestamppb.New(expr.Next(time.Now()))
		api.logger.Info().Msgf("Next scheduled restart at: %v", process.NextStartAt.AsTime())
	}

	osProcess, running := utils.GetProcess(process.Pid)
	if !running {
		api.logger.Error().Int32("pid", process.Pid).Msg("Failed to get OS process - process not running")
		return nil, status.Error(400, "failed to spawn process")
	}

	api.logger.Info().Msgf("Adding process to database with ID: %d", api.nextId)
	api.databaseById[api.nextId] = process
	api.databaseByName[process.Name] = process
	api.processes[process.Id] = osProcess
	api.nextId++

	go osProcess.Wait()

	return &pb.SpawnProcessResponse{
		Success: true,
	}, nil
}
