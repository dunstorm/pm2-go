package app

import (
	"net/rpc"
	"os"

	"github.com/dunstorm/pm2-go/rpc/server"
	log "github.com/sirupsen/logrus"
)

type App struct {
	client *rpc.Client
}

func New() *App {
	return &App{}
}

func (a *App) createClient() {
	var err error
	a.client, err = rpc.DialHTTP("tcp", "localhost:9001")
	if err != nil {
		log.Fatal("Connection error: ", err)
		os.Exit(1)
	}
}

func (a *App) AddProcess(process *server.Process) server.Process {
	var reply server.Process
	a.createClient()
	defer a.client.Close()
	a.client.Call("API.AddProcess", process, &reply)
	return reply
}

func (a *App) ListProcs() []server.Process {
	var db []server.Process
	a.createClient()
	defer a.client.Close()
	a.client.Call("API.GetDB", "", &db)
	return db
}

func (a *App) FindProcess(name string) *server.Process {
	var reply server.Process
	a.createClient()
	defer a.client.Close()
	a.client.Call("API.FindProcess", name, &reply)
	return &reply
}

func (a *App) StopProcessByIndex(index int) bool {
	var reply bool
	a.createClient()
	defer a.client.Close()
	a.client.Call("API.StopProcessByIndex", index, &reply)
	return reply
}

func (a *App) StopProcessByName(name string) bool {
	var reply bool
	a.createClient()
	defer a.client.Close()
	a.client.Call("API.StopProcessByName", name, &reply)
	return reply
}

func (a *App) StartProcess(newProcess *server.Process) *server.Process {
	var reply *server.Process
	a.createClient()
	defer a.client.Close()
	a.client.Call("API.UpdateProcess", newProcess, &reply)
	return reply
}

func (a *App) RestartProcess(process *server.Process) *server.Process {
	a.StopProcessByIndex(process.ID)
	newProcess := a.SpawnNewProcess(SpawnParams{
		Name:           process.Name,
		Args:           process.Args,
		ExecutablePath: process.ExecutablePath,
	})
	process = a.StartProcess(newProcess)
	return process
}
