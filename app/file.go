package app

import (
	"encoding/json"
	"io/ioutil"

	"github.com/dunstorm/pm2-go/shared"
)

type Data struct {
	Name           string   `json:"name"`
	Args           []string `json:"args"`
	ExecutablePath string   `json:"executablePath"`
	AutoRestart    bool     `json:"autorestart"`
	Cwd            string   `json:"cwd"`
}

func (app *App) StartFile(filePath string) error {
	// read file
	content, err := ioutil.ReadFile(filePath)
	if err != nil {
		return err
	}

	var payload []Data
	err = json.Unmarshal(content, &payload)
	if err != nil {
		return err
	}

	for _, p := range payload {
		var process *shared.Process
		process = app.FindProcess(p.Name)
		if process.ProcStatus == nil {
			process = shared.SpawnNewProcess(shared.SpawnParams{
				Name:           p.Name,
				Args:           p.Args,
				ExecutablePath: p.ExecutablePath,
				AutoRestart:    p.AutoRestart,
				Logger:         app.logger,
				Cwd:            p.Cwd,
			})
			app.AddProcess(process)
		} else {
			if process.ProcStatus.Status == "online" {
				app.logger.Info().Msgf("Applying action restartProcessId on app [%s](pid: [ %d ])", process.Name, process.Pid)
				app.StopProcessByIndex(process.ID)
			} else {
				app.logger.Info().Msgf("Applying action startProcessId on app [%s]", process.Name)
			}
			newProcess := shared.SpawnNewProcess(shared.SpawnParams{
				Name:           process.Name,
				Args:           process.Args,
				ExecutablePath: process.ExecutablePath,
				AutoRestart:    process.AutoRestart,
				Logger:         app.logger,
				Cwd:            process.Cwd,
			})
			newProcess.ID = process.ID
			app.StartProcess(newProcess)
		}
	}
	return nil
}

func (app *App) StopFile(filePath string) error {
	// read file
	content, err := ioutil.ReadFile(filePath)
	if err != nil {
		return err
	}

	var payload []Data
	err = json.Unmarshal(content, &payload)
	if err != nil {
		return err
	}

	for _, p := range payload {
		var process *shared.Process = app.FindProcess(p.Name)
		if process.ProcStatus == nil {
			app.logger.Warn().Msgf("App [%s] not found", p.Name)
		} else {
			if process.ProcStatus.Status == "online" {
				app.logger.Info().Msgf("Applying action stopProcessId on app [%s](pid: [ %d ])", process.Name, process.Pid)
				app.StopProcess(process)
			} else {
				app.logger.Warn().Msgf("App [%s] is not running", p.Name)
			}
		}
	}
	return nil
}

func (app *App) DeleteFile(filePath string) error {
	// read file
	content, err := ioutil.ReadFile(filePath)
	if err != nil {
		return err
	}

	var payload []Data
	err = json.Unmarshal(content, &payload)
	if err != nil {
		return err
	}

	for _, p := range payload {
		var process *shared.Process = app.FindProcess(p.Name)
		if process.ProcStatus == nil {
			app.logger.Warn().Msgf("App [%s] not found", p.Name)
		} else {
			if process.ProcStatus.Status == "online" {
				app.logger.Info().Msgf("Applying action stopProcessId on app [%s](pid: [ %d ])", process.Name, process.Pid)
				app.StopProcess(process)
			}
			app.logger.Info().Msgf("Applying action deleteProcessId on app [%s]", process.Name)
			app.DeleteProcess(process)

			app.logger.Info().Msgf("[%s] ✓", p.Name)
		}
	}
	return nil
}
