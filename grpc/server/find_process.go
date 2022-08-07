package server

import (
	"context"
	"strconv"

	pb "github.com/dunstorm/pm2-go/proto"
	"google.golang.org/grpc/status"
)

// find process
func (api *Handler) FindProcess(ctx context.Context, in *pb.FindProcessRequest) (*pb.Process, error) {
	api.mu.Lock()
	defer api.mu.Unlock()
	var process *pb.Process

	if id, err := strconv.Atoi(in.Name); err == nil {
		process = api.databaseById[int32(id)]
	} else {
		process = api.databaseByName[in.Name]
	}

	if process == nil {
		return nil, status.Error(400, "failed to find process")
	}

	return process, nil
}
