package app

import (
	"path/filepath"
	"strings"

	"github.com/dunstorm/pm2-go/internal/grpc/client"
	"github.com/dunstorm/pm2-go/internal/utils"
	pb "github.com/dunstorm/pm2-go/proto"
	"github.com/rs/zerolog"
)

type SpawnParams struct {
	Name           string
	ExecutablePath string
	Args           []string
	Cwd            string
	Env            map[string]string
	AutoRestart    bool
	CronRestart    string
}

type App struct {
	client *client.Client
	logger *zerolog.Logger
}

func New() *App {
	return NewWithPort(50051)
}

func NewWithPort(port int) *App {
	logger := utils.NewLogger()
	client, err := client.New(port)
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

func (app *App) ListProcess() []*pb.Process {
	return app.client.ListProcess()
}

func (app *App) FindProcess(name string) *pb.Process {
	return app.client.FindProcess(name)
}

func (app *App) StopProcess(index int32) bool {
	return app.client.StopProcess(index)
}

func (app *App) RestartProcess(process *pb.Process) *pb.Process {
	return app.RestartProcessWithEnv(process, nil)
}

func (app *App) RestartProcessWithEnv(process *pb.Process, env map[string]string) *pb.Process {
	if env == nil {
		env = process.Env
	}

	return app.client.RestartProcess(&pb.RestartProcessRequest{
		Id:             process.Id,
		Name:           process.Name,
		Args:           process.Args,
		ExecutablePath: process.ExecutablePath,
		AutoRestart:    process.AutoRestart,
		Cwd:            process.Cwd,
		CronRestart:    process.CronRestart,
		Env:            env,
	})
}

func (app *App) DeleteProcess(process *pb.Process) bool {
	return app.client.DeleteProcess(process.Id)
}

func (app *App) SpawnProcess(params SpawnParams) bool {
	resp := app.client.SpawnProcess(&pb.SpawnProcessRequest{
		Name:           params.Name,
		ExecutablePath: params.ExecutablePath,
		Args:           params.Args,
		Cwd:            params.Cwd,
		AutoRestart:    params.AutoRestart,
		CronRestart:    params.CronRestart,
		Env:            params.Env,
	})

	if !resp.Success {
		app.logger.Fatal().Msg("Server failed to register spawned process")
		return false
	}

	name := params.Name
	if name == "" {
		name = strings.ToLower(filepath.Base(params.ExecutablePath))
	}
	app.logger.Info().Msgf("[%s] ✓", name)

	return resp.Success
}
