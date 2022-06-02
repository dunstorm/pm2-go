package server

import (
	"os"
	"syscall"
	"time"

	log "github.com/sirupsen/logrus"
)

type API int

var database []Process

func (a *API) GetDB(empty string, reply *[]Process) error {
	*reply = database
	return nil
}

// FindProcess takes a string type and returns a Process
func (a *API) FindProcess(name string, reply *Process) error {
	var found Process
	// Range statement that iterates over processArray
	// 'v' is the value of the current iterateee
	for _, v := range database {
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
func (a *API) AddProcess(process Process, reply *Process) error {
	process.ProcStatus = &ProcStatus{
		Status:    "online",
		StartedAt: time.Now(),
		Uptime:    time.Duration(0),
		CPU:       "0.0%",
		Memory:    "0.0MB",
	}
	process.ID = len(database)
	var err error
	process.process, err = os.FindProcess(process.Pid)
	if err != nil {
		process.Pid = 0
	}
	database = append(database, process)
	*reply = process
	return nil
}

func (a *API) StopProcessByIndex(processIndex int, reply *bool) error {
	process := database[processIndex]
	err := process.process.Signal(syscall.SIGTERM)
	if err != nil {
		log.Info("failed to stop process: ", err.Error())
		*reply = false
		return nil
	}
	*reply = true
	return nil
}

func (a *API) StopProcessByName(name string, reply *bool) error {
	var found Process
	for _, process := range database {
		if process.Name == name {
			found = process
			break
		}
	}
	if found.Pid == 0 {
		log.Info("process not found: ", name)
		*reply = false
		return nil
	}
	err := found.process.Signal(syscall.SIGTERM)
	if err != nil {
		log.Info("failed to stop process: ", err.Error())
		*reply = false
		return nil
	}
	found.process.Wait()
	*reply = true
	return nil
}

func (a *API) UpdateProcess(newProcess Process, reply *Process) error {
	process := database[newProcess.ID]

	process.InitStartedAt()
	process.UpdatePid(newProcess.Pid)
	process.UpdateStatus("online")

	database[newProcess.ID] = process
	*reply = process
	return nil
}

// DeleteToDo takes a ToDo type and deletes it from todoArray
func (a *API) DeleteProcess(process Process, reply *Process) error {
	var deleted Process
	for i, v := range database {
		if v.Name == process.Name {
			// Delete ToDo by appending the items before it and those
			// after to the todoArray variable
			database = append(database[:i], database[i+1:]...)
			deleted = process
			break
		}
	}
	*reply = deleted
	return nil
}
