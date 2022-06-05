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
			})
			app.AddProcess(process)
		} else {
			if process.ProcStatus.Status == "online" {
				app.logger.Info().Msgf("Applying action restartProcessId on app [%d](pid: [ %d ])", process.ID, process.Pid)
				app.StopProcessByIndex(process.ID)
			} else {
				app.logger.Info().Msgf("Applying action startProcessId on app [%d](pid: [ %d ])", process.ID, process.Pid)
			}
			newProcess := shared.SpawnNewProcess(shared.SpawnParams{
				Name:           process.Name,
				Args:           process.Args,
				ExecutablePath: process.ExecutablePath,
				AutoRestart:    process.AutoRestart,
				Logger:         app.logger,
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
				app.logger.Info().Msgf("Applying action stopProcessId on app [%d](pid: [ %d ])", process.ID, process.Pid)
				app.StopProcessByIndex(process.ID)
			} else {
				app.logger.Warn().Msgf("App [%s] is already stopped", p.Name)
			}
		}
	}
	return nil
}
