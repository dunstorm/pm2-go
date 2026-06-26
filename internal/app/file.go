package app

import (
	"encoding/json"
	"os"

	"github.com/dunstorm/pm2-go/internal/utils"
	pb "github.com/dunstorm/pm2-go/proto"
)

type Data struct {
	Name                     string            `json:"name"`
	Args                     []string          `json:"args"`
	ExecutablePath           string            `json:"executable_path"`
	AutoRestart              bool              `json:"autorestart"`
	Cwd                      string            `json:"cwd"`
	Env                      map[string]string `json:"env"`
	Scripts                  []string          `json:"scripts"`
	CronRestart              string            `json:"cron_restart"`
	MaxRestarts              int32             `json:"max_restarts"`
	MinUptimeMS              int32             `json:"min_uptime"`
	RestartDelayMS           int32             `json:"restart_delay"`
	ExpBackoffRestartDelayMS int32             `json:"exp_backoff_restart_delay"`
	MaxMemoryRestart         int64             `json:"max_memory_restart"`
}

type StartFileOptions struct {
	Env           map[string]string
	Graceful      bool
	Signal        string
	KillTimeoutMS int32
}

func readFileJson(filePath string) ([]Data, error) {
	// read file
	content, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}

	var payload []Data
	err = tryToParseApps(content, &payload)
	if err != nil {
		return nil, err
	}
	return payload, nil
}

func (app *App) StartFile(filePath string) error {
	return app.StartFileWithOptions(filePath, StartFileOptions{})
}

func (app *App) StartFileWithOptions(filePath string, options StartFileOptions) error {
	payload, err := readFileJson(filePath)
	if err != nil {
		return err
	}
	baseEnv := options.Env
	if baseEnv == nil {
		baseEnv = utils.EnvironmentMap(os.Environ())
	}

	for _, p := range payload {
		process := app.FindProcess(p.Name)
		env := utils.MergeStringMaps(baseEnv, p.Env)
		if process == nil {
			app.SpawnProcess(SpawnParams{
				Name:                     p.Name,
				Args:                     p.Args,
				ExecutablePath:           p.ExecutablePath,
				AutoRestart:              p.AutoRestart,
				Cwd:                      p.Cwd,
				CronRestart:              p.CronRestart,
				Env:                      env,
				MaxRestarts:              p.MaxRestarts,
				MinUptimeMS:              p.MinUptimeMS,
				RestartDelayMS:           p.RestartDelayMS,
				ExpBackoffRestartDelayMS: p.ExpBackoffRestartDelayMS,
				MaxMemoryRestart:         p.MaxMemoryRestart,
			})
		} else {
			restartProcess := processFromData(process.Id, p, env)
			if process.ProcStatus.Status == "online" {
				app.logger.Info().Msgf("Applying action restartProcessId on app [%s](pid: [ %d ])", process.Name, process.Pid)
				app.RestartProcessWithOptions(restartProcess, RestartOptions{
					Graceful:      options.Graceful,
					Signal:        options.Signal,
					KillTimeoutMS: options.KillTimeoutMS,
				})
			} else {
				app.logger.Info().Msgf("Applying action startProcessId on app [%s]", process.Name)
				app.RestartProcessWithOptions(restartProcess, RestartOptions{
					Graceful:      options.Graceful,
					Signal:        options.Signal,
					KillTimeoutMS: options.KillTimeoutMS,
				})
			}
		}
	}
	return nil
}

func (app *App) StopFile(filePath string) error {
	payload, err := readFileJson(filePath)
	if err != nil {
		return err
	}

	for _, p := range payload {
		var process *pb.Process = app.FindProcess(p.Name)
		if process == nil {
			app.logger.Warn().Msgf("App [%s] not found", p.Name)
		} else {
			if process.ProcStatus.Status == "online" {
				app.logger.Info().Msgf("Applying action stopProcessId on app [%s](pid: [ %d ])", process.Name, process.Pid)
				app.StopProcess(process.Id)
			} else {
				app.logger.Warn().Msgf("App [%s] is not running", p.Name)
			}
		}
	}
	return nil
}

