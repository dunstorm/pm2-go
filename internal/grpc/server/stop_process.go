package server

import (
	"context"
	"time"

	"github.com/dunstorm/pm2-go/internal/utils"
	pb "github.com/dunstorm/pm2-go/proto"
)

// stop process
func (api *Handler) StopProcess(ctx context.Context, in *pb.StopProcessRequest) (*pb.StopProcessResponse, error) {
	api.mu.Lock()
	defer api.mu.Unlock()

	process := api.databaseById[in.Id]
	found := api.processes[in.Id]

	if process == nil {
		api.logger.Info().Msgf("process not found: %d", in.Id)
		return &pb.StopProcessResponse{
			Success: false,
		}, nil
	}

	process.SetStatus("stopped")
	process.ResetCPUMemory()
	process.StopSignal = true
	delete(api.metricsUpdatedAt, in.Id)

	if found == nil {
		api.logger.Info().Msgf("process not found: %d", in.Id)
		api.persistStateLocked()
		return &pb.StopProcessResponse{
			Success: false,
		}, nil
	}

	pid := process.Pid
	process.ResetPid()

	// for child process
	_ = utils.KillProcessGroup(found)
	if pid > 0 {
		utils.ExitPid(pid, 2*time.Second)
	}
	updateProcessMap(api, in.Id, nil)
	api.persistStateLocked()

	return &pb.StopProcessResponse{
		Success: true,
	}, nil
}
