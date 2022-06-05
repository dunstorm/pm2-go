package shared

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path"
	"strings"
	"syscall"

	"github.com/dunstorm/pm2-go/utils"
	"github.com/rs/zerolog"
)

type SpawnParams struct {
	Name           string   `json:"name"`
	ExecutablePath string   `json:"executablePath"`
	Args           []string `json:"args"`
	Cwd            string   `json:"cwd"`
	AutoRestart    bool     `json:"autorestart"`
	Logger         *zerolog.Logger

	PidPilePath string `json:"-"`
	LogFilePath string `json:"-"`
	ErrFilePath string `json:"-"`

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

	if params.Logger == nil {
		params.Logger = utils.NewLogger()
	}

	if params.Cwd == "" {
		params.Cwd, _ = os.Getwd()
	}

	nameLower := strings.ToLower(params.Name)
	params.PidPilePath = path.Join(utils.GetMainDirectory(), "pids", fmt.Sprintf("%s.pid", nameLower))
	params.LogFilePath = path.Join(utils.GetMainDirectory(), "logs", fmt.Sprintf("%s-out.log", nameLower))
	params.ErrFilePath = path.Join(utils.GetMainDirectory(), "logs", fmt.Sprintf("%s-err.log", nameLower))

	return nil
}

func (params *SpawnParams) createFiles() error {
	var err error
	if params.logFile, err = os.OpenFile(params.LogFilePath, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0640); err != nil {
		return err
	}
	if params.errFile, err = os.OpenFile(params.ErrFilePath, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0640); err != nil {
		return err
	}
	if params.nullFile, err = os.Open(os.DevNull); err != nil {
		return err
	}
	return nil
}

func SpawnNewProcess(params SpawnParams) *Process {
	// validate params
	err := params.fillDefaults()
	if err != nil {
		params.Logger.Fatal().Msg(err.Error())
		return nil
	}

	splitExecutablePath := strings.Split(params.ExecutablePath, "/")
	if splitExecutablePath[len(splitExecutablePath)-1] == "python" && params.Args[0] != "-u" {
		params.Logger.Warn().Msg("Add -u flag to prevent output buffering on python")
	}

	// params.Logger.Info().Msg("Spawning new process with params: ")
	// fmt.Println(string(jsonParams))

	// create files
	err = params.createFiles()
	if err != nil {
		params.Logger.Fatal().Msg(err.Error())
		return nil
	}

	params.ExecutablePath, err = exec.LookPath(params.ExecutablePath)
	if err != nil {
		params.Logger.Fatal().Msg(err.Error())
		return nil
	}

	// create process
	var attr = os.ProcAttr{
		Dir: params.Cwd,
		Env: os.Environ(),
		Files: []*os.File{
			params.nullFile,
			params.logFile,
			params.errFile,
		},
		Sys: &syscall.SysProcAttr{
			Foreground: false,
		},
	}

	defer params.nullFile.Close()
	defer params.logFile.Close()
	defer params.errFile.Close()

	fullCommand := []string{params.ExecutablePath}
	fullCommand = append(fullCommand, params.Args...)

	process, err := os.StartProcess(params.ExecutablePath, fullCommand, &attr)

	if err == nil {
		params.Logger.Info().Msgf("[%s] ✓", params.Name)

		// write pid to file
		err = utils.WritePidToFile(params.PidPilePath, process.Pid)
		if err != nil {
			params.Logger.Fatal().Msg(err.Error())
			process.Kill()
			return nil
		}

		rpcProcess := Process{
			Name:           params.Name,
			ExecutablePath: params.ExecutablePath,
			Pid:            process.Pid,
			Args:           params.Args,
			Cwd:            params.Cwd,
			LogFilePath:    params.LogFilePath,
			ErrFilePath:    params.ErrFilePath,
			PidFilePath:    params.PidPilePath,
			AutoRestart:    params.AutoRestart,
		}

		// detaches the process
		err = process.Release()
		if err != nil {
			params.Logger.Fatal().Msg(err.Error())
			return nil
		}

		return &rpcProcess
	} else {
		params.Logger.Fatal().Msg(err.Error())
		return nil
	}
}
