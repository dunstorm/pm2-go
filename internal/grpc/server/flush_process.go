package server

import (
	"context"

	"github.com/dunstorm/pm2-go/internal/logstore"
	processrunner "github.com/dunstorm/pm2-go/internal/process"
	pb "github.com/dunstorm/pm2-go/proto"
)

func (api *Handler) FlushProcess(ctx context.Context, in *pb.FlushProcessRequest) (*pb.FlushProcessResponse, error) {
	api.mu.Lock()
	process := api.databaseById[in.Id]
	if process == nil {
		api.mu.Unlock()
		api.logger.Info().Msgf("process not found: %d", in.Id)
		return &pb.FlushProcessResponse{Success: false}, nil
	}
	logFilePaths := processLogFilePaths(process)
	api.mu.Unlock()

	success := true
	for _, logFilePath := range logFilePaths {
		if err := processrunner.FlushLogFile(logFilePath); err != nil {
			api.logger.Error().Msgf("Error while flushing log file %s: %s", logFilePath, err)
			success = false
		}
	}

	return &pb.FlushProcessResponse{
		Success:      success,
		LogFilePaths: logFilePaths,
	}, nil
}

func processLogFilePaths(process *pb.Process) []string {
	paths := make([]string, 0, 3)
	seen := make(map[string]bool, 3)
	for _, path := range []string{
		process.LogFilePath,
		process.ErrFilePath,
		logstore.CombinedPath(process.LogFilePath),
	} {
		if path == "" || seen[path] {
			continue
		}
		seen[path] = true
		paths = append(paths, path)
	}
	return paths
}
