package server

import (
	"context"

	pb "github.com/dunstorm/pm2-go/proto"
)

// stop process
func (api *Handler) StopProcess(ctx context.Context, in *pb.StopProcessRequest) (*pb.StopProcessResponse, error) {
	api.mu.Lock()
	defer api.mu.Unlock()

	process := api.databaseById[in.Id]
	found := api.processes[in.Id]

	process.SetStatus("stopped")
	process.ResetCPUMemory()
	process.StopSignal = true

	if found == nil {
		api.logger.Info().Msgf("process not found: %d", in.Id)
		return &pb.StopProcessResponse{
			Success: false,
		}, nil
	}

	process.ResetPid()

	// for child process
	found.Kill()
	updateProcessMap(api, in.Id, nil)

	return &pb.StopProcessResponse{
		Success: true,
	}, nil
}
