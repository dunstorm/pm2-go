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

	delete(api.databaseById, process.Id)
	delete(api.databaseByName, process.Name)
	delete(api.processes, in.Id)

	return &pb.DeleteProcessResponse{
		Success: true,
	}, nil
}
