package server

import (
	"context"

	"github.com/dunstorm/pm2-go/internal/utils"
	pb "github.com/dunstorm/pm2-go/proto"
)

// delete process
func (api *Handler) DeleteProcess(ctx context.Context, in *pb.DeleteProcessRequest) (*pb.DeleteProcessResponse, error) {
	api.mu.Lock()
	defer api.mu.Unlock()

	process := api.databaseById[in.Id]
	if process == nil {
		api.logger.Info().Msgf("process not found: %d", in.Id)
		return &pb.DeleteProcessResponse{
			Success: false,
		}, nil
	}

	api.bumpOperationGenerationLocked(in.Id)
	suppressProcessRestart(process)
	if found := api.processes[in.Id]; found != nil {
		_ = utils.KillProcessGroup(found)
	}

	delete(api.databaseById, process.Id)
	delete(api.databaseByName, process.Name)
	delete(api.processes, in.Id)
	delete(api.metricsUpdatedAt, in.Id)
	api.clearOperationGenerationLocked(in.Id)
	api.persistStateLocked()

	return &pb.DeleteProcessResponse{
		Success: true,
	}, nil
}

func suppressProcessRestart(process *pb.Process) {
	process.AutoRestart = false
	process.SetStopSignal(true)
	process.RestartAt = nil
	process.NextStartAt = nil
	if process.ProcStatus != nil {
		process.SetStatus("stopped")
		process.ResetCPUMemory()
	}
}
