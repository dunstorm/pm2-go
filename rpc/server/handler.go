package server

import (
	"os"
	"strconv"
	"sync"
	"syscall"
	"time"

	"github.com/dunstorm/pm2-go/shared"
	"github.com/dunstorm/pm2-go/utils"
	"github.com/rs/zerolog"
)

type API struct {
	logger   *zerolog.Logger
	database []shared.Process
	mu       sync.Mutex
}

func (api *API) GetDB(empty string, reply *[]shared.Process) error {
	*reply = api.database
	return nil
}

// FindProcess takes a string type and returns a Process
func (api *API) FindProcess(name string, reply *shared.Process) error {
	var found shared.Process
	// Range statement that iterates over database
	// 'v' is the value of the current iterateee
	id, err := strconv.Atoi(name)
	if err != nil {
		id = -1
	}
	for _, v := range api.database {
		if v.Name == name || v.ID == id {
			found = v
			break
		}
	}
	// found will either be the found Process or a zerod Process
	*reply = found
	return nil
}

// MakeProcess takes a Process type and appends to the database
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
		process.Pid = 0
	}
	process.SetProcess(found)
	process.SetToStop(false)
	api.mu.Lock()
	api.database = append(api.database, process)
	api.mu.Unlock()
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

func (api *API) StopProcess(process shared.Process, reply *bool) error {
	var found shared.Process
	for _, p := range api.database {
		if p.Name == process.Name || p.ID == process.ID {
			found = p
			break
		}
	}
	if found.Pid == 0 {
		api.logger.Info().Msgf("process not found: %s", found.Name)
		*reply = false
		return nil
	}
	found.SetStatus("stopping")
	if found.AutoRestart {
		found.SetToStop(true)
		api.database[found.ID] = found
	}
	p := found.GetProcess()
	if p == nil {
		api.logger.Info().Msgf("process not found: %s", process.Name)
		*reply = false
		return nil
	}
	err := p.Signal(syscall.SIGINT)
	if err != nil {
		api.logger.Info().Msgf("failed to stop process: %s", err.Error())
		*reply = false
		return nil
	}

	go func() {
		// 2 seconds to wait for process to stop
		utils.ExitPid(found.Pid, 2*time.Second)
		found.Pid = 0
		found.SetStatus("stopped")

		api.mu.Lock()
		api.database[found.ID] = found
		api.mu.Unlock()
	}()

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
	process.Cwd = newProcess.Cwd

	found, err := os.FindProcess(process.Pid)
	if err != nil {
		process.Pid = 0
	}
	process.SetProcess(found)
	process.SetToStop(false)

	api.mu.Lock()
	api.database[newProcess.ID] = process
	api.mu.Unlock()
	*reply = process
	return nil
}

// DeleteProcess takes a Process type and deletes it from ProcessArray
func (api *API) DeleteProcess(process shared.Process, reply *shared.Process) error {
	var deleted shared.Process
	for i, v := range api.database {
		if v.Name == process.Name || v.ID == process.ID {
			// Delete Process by appending the items before it and those
			// after to the database variable
			api.mu.Lock()
			api.database = append(api.database[:i], api.database[i+1:]...)
			api.mu.Unlock()
			deleted = process
			break
		}
	}
	*reply = deleted
	return nil
}
