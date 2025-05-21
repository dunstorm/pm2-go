package shared

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
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
	params.PidPilePath = filepath.Join(utils.GetMainDirectory(), "pids", fmt.Sprintf("%s.pid", nameLower))
	params.LogFilePath = filepath.Join(utils.GetMainDirectory(), "logs", fmt.Sprintf("%s-out.log", nameLower))
	params.ErrFilePath = filepath.Join(utils.GetMainDirectory(), "logs", fmt.Sprintf("%s-err.log", nameLower))

	return nil
}

func (params *SpawnParams) createFiles() error {
	var err error
	if params.logFile, err = os.OpenFile(params.LogFilePath, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0644); err != nil {
		return err
	}
	if params.errFile, err = os.OpenFile(params.ErrFilePath, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0644); err != nil {
		return err
	}
	if params.nullFile, err = os.Open(os.DevNull); err != nil {
		return err
	}
	return nil
}

func SpawnNewProcess(params SpawnParams) (*pb.Process, error) {
	if err := params.fillDefaults(); err != nil {
		return nil, err
	}

	baseExecutable := filepath.Base(params.ExecutablePath)
	if (baseExecutable == "python" || baseExecutable == "python.exe" || baseExecutable == "python3" || baseExecutable == "python3.exe") && len(params.Args) > 0 && params.Args[0] != "-u" {
		// Check if '-u' is anywhere in args
		hasUFlag := false
		for _, arg := range params.Args {
			if arg == "-u" {
				hasUFlag = true
				break
			}
		}
		if !hasUFlag {
			params.Logger.Warn().Msg("Consider adding the '-u' flag to your python script arguments to prevent output buffering.")
		}
	}

	if err := params.createFiles(); err != nil {
		return nil, err
	}

	var err error
	params.ExecutablePath, err = exec.LookPath(params.ExecutablePath)
	if err != nil {
		return nil, err
	}

	stdoutReader, stdoutWriter, err := createPipe()
	if err != nil {
		return nil, err
	}
	stderrReader, stderrWriter, err := createPipe()
	if err != nil {
		return nil, err
	}

	stdoutTimestampWriter := newTimestampWriter(params.logFile, "")
	stderrTimestampWriter := newTimestampWriter(params.errFile, "")

	go stdoutTimestampWriter.processLogs(stdoutReader)
	go stderrTimestampWriter.processLogs(stderrReader)

	cmd := exec.Command(params.ExecutablePath, params.Args...)
	cmd.Dir = params.Cwd
	cmd.Env = os.Environ()
	cmd.Env = append(cmd.Env, "PYTHONUNBUFFERED=1")
	cmd.Stdin = params.nullFile
	cmd.Stdout = stdoutWriter
	cmd.Stderr = stderrWriter
	// SysProcAttr is OS-specific. Setpgid is for Unix-like systems.
	if runtime.GOOS != "windows" {
		cmd.SysProcAttr = &syscall.SysProcAttr{
			Setpgid: true,
		}
	}

	if err := cmd.Start(); err != nil {
		stdoutWriter.Close()
		stderrWriter.Close()
		params.nullFile.Close()
		params.logFile.Close()
		params.errFile.Close()
		return nil, err
	}

	go func() {
		cmd.Wait()

		stdoutWriter.Close()
		stderrWriter.Close()
		params.nullFile.Close()
		params.logFile.Close()
		params.errFile.Close()
	}()

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
		LogFilePath:    params.LogFilePath,
		ErrFilePath:    params.ErrFilePath,
		PidFilePath:    params.PidPilePath,
		AutoRestart:    params.AutoRestart,
		CronRestart:    params.CronRestart,
	}

	return rpcProcess, nil
}