func (app *App) DeleteFile(filePath string) error {
	payload, err := readFileJson(filePath)
	if err != nil {
		return err
	}

	for _, p := range payload {
		var process *pb.Process = app.FindProcess(p.Name)
		if process == nil {
			app.logger.Warn().Msgf("App [%s] not found", p.Name)
		} else {
			if process.ProcStatus.Status == "online" {
				app.logger.Info().Msgf("Applying action stopProcessId on app [%s](pid: [ %d ])", process.Name, process.Pid)
				app.StopProcess(process.Id)
			}
			app.logger.Info().Msgf("Applying action deleteProcessId on app [%s]", process.Name)
			app.DeleteProcess(process)

			app.logger.Info().Msgf("[%s] ✓", p.Name)
		}
	}
	return nil
}

func (app *App) FlushFile(filePath string, flushProcess func(process *pb.Process)) error {
	payload, err := readFileJson(filePath)
	if err != nil {
		return err
	}

	for _, p := range payload {
		var process *pb.Process = app.FindProcess(p.Name)
		if process == nil || process.ProcStatus == nil {
			app.logger.Warn().Msgf("App [%s] not found", p.Name)
		} else {
			flushProcess(process)
		}
	}
	return nil
}

func (app *App) RestoreProcess(allProcesses []*pb.Process) {
	for _, p := range allProcesses {
		process := app.FindProcess(p.Name)
		if process == nil || process.ProcStatus == nil {
			app.SpawnProcess(SpawnParams{
				Name:                     p.Name,
				Args:                     p.Args,
				ExecutablePath:           p.ExecutablePath,
				AutoRestart:              p.AutoRestart,
				Cwd:                      p.Cwd,
				CronRestart:              p.CronRestart,
				Env:                      p.Env,
				MaxRestarts:              p.MaxRestarts,
				MinUptimeMS:              p.MinUptimeMs,
				RestartDelayMS:           p.RestartDelayMs,
				ExpBackoffRestartDelayMS: p.ExpBackoffRestartDelayMs,
				MaxMemoryRestart:         p.MaxMemoryRestart,
			})
		} else {
			if process.ProcStatus.Status == "online" {
				app.logger.Info().Msgf("Applying action restartProcessId on app [%s](pid: [ %d ])", process.Name, process.Pid)
			} else {
				app.logger.Info().Msgf("Applying action startProcessId on app [%s]", process.Name)
			}
			p.Id = process.Id
			app.RestartProcess(&pb.Process{
				Id:                       p.Id,
				Name:                     p.Name,
				Args:                     p.Args,
				ExecutablePath:           p.ExecutablePath,
				AutoRestart:              p.AutoRestart,
				Cwd:                      p.Cwd,
				CronRestart:              p.CronRestart,
				Env:                      p.Env,
				MaxRestarts:              p.MaxRestarts,
				MinUptimeMs:              p.MinUptimeMs,
				RestartDelayMs:           p.RestartDelayMs,
				ExpBackoffRestartDelayMs: p.ExpBackoffRestartDelayMs,
				MaxMemoryRestart:         p.MaxMemoryRestart,
			})
		}
	}
}

func processFromData(id int32, data Data, env map[string]string) *pb.Process {
	return &pb.Process{
		Id:                       id,
		Name:                     data.Name,
		Args:                     data.Args,
		ExecutablePath:           data.ExecutablePath,
		AutoRestart:              data.AutoRestart,
		Cwd:                      data.Cwd,
		CronRestart:              data.CronRestart,
		Env:                      env,
		MaxRestarts:              data.MaxRestarts,
		MinUptimeMs:              data.MinUptimeMS,
		RestartDelayMs:           data.RestartDelayMS,
		ExpBackoffRestartDelayMs: data.ExpBackoffRestartDelayMS,
		MaxMemoryRestart:         data.MaxMemoryRestart,
	}
}

type WithAppsField struct {
	Apps []Data `json:"apps"`
}

func tryToParseApps(content []byte, payload *[]Data) error {
	var withAppsField WithAppsField
	err := json.Unmarshal(content, &withAppsField)
	if err != nil {
		return json.Unmarshal(content, payload)
	}
	*payload = withAppsField.Apps
	return nil
}
