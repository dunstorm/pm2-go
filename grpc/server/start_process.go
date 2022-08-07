package server

import (
	"context"
	"os"

	pb "github.com/dunstorm/pm2-go/proto"
)

// start/update process
func (api *Handler) StartProcess(ctx context.Context, in *pb.StartProcessRequest) (*pb.Process, error) {
	api.mu.Lock()
	defer api.mu.Unlock()

	process := api.databaseById[in.Id]

	process.InitStartedAt()
	process.InitUptime()

	process.Name = in.Name
	process.ExecutablePath = in.ExecutablePath
	process.Args = in.Args
	process.PidFilePath = in.PidFilePath
	process.LogFilePath = in.LogFilePath
	process.ErrFilePath = in.ErrFilePath
	process.AutoRestart = in.AutoRestart
	process.Cwd = in.Cwd
	process.Pid = in.Pid

	found, err := os.FindProcess(int(in.Pid))
	if err != nil {
		process.Pid = in.Pid
	}

	process.SetToStop(false)
	process.SetStatus("online")
	updateProcessMap(api, in.Id, found)

	return process, nil
}
