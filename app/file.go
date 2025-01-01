package app

import (
	"encoding/json"
	"os"

	pb "github.com/dunstorm/pm2-go/proto"
	"github.com/dunstorm/pm2-go/shared"
)

type Data struct {
	Name           string   `json:"name"`
	Args           []string `json:"args"`
	ExecutablePath string   `json:"executable_path"`
	AutoRestart    bool     `json:"autorestart"`
	Cwd            string   `json:"cwd"`
	Scripts        []string `json:"scripts"`
	CronRestart    string   `json:"cron_restart"`
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
	payload, err := readFileJson(filePath)
	if err != nil {
		return err
	}

	for _, p := range payload {
		process := app.FindProcess(p.Name)
		if process == nil {
			app.SpawnProcess(shared.SpawnParams{
				Name:           p.Name,
				Args:           p.Args,
				ExecutablePath: p.ExecutablePath,
				AutoRestart:    p.AutoRestart,
				Cwd:            p.Cwd,
				CronRestart:    p.CronRestart,
			})
		} else {
			if process.ProcStatus.Status == "online" {
				app.logger.Info().Msgf("Applying action restartProcessId on app [%s](pid: [ %d ])", process.Name, process.Pid)
				app.RestartProcess(process)
			} else {
				app.logger.Info().Msgf("Applying action startProcessId on app [%s]", process.Name)
				app.StartProcess(process)
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
		if process.ProcStatus == nil {
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
		if process.ProcStatus == nil {
			process, err := shared.SpawnNewProcess(shared.SpawnParams{
				Name:           p.Name,
				Args:           p.Args,
				ExecutablePath: p.ExecutablePath,
				AutoRestart:    p.AutoRestart,
				Logger:         app.logger,
				Cwd:            p.Cwd,
				CronRestart:    p.CronRestart,
			})
			if err != nil {
				app.logger.Fatal().Msgf("Error while restoring process [%s]", p.Name)
			}
			app.AddProcess(process)
		} else {
			if process.ProcStatus.Status == "online" {
				app.logger.Info().Msgf("Applying action restartProcessId on app [%s](pid: [ %d ])", process.Name, process.Pid)
				app.StopProcess(process.Id)
			} else {
				app.logger.Info().Msgf("Applying action startProcessId on app [%s]", process.Name)
			}
			newProcess, err := shared.SpawnNewProcess(shared.SpawnParams{
				Name:           process.Name,
				Args:           p.Args,
				ExecutablePath: p.ExecutablePath,
				AutoRestart:    p.AutoRestart,
				Logger:         app.logger,
				Cwd:            p.Cwd,
				CronRestart:    p.CronRestart,
			})
			if err != nil {
				app.logger.Fatal().Msgf("Error while restoring process [%s]", err.Error())
			}
			newProcess.Id = process.Id
			app.StartProcess(newProcess)
		}
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
