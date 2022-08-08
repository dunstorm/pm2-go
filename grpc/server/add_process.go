package server

import (
	"context"
	"time"

	"github.com/aptible/supercronic/cronexpr"
	pb "github.com/dunstorm/pm2-go/proto"
	"github.com/dunstorm/pm2-go/utils"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// create process
func (api *Handler) AddProcess(ctx context.Context, in *pb.AddProcessRequest) (*pb.Process, error) {
	newProcess := &pb.Process{
		Id:             api.nextId,
		Name:           in.Name,
		ExecutablePath: in.ExecutablePath,
		Pid:            int32(in.Pid),
		Args:           in.Args,
		Cwd:            in.Cwd,
		Scripts:        in.Scripts,
		LogFilePath:    in.LogFilePath,
		ErrFilePath:    in.ErrFilePath,
		PidFilePath:    in.PidFilePath,
		AutoRestart:    in.AutoRestart,
		CronRestart:    in.CronRestart,
		ProcStatus: &pb.ProcStatus{
			Status:    "online",
			StartedAt: timestamppb.New(time.Now()),
			Uptime:    durationpb.New(0),
			Cpu:       "0.0%",
			Memory:    "0.0MB",
			ParentPid: 1,
		},
	}

	if in.CronRestart != "" {
		expr, err := cronexpr.Parse(in.CronRestart)
		if err != nil {
			return nil, status.Errorf(400, "invalid cron expression: %v", err)
		}
		newProcess.CronRestart = in.CronRestart
		newProcess.NextStartAt = timestamppb.New(expr.Next(time.Now()))
	}

	process, running := utils.GetProcess(newProcess.Pid)
	if !running {
		return nil, status.Error(400, "failed to add process")
	}

	api.mu.Lock()
	defer api.mu.Unlock()
	api.databaseById[api.nextId] = newProcess
	api.databaseByName[newProcess.Name] = newProcess
	api.processes[newProcess.Id] = process
	api.nextId++

	return newProcess, nil
}
