package process

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/dunstorm/pm2-go/internal/utils"
	pb "github.com/dunstorm/pm2-go/proto"
	"github.com/rs/zerolog"
)

type SpawnParams struct {
	Name           string            `json:"name"`
	ExecutablePath string            `json:"executablePath"`
	Args           []string          `json:"args"`
	Cwd            string            `json:"cwd"`
	Env            map[string]string `json:"env"`
	AutoRestart    bool              `json:"autorestart"`
	CronRestart    string            `json:"cron_restart"`
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
		params.Name = defaultProcessName(params.ExecutablePath)
	}

	if params.Logger == nil {
		params.Logger = utils.NewLogger()
	}

	if params.Cwd == "" {
		params.Cwd, _ = os.Getwd()
	}

	fileName := processFileName(params.Name)
	params.PidPilePath = filepath.Join(utils.GetMainDirectory(), "pids", fmt.Sprintf("%s.pid", fileName))
	params.LogFilePath = filepath.Join(utils.GetMainDirectory(), "logs", fmt.Sprintf("%s-out.log", fileName))
	params.ErrFilePath = filepath.Join(utils.GetMainDirectory(), "logs", fmt.Sprintf("%s-err.log", fileName))

	return nil
}

func defaultProcessName(executablePath string) string {
	return strings.ToLower(filepath.Base(executablePath))
}

func processFileName(name string) string {
	replacer := strings.NewReplacer("/", "-", "\\", "-")
	fileName := strings.ToLower(strings.TrimSpace(name))
	fileName = replacer.Replace(fileName)
	fileName = strings.Trim(fileName, ".- ")
	if fileName == "" {
		return "process"
	}
	return fileName
}

func isPythonExecutable(executablePath string) bool {
	base := strings.ToLower(filepath.Base(executablePath))
	if base == "python" {
		return true
	}
	if !strings.HasPrefix(base, "python") {
		return false
	}

	suffix := strings.TrimPrefix(base, "python")
	for _, part := range strings.Split(suffix, ".") {
		if part == "" {
			return false
		}
		for _, char := range part {
			if char < '0' || char > '9' {
				return false
			}
		}
	}
	return true
}

func commandEnvironment(base []string, overrides map[string]string, pythonExecutable bool) []string {
	environment := utils.EnvironmentMap(base)
	if pythonExecutable {
		if _, hasOverride := overrides["PYTHONUNBUFFERED"]; !hasOverride {
			if _, exists := environment["PYTHONUNBUFFERED"]; !exists {
				environment["PYTHONUNBUFFERED"] = "1"
			}
		}
	}
	for key, value := range overrides {
		if key == "" {
			continue
		}
		environment[key] = value
	}
	return utils.EnvironmentSlice(environment)
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

func SpawnNewProcess(params SpawnParams) (*pb.Process, error) {
	if err := params.fillDefaults(); err != nil {
		return nil, err
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
	cmd.Env = commandEnvironment(os.Environ(), params.Env, isPythonExecutable(params.ExecutablePath))
	cmd.Stdin = params.nullFile
	cmd.Stdout = stdoutWriter
	cmd.Stderr = stderrWriter
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setpgid: true,
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
		Env:            utils.CloneStringMap(params.Env),
	}

	return rpcProcess, nil
}
