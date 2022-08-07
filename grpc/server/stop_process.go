package server

import (
	"context"
	"syscall"
	"time"

	pb "github.com/dunstorm/pm2-go/proto"
	"github.com/dunstorm/pm2-go/utils"
)

// stop process
func (api *Handler) StopProcess(ctx context.Context, in *pb.StopProcessRequest) (*pb.StopProcessResponse, error) {
	api.mu.Lock()
	defer api.mu.Unlock()

	process := api.databaseById[in.Id]
	found := api.processes[in.Id]

	process.SetStatus("stopped")
	process.Pid = 0

	if found == nil {
		api.logger.Info().Msgf("process not found: %d", in.Id)
		return &pb.StopProcessResponse{
			Success: false,
		}, nil
	}

	if process.ProcStatus.ParentPid == 1 {
		// kill process
		err := found.Signal(syscall.SIGTERM)
		if err != nil {
			api.logger.Info().Msgf("failed to stop process: %s", err.Error())
			return &pb.StopProcessResponse{
				Success: false,
			}, nil
		}
		updateProcessMap(api, in.Id, nil)

		return &pb.StopProcessResponse{
			Success: true,
		}, nil
	}

	utils.ExitPid(process.Pid, 1*time.Second)
	updateProcessMap(api, in.Id, nil)

	return &pb.StopProcessResponse{
		Success: true,
	}, nil
}
