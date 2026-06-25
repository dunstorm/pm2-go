package server

import (
	"context"

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

	if found := api.processes[in.Id]; found != nil {
		found.Kill()
	}

	delete(api.databaseById, process.Id)
	delete(api.databaseByName, process.Name)
	delete(api.processes, in.Id)

	return &pb.DeleteProcessResponse{
		Success: true,
	}, nil
}
