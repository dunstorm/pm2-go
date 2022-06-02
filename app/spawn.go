package app

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path"
	"strings"

	"github.com/dunstorm/pm2-go/rpc/server"
	"github.com/dunstorm/pm2-go/utils"
	log "github.com/sirupsen/logrus"
)

type SpawnParams struct {
	Name           string   `json:"name"`
	ExecutablePath string   `json:"executablePath"`
	Args           []string `json:"args"`

	PidPileName string `json:"-"`
	LogFileName string `json:"-"`
	ErrFileName string `json:"-"`

	logFile  *os.File
	errFile  *os.File
	nullFile *os.File
}

func (params *SpawnParams) fillDefaults() error {
	if params.ExecutablePath == "" {
		return errors.New("executable path is required")
	}

	if params.Name == "" {
		params.Name = strings.ToLower(params.ExecutablePath)
	}

	nameLower := strings.ToLower(params.Name)
	params.PidPileName = path.Join(utils.GetMainDirectory(), "pids", fmt.Sprintf("%s.pid", nameLower))
	params.LogFileName = path.Join(utils.GetMainDirectory(), "logs", fmt.Sprintf("%s-out.log", nameLower))
	params.ErrFileName = path.Join(utils.GetMainDirectory(), "logs", fmt.Sprintf("%s-err.log", nameLower))

	return nil
}

func (params *SpawnParams) createFiles() error {
	var err error
	if params.logFile, err = os.OpenFile(params.LogFileName, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0640); err != nil {
		log.Fatal(err)
		return err
	}
	if params.errFile, err = os.OpenFile(params.ErrFileName, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0640); err != nil {
		log.Fatal(err)
		return err
	}
	if params.nullFile, err = os.Open(os.DevNull); err != nil {
		return err
	}
	return nil
}

func (a *App) SpawnNewProcess(params SpawnParams) *server.Process {
	// validate params
	err := params.fillDefaults()
	if err != nil {
		log.Fatal(err)
		return nil
	}

	// for logging purposes
	jsonParams, err := json.MarshalIndent(params, "", "  ")
	if err != nil {
		log.Error("error:", err)
		return nil
	}

	log.Info("Spawning new process with params: ")
	fmt.Println(string(jsonParams))

	// create files
	err = params.createFiles()
	if err != nil {
		log.Fatal(err)
		return nil
	}

	params.ExecutablePath, err = exec.LookPath(params.ExecutablePath)
	if err != nil {
		log.Fatal(err)
		return nil
	}

	// create process
	var attr = os.ProcAttr{
		Dir: ".",
		Env: os.Environ(),
		Files: []*os.File{
			params.nullFile,
			params.logFile,
			params.errFile,
		},
	}

	defer params.nullFile.Close()
	defer params.logFile.Close()
	defer params.errFile.Close()

	fullCommand := []string{params.ExecutablePath}
	fullCommand = append(fullCommand, params.Args...)
	process, err := os.StartProcess(params.ExecutablePath, fullCommand, &attr)

	if err == nil {
		log.Info("PID: ", process.Pid)

		// write pid to file
		err = utils.WritePidToFile(params.PidPileName, process.Pid)
		if err != nil {
			log.Error(err.Error())
			process.Kill()
			return nil
		}

		rpcProcess := server.Process{
			Name:           params.Name,
			ExecutablePath: params.ExecutablePath,
			Pid:            process.Pid,
			Args:           params.Args,
		}

		// detaches the process
		err = process.Release()
		if err != nil {
			log.Error(err.Error())
			return nil
		}

		return &rpcProcess
	} else {
		log.Error(err.Error())
		return nil
	}
}
