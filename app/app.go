package app

import (
	"github.com/dunstorm/pm2-go/grpc/client"
	pb "github.com/dunstorm/pm2-go/proto"
	"github.com/dunstorm/pm2-go/shared"
	"github.com/dunstorm/pm2-go/utils"
	"github.com/rs/zerolog"
)

type App struct {
	client *client.Client
	logger *zerolog.Logger
}

func New() *App {
	logger := utils.NewLogger()
	client, err := client.New(50051)
	if err != nil {
		logger.Fatal().Err(err).Msg("Failed to create client")
	}
	return &App{
		logger: logger,
		client: client,
	}
}

func (app *App) GetLogger() *zerolog.Logger {
	return app.logger
}

func (app *App) AddProcess(process *pb.Process) int32 {
	return app.client.AddProcess(&pb.AddProcessRequest{
		Name:           process.Name,
		ExecutablePath: process.ExecutablePath,
		Args:           process.Args,
		Cwd:            process.Cwd,
		Pid:            process.Pid,
		AutoRestart:    process.AutoRestart,
		Scripts:        process.Scripts,
		PidFilePath:    process.PidFilePath,
		LogFilePath:    process.LogFilePath,
		ErrFilePath:    process.ErrFilePath,
		CronRestart:    process.CronRestart,
	})
}

func (app *App) ListProcess() []*pb.Process {
	return app.client.ListProcess()
}

func (app *App) FindProcess(name string) *pb.Process {
	return app.client.FindProcess(name)
}

func (app *App) StopProcess(index int32) bool {
	return app.client.StopProcess(index)
}

func (app *App) StartProcess(newProcess *pb.Process) *pb.Process {
	return app.client.StartProcess(&pb.StartProcessRequest{
		Id:             newProcess.Id,
		Name:           newProcess.Name,
		Args:           newProcess.Args,
		ExecutablePath: newProcess.ExecutablePath,
		Cwd:            newProcess.Cwd,
		AutoRestart:    newProcess.AutoRestart,
		Scripts:        newProcess.Scripts,
		Pid:            newProcess.Pid,
		PidFilePath:    newProcess.PidFilePath,
		LogFilePath:    newProcess.LogFilePath,
		ErrFilePath:    newProcess.ErrFilePath,
		CronRestart:    newProcess.CronRestart,
	})
}

func (app *App) RestartProcess(process *pb.Process) *pb.Process {
	app.StopProcess(process.Id)
	newProcess, err := shared.SpawnNewProcess(shared.SpawnParams{
		Name:           process.Name,
		Args:           process.Args,
		ExecutablePath: process.ExecutablePath,
		AutoRestart:    process.AutoRestart,
		Logger:         app.logger,
		Cwd:            process.Cwd,
		CronRestart:    process.CronRestart,
	})
	if err != nil {
		app.logger.Fatal().Err(err).Msg("Failed to restart process")
	}
	newProcess.Id = process.Id
	process = app.StartProcess(newProcess)
	return process
}

func (app *App) DeleteProcess(process *pb.Process) bool {
	return app.client.DeleteProcess(process.Id)
}

func (app *App) SpawnProcess(params shared.SpawnParams) bool {
	resp := app.client.SpawnProcess(&pb.SpawnProcessRequest{
		Name:           params.Name,
		ExecutablePath: params.ExecutablePath,
		Args:           params.Args,
		Cwd:            params.Cwd,
		AutoRestart:    params.AutoRestart,
		CronRestart:    params.CronRestart,
	})

	if !resp.Success {
		app.logger.Fatal().Msg("Server failed to register spawned process")
		return false
	}

	app.logger.Info().Msgf("[%s] ✓", params.Name)

	return resp.Success
}
