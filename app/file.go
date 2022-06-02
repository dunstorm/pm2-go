package app

import (
	"encoding/json"
	"io/ioutil"

	"github.com/dunstorm/pm2-go/rpc/server"
	log "github.com/sirupsen/logrus"
)

type Data struct {
	Name           string   `json:"name"`
	Args           []string `json:"args"`
	ExecutablePath string   `json:"executablePath"`
	Autorestart    bool     `json:"autorestart"`
}

func (a *App) StartFile(filePath string) {
	// read file
	content, err := ioutil.ReadFile(filePath)
	if err != nil {
		log.Fatal(err)
		return
	}

	var payload []Data
	err = json.Unmarshal(content, &payload)
	if err != nil {
		log.Fatal(err)
		return
	}

	for _, p := range payload {
		var process *server.Process
		process = a.FindProcess(p.Name)
		if process.ProcStatus == nil {
			process = a.SpawnNewProcess(SpawnParams{
				Name:           p.Name,
				Args:           p.Args,
				ExecutablePath: p.ExecutablePath,
			})
			a.AddProcess(process)
		} else {
			log.Info("Applying action restartProcessId on app [", process.ID, "](pid: [ '", process.Pid, "' ])")
			a.StopProcessByIndex(process.ID)
			newProcess := a.SpawnNewProcess(SpawnParams{
				Name:           process.Name,
				Args:           process.Args,
				ExecutablePath: process.ExecutablePath,
			})
			newProcess.ID = process.ID
			a.StartProcess(newProcess)
		}
	}
}
