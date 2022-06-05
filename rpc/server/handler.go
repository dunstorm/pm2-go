package server

import (
	"os"
	"syscall"
	"time"

	"github.com/dunstorm/pm2-go/shared"
	"github.com/rs/zerolog"
)

type API struct {
	logger   *zerolog.Logger
	database []shared.Process
}

func (api *API) GetDB(empty string, reply *[]shared.Process) error {
	*reply = api.database
	return nil
}

// FindProcess takes a string type and returns a Process
func (api *API) FindProcess(name string, reply *shared.Process) error {
	var found shared.Process
	// Range statement that iterates over processArray
	// 'v' is the value of the current iterateee
	for _, v := range api.database {
		if v.Name == name {
			found = v
			break
		}
	}
	// found will either be the found Process or a zerod Process
	*reply = found
	return nil
}

// MakeToDo takes a ToDo type and appends to the todoArray
func (api *API) AddProcess(process shared.Process, reply *shared.Process) error {
	process.ProcStatus = &shared.ProcStatus{
		Status:    "online",
		StartedAt: time.Now(),
		Uptime:    time.Duration(0),
		CPU:       "0.0%",
		Memory:    "0.0MB",
	}
	process.ID = len(api.database)
	found, err := os.FindProcess(process.Pid)
	if err != nil {
		process.Pid = -1
	}
	process.SetProcess(found)
	process.SetToStop(false)
	api.database = append(api.database, process)
	*reply = process
	return nil
}

func (api *API) StopProcessByIndex(processIndex int, reply *bool) error {
	process := api.database[processIndex]
	found := process.GetProcess()
	if found == nil {
		api.logger.Info().Msgf("process not found: %s", processIndex)
		*reply = false
		return nil
	}
	err := found.Signal(syscall.SIGINT)
	if err != nil {
		api.logger.Info().Msgf("failed to stop process: %s", err.Error())
		*reply = false
		return nil
	}
	*reply = true
	return nil
}

func (api *API) StopProcessByName(name string, reply *bool) error {
	var found shared.Process
	for _, process := range api.database {
		if process.Name == name {
			found = process
			break
		}
	}
	if found.Pid == 0 {
		api.logger.Info().Msgf("process not found: %s", name)
		*reply = false
		return nil
	}
	if found.AutoRestart {
		found.SetToStop(true)
		api.database[found.ID] = found
	}
	process := found.GetProcess()
	if process == nil {
		api.logger.Info().Msgf("process not found: %s", name)
		*reply = false
		return nil
	}
	err := process.Signal(syscall.SIGINT)
	if err != nil {
		api.logger.Info().Msgf("failed to stop process: %s", err.Error())
		*reply = false
		return nil
	}
	found.GetProcess().Wait()
	*reply = true
	return nil
}

func (api *API) UpdateProcess(newProcess shared.Process, reply *shared.Process) error {
	process := api.database[newProcess.ID]

	process.InitStartedAt()
	process.InitUptime()
	process.UpdatePid(newProcess.Pid)
	process.UpdateStatus("online")

	process.Name = newProcess.Name
	process.ExecutablePath = newProcess.ExecutablePath
	process.Args = newProcess.Args
	process.PidFilePath = newProcess.PidFilePath
	process.LogFilePath = newProcess.LogFilePath
	process.ErrFilePath = newProcess.ErrFilePath
	process.AutoRestart = newProcess.AutoRestart

	found, err := os.FindProcess(process.Pid)
	if err != nil {
		process.Pid = -1
	}
	process.SetProcess(found)
	process.SetToStop(false)

	api.database[newProcess.ID] = process
	*reply = process
	return nil
}

// DeleteToDo takes a ToDo type and deletes it from todoArray
func (api *API) DeleteProcess(process shared.Process, reply *shared.Process) error {
	var deleted shared.Process
	for i, v := range api.database {
		if v.Name == process.Name {
			// Delete ToDo by appending the items before it and those
			// after to the todoArray variable
			api.database = append(api.database[:i], api.database[i+1:]...)
			deleted = process
			break
		}
	}
	*reply = deleted
	return nil
}
