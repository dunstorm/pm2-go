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
	Name                     string
	ExecutablePath           string
	Args                     []string
	Cwd                      string
	Env                      map[string]string
	AutoRestart              bool
	CronRestart              string
	MaxRestarts              int32
	MinUptimeMS              int32
	RestartDelayMS           int32
	ExpBackoffRestartDelayMS int32
	MaxMemoryRestart         int64
	HealthCheckURL           string
	HealthCheckIntervalMS    int32
	HealthCheckTimeoutMS     int32
	Watch                    bool
	WatchPaths               []string
	WatchIntervalMS          int32
}

type RestartOptions struct {
	Env           map[string]string
	Graceful      bool
	Signal        string
	KillTimeoutMS int32
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
	return app.RestartProcessWithOptions(process, RestartOptions{})
}

func (app *App) RestartProcessWithEnv(process *pb.Process, env map[string]string) *pb.Process {
	return app.RestartProcessWithOptions(process, RestartOptions{Env: env})
}

func (app *App) RestartProcessWithOptions(process *pb.Process, options RestartOptions) *pb.Process {
	if options.Env == nil {
		options.Env = process.Env
	}

	return app.client.RestartProcess(&pb.RestartProcessRequest{
		Id:                       process.Id,
		Name:                     process.Name,
		Args:                     process.Args,
		ExecutablePath:           process.ExecutablePath,
		AutoRestart:              process.AutoRestart,
		Cwd:                      process.Cwd,
		CronRestart:              process.CronRestart,
		Env:                      options.Env,
		Graceful:                 options.Graceful,
		Signal:                   options.Signal,
		KillTimeoutMs:            options.KillTimeoutMS,
		MaxRestarts:              process.MaxRestarts,
		MinUptimeMs:              process.MinUptimeMs,
		RestartDelayMs:           process.RestartDelayMs,
		ExpBackoffRestartDelayMs: process.ExpBackoffRestartDelayMs,
		MaxMemoryRestart:         process.MaxMemoryRestart,
		HealthCheckUrl:           process.HealthCheckUrl,
		HealthCheckIntervalMs:    process.HealthCheckIntervalMs,
		HealthCheckTimeoutMs:     process.HealthCheckTimeoutMs,
		Watch:                    process.Watch,
		WatchPaths:               process.WatchPaths,
		WatchIntervalMs:          process.WatchIntervalMs,
	})
}

func (app *App) DeleteProcess(process *pb.Process) bool {
	return app.client.DeleteProcess(process.Id)
}

func (app *App) FlushProcess(process *pb.Process) *pb.FlushProcessResponse {
	return app.client.FlushProcess(process.Id)
}

func (app *App) SpawnProcess(params SpawnParams) bool {
	resp := app.client.SpawnProcess(&pb.SpawnProcessRequest{
		Name:                     params.Name,
		ExecutablePath:           params.ExecutablePath,
		Args:                     params.Args,
		Cwd:                      params.Cwd,
		AutoRestart:              params.AutoRestart,
		CronRestart:              params.CronRestart,
		Env:                      params.Env,
		MaxRestarts:              params.MaxRestarts,
		MinUptimeMs:              params.MinUptimeMS,
		RestartDelayMs:           params.RestartDelayMS,
		ExpBackoffRestartDelayMs: params.ExpBackoffRestartDelayMS,
		MaxMemoryRestart:         params.MaxMemoryRestart,
		HealthCheckUrl:           params.HealthCheckURL,
		HealthCheckIntervalMs:    params.HealthCheckIntervalMS,
		HealthCheckTimeoutMs:     params.HealthCheckTimeoutMS,
		Watch:                    params.Watch,
		WatchPaths:               params.WatchPaths,
		WatchIntervalMs:          params.WatchIntervalMS,
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
