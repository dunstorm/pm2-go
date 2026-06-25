package server

import (
	"context"
	"sort"

	pb "github.com/dunstorm/pm2-go/proto"
)

// list processes
func (api *Handler) ListProcess(ctx context.Context, in *pb.ListProcessRequest) (*pb.ListProcessResponse, error) {
	api.mu.Lock()
	defer api.mu.Unlock()
	for _, p := range api.databaseById {
		p.UpdateCPUMemory()
	}

	// convert map to sorted slice
	keys := make([]int32, len(api.databaseById))
	i := 0
	for k := range api.databaseById {
		keys[i] = k
		i++
	}
	sort.Slice(keys, func(i, j int) bool {
		return keys[i] < keys[j]
	})

	var processes []*pb.Process
	for _, k := range keys {
		processes = append(processes, api.databaseById[k])
	}

	return &pb.ListProcessResponse{Processes: processes}, nil
}
