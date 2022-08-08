package shared

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path"
	"strings"
	"syscall"

	pb "github.com/dunstorm/pm2-go/proto"
	"github.com/dunstorm/pm2-go/utils"
	"github.com/rs/zerolog"
)

type SpawnParams struct {
	Name           string   `json:"name"`
	ExecutablePath string   `json:"executablePath"`
	Args           []string `json:"args"`
	Cwd            string   `json:"cwd"`
	AutoRestart    bool     `json:"autorestart"`
	Scripts        []string `json:"scripts"`
	CronRestart    string   `json:"cron_restart"`
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

func (params *SpawnParams) checkScripts() {
	// check if we have a script to run
	for _, script := range params.Scripts {
		scriptPath := path.Join(utils.GetMainDirectory(), "scripts", script+".sh")
		if _, err := os.Stat(scriptPath); os.IsNotExist(err) {
			params.Logger.Fatal().Msgf("Script %s not found in path %s", script, scriptPath)
		}
	}
}

// pipe each script to the process
func createPipedProcesses(params *SpawnParams, stdoutLogsRead *os.File, stderrLogsRead *os.File, stdoutLogsWrite *os.File, stderrLogsWrite *os.File) error {
	var err error
	var newStdoutLogsRead *os.File
	var newErrorLogsRead *os.File
	for index, script := range params.Scripts {
		scriptPath := path.Join(utils.GetMainDirectory(), "scripts", script+".sh")
		if index == len(params.Scripts)-1 {
			stdoutLogsWrite = params.logFile
			stderrLogsWrite = params.errFile
		} else {
			newStdoutLogsRead, stdoutLogsWrite, err = os.Pipe()
			if err != nil {
				params.Logger.Fatal().Msg(err.Error())
				return err
			}
			newErrorLogsRead, stderrLogsWrite, err = os.Pipe()
			if err != nil {
				params.Logger.Fatal().Msg(err.Error())
				return err
			}
		}
		// start stdout piped process
		_, err := os.StartProcess("/bin/sh", []string{"/bin/sh", scriptPath}, &os.ProcAttr{
			Dir: params.Cwd,
			Env: os.Environ(),
			Files: []*os.File{
				stdoutLogsRead,
				stdoutLogsWrite,
				params.nullFile,
			},
			Sys: &syscall.SysProcAttr{
				Foreground: false,
			},
		})
		if err != nil {
			params.Logger.Fatal().Msg(err.Error())
			return err
		}
		// start stderr piped process
		_, err = os.StartProcess("/bin/sh", []string{"/bin/sh", scriptPath}, &os.ProcAttr{
			Dir: params.Cwd,
			Env: os.Environ(),
			Files: []*os.File{
				stderrLogsRead,
				stderrLogsWrite,
				params.nullFile,
			},
			Sys: &syscall.SysProcAttr{
				Foreground: false,
			},
		})
		if err != nil {
			params.Logger.Fatal().Msg(err.Error())
			return err
		}
		if newStdoutLogsRead != nil {
			stdoutLogsRead = newStdoutLogsRead
			stderrLogsRead = newErrorLogsRead
		}
	}
	return nil
}

func SpawnNewProcess(params SpawnParams) (*pb.Process, error) {
	// validate params
	err := params.fillDefaults()
	if err != nil {
		return nil, err
	}

	splitExecutablePath := strings.Split(params.ExecutablePath, "/")
	if splitExecutablePath[len(splitExecutablePath)-1] == "python" && len(params.Args) > 0 && params.Args[0] != "-u" {
		params.Logger.Warn().Msg("Add -u flag to prevent output buffering on python")
	}

	// create files
	err = params.createFiles()
	if err != nil {
		return nil, err
	}

	params.ExecutablePath, err = exec.LookPath(params.ExecutablePath)
	if err != nil {
		return nil, err
	}

	var stdoutLogsWrite *os.File
	var stdoutLogsRead *os.File

	var stderrLogsWrite *os.File
	var stderrLogsRead *os.File

	if len(params.Scripts) == 0 {
		stdoutLogsWrite = params.logFile
		stderrLogsWrite = params.errFile
	} else {
		// create initial stdout pipe
		stdoutLogsRead, stdoutLogsWrite, err = os.Pipe()
		if err != nil {
			return nil, err
		}

		// create initial err pipe
		stderrLogsRead, stderrLogsWrite, err = os.Pipe()
		if err != nil {
			return nil, err
		}

		// check if scripts exist
		params.checkScripts()
	}

	defer stdoutLogsRead.Close()
	defer stdoutLogsWrite.Close()

	// create process
	var attr = os.ProcAttr{
		Dir: params.Cwd,
		Env: os.Environ(),
		Files: []*os.File{
			params.nullFile,
			stdoutLogsWrite,
			stderrLogsWrite,
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
	if err != nil {
		return nil, err
	}

	err = createPipedProcesses(&params, stdoutLogsRead, stderrLogsRead, stdoutLogsWrite, stderrLogsWrite)
	if err != nil {
		return nil, err
	}

	params.Logger.Info().Msgf("[%s] ✓", params.Name)

	// write pid to file
	err = utils.WritePidToFile(params.PidPilePath, process.Pid)
	if err != nil {
		params.Logger.Fatal().Msg(err.Error())
		process.Kill()
		return nil, err
	}

	rpcProcess := pb.Process{
		Name:           params.Name,
		ExecutablePath: params.ExecutablePath,
		Pid:            int32(process.Pid),
		Args:           params.Args,
		Cwd:            params.Cwd,
		Scripts:        params.Scripts,
		LogFilePath:    params.LogFilePath,
		ErrFilePath:    params.ErrFilePath,
		PidFilePath:    params.PidPilePath,
		AutoRestart:    params.AutoRestart,
		CronRestart:    params.CronRestart,
	}

	// detaches the process
	err = process.Release()
	if err != nil {
		params.Logger.Fatal().Msg(err.Error())
		return nil, err
	}

	return &rpcProcess, nil
}
