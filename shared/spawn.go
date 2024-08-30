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
	for _, script := range params.Scripts {
		scriptPath := path.Join(utils.GetMainDirectory(), "scripts", script+".sh")
		if _, err := os.Stat(scriptPath); os.IsNotExist(err) {
			params.Logger.Fatal().Msgf("Script %s not found in path %s", script, scriptPath)
		}
	}
}

func createPipedProcesses(params *SpawnParams, stdoutLogsRead *os.File, stderrLogsRead *os.File, stdoutLogsWrite *os.File, stderrLogsWrite *os.File) error {
	var err error
	var newStdoutLogsRead, newErrorLogsRead *os.File
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
		for _, stream := range []struct {
			read, write *os.File
		}{
			{stdoutLogsRead, stdoutLogsWrite},
			{stderrLogsRead, stderrLogsWrite},
		} {
			cmd := exec.Command("/bin/sh", scriptPath)
			cmd.Dir = params.Cwd
			cmd.Env = os.Environ()
			cmd.Stdin = stream.read
			cmd.Stdout = stream.write
			cmd.Stderr = params.nullFile
			if err := cmd.Start(); err != nil {
				params.Logger.Fatal().Msg(err.Error())
				return err
			}
		}
		if newStdoutLogsRead != nil {
			stdoutLogsRead = newStdoutLogsRead
			stderrLogsRead = newErrorLogsRead
		}
	}
	return nil
}

func SpawnNewProcess(params SpawnParams) (*pb.Process, error) {
	if err := params.fillDefaults(); err != nil {
		return nil, err
	}

	splitExecutablePath := strings.Split(params.ExecutablePath, "/")
	if splitExecutablePath[len(splitExecutablePath)-1] == "python" && len(params.Args) > 0 && params.Args[0] != "-u" {
		params.Logger.Warn().Msg("Add -u flag to prevent output buffering on python")
	}

	if err := params.createFiles(); err != nil {
		return nil, err
	}

	var err error
	params.ExecutablePath, err = exec.LookPath(params.ExecutablePath)
	if err != nil {
		return nil, err
	}

	var stdoutLogsWrite, stdoutLogsRead, stderrLogsWrite, stderrLogsRead *os.File

	if len(params.Scripts) == 0 {
		stdoutLogsWrite = params.logFile
		stderrLogsWrite = params.errFile
	} else {
		stdoutLogsRead, stdoutLogsWrite, err = os.Pipe()
		if err != nil {
			return nil, err
		}
		stderrLogsRead, stderrLogsWrite, err = os.Pipe()
		if err != nil {
			return nil, err
		}
		params.checkScripts()
	}

	defer func() {
		if stdoutLogsRead != nil {
			stdoutLogsRead.Close()
		}
		if stdoutLogsWrite != nil {
			stdoutLogsWrite.Close()
		}
		params.nullFile.Close()
		params.logFile.Close()
		params.errFile.Close()
	}()

	cmd := exec.Command(params.ExecutablePath, params.Args...)
	cmd.Dir = params.Cwd
	cmd.Env = os.Environ()
	cmd.Stdin = params.nullFile
	cmd.Stdout = stdoutLogsWrite
	cmd.Stderr = stderrLogsWrite
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setpgid: true,
	}

	if err := cmd.Start(); err != nil {
		return nil, err
	}

	if len(params.Scripts) > 0 {
		err = createPipedProcesses(&params, stdoutLogsRead, stderrLogsRead, stdoutLogsWrite, stderrLogsWrite)
		if err != nil {
			return nil, err
		}
	}

	params.Logger.Info().Msgf("[%s] ✓", params.Name)

	if err := utils.WritePidToFile(params.PidPilePath, cmd.Process.Pid); err != nil {
		params.Logger.Fatal().Msg(err.Error())
		cmd.Process.Kill()
		return nil, err
	}

	rpcProcess := &pb.Process{
		Name:           params.Name,
		ExecutablePath: params.ExecutablePath,
		Pid:            int32(cmd.Process.Pid),
		Args:           params.Args,
		Cwd:            params.Cwd,
		Scripts:        params.Scripts,
		LogFilePath:    params.LogFilePath,
		ErrFilePath:    params.ErrFilePath,
		PidFilePath:    params.PidPilePath,
		AutoRestart:    params.AutoRestart,
		CronRestart:    params.CronRestart,
	}

	return rpcProcess, nil
}
