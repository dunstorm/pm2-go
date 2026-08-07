package process

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/dunstorm/pm2-go/internal/logstore"
	"github.com/dunstorm/pm2-go/internal/utils"
	pb "github.com/dunstorm/pm2-go/proto"
	"github.com/rs/zerolog"
)

var logCaptureDrainTimeout = 2 * time.Second
var spawnedProcessWaitRetention = 30 * time.Second
var spawnedProcessWaits sync.Map

type SpawnParams struct {
	Name                     string            `json:"name"`
	ExecutablePath           string            `json:"executablePath"`
	Args                     []string          `json:"args"`
	Cwd                      string            `json:"cwd"`
	Env                      map[string]string `json:"env"`
	AutoRestart              bool              `json:"autorestart"`
	CronRestart              string            `json:"cron_restart"`
	MaxRestarts              int32             `json:"max_restarts"`
	MinUptimeMS              int32             `json:"min_uptime"`
	RestartDelayMS           int32             `json:"restart_delay"`
	ExpBackoffRestartDelayMS int32             `json:"exp_backoff_restart_delay"`
	MaxMemoryRestart         int64             `json:"max_memory_restart"`
	HealthCheckURL           string            `json:"health_check_url"`
	HealthCheckIntervalMS    int32             `json:"health_check_interval"`
	HealthCheckTimeoutMS     int32             `json:"health_check_timeout"`
	Watch                    bool              `json:"watch"`
	WatchPaths               []string          `json:"watch_paths"`
	WatchIntervalMS          int32             `json:"watch_interval"`
	Logger                   *zerolog.Logger

	PidPilePath         string `json:"-"`
	LogFilePath         string `json:"-"`
	ErrFilePath         string `json:"-"`
	CombinedLogFilePath string `json:"-"`

	logFile      *managedLogFile
	errFile      *managedLogFile
	combinedFile *managedLogFile
	nullFile     *os.File
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
	params.CombinedLogFilePath = logstore.CombinedPath(params.LogFilePath)

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
	if overrides != nil {
		environment = utils.CloneStringMap(overrides)
		if environment == nil {
			environment = make(map[string]string)
		}
	}
	if pythonExecutable {
		if _, hasOverride := overrides["PYTHONUNBUFFERED"]; !hasOverride {
			if _, exists := environment["PYTHONUNBUFFERED"]; !exists {
				environment["PYTHONUNBUFFERED"] = "1"
			}
		}
	}
	return utils.EnvironmentSlice(environment)
}

func (params *SpawnParams) createFiles() error {
	var err error
	params.logFile, err = openManagedLogFile(params.LogFilePath)
	if err != nil {
		return err
	}

	params.errFile, err = openManagedLogFile(params.ErrFilePath)
	if err != nil {
		params.closeFiles()
		return err
	}

	params.combinedFile, err = openManagedLogFile(params.CombinedLogFilePath)
	if err != nil {
		params.closeFiles()
		return err
	}
	if params.nullFile, err = os.Open(os.DevNull); err != nil {
		params.closeFiles()
		return err
	}
	return nil
}

func (params *SpawnParams) closeFiles() {
	for _, file := range []*managedLogFile{params.logFile, params.errFile, params.combinedFile} {
		file.close()
	}
	if params.nullFile != nil {
		_ = params.nullFile.Close()
	}
}

func waitForLogCapture(logsWG *sync.WaitGroup, readers ...*os.File) {
	done := make(chan struct{})
	go func() {
		logsWG.Wait()
		close(done)
	}()

	select {
	case <-done:
		return
	case <-time.After(logCaptureDrainTimeout):
		for _, reader := range readers {
			if reader != nil {
				_ = reader.Close()
			}
		}
		<-done
	}
}

func WaitForSpawnedProcess(pid int32, timeout time.Duration) (bool, bool) {
	value, ok := spawnedProcessWaits.Load(pid)
	if !ok {
		return false, false
	}

	done, ok := value.(chan struct{})
	if !ok {
		return false, false
	}

	if timeout <= 0 {
		select {
		case <-done:
			deleteSpawnedProcessWait(pid, done)
			return true, true
		default:
			return false, true
		}
	}

	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case <-done:
		deleteSpawnedProcessWait(pid, done)
		return true, true
	case <-timer.C:
		return false, true
	}
}

func deleteSpawnedProcessWait(pid int32, waitDone chan struct{}) {
	spawnedProcessWaits.CompareAndDelete(pid, waitDone)
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
		params.closeFiles()
		return nil, err
	}

	stdoutReader, stdoutWriter, err := createPipe()
	if err != nil {
		params.closeFiles()
		return nil, err
	}
	stderrReader, stderrWriter, err := createPipe()
	if err != nil {
		_ = stdoutReader.Close()
		_ = stdoutWriter.Close()
		params.closeFiles()
		return nil, err
	}

	combinedSink := newCombinedLogSink(params.combinedFile)
	var logsWG sync.WaitGroup
	logsWG.Add(1)
	go func() {
		defer logsWG.Done()
		defer stdoutReader.Close()
		defer stderrReader.Close()
		processStreamLogs(stdoutReader, stderrReader, params.logFile, params.errFile, combinedSink)
	}()

	closeLogCapture := func() {
		_ = stdoutWriter.Close()
		_ = stderrWriter.Close()
		waitForLogCapture(&logsWG, stdoutReader, stderrReader)
		combinedSink.close()
		params.closeFiles()
	}

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
		closeLogCapture()
		return nil, err
	}

	pid := int32(cmd.Process.Pid)
	waitDone := make(chan struct{})
	spawnedProcessWaits.Store(pid, waitDone)
	go func() {
		_ = cmd.Wait()
		close(waitDone)
		time.AfterFunc(spawnedProcessWaitRetention, func() {
			deleteSpawnedProcessWait(pid, waitDone)
		})
		closeLogCapture()
	}()

	params.Logger.Info().Msgf("[%s] ✓", params.Name)

	if err := utils.WritePidToFile(params.PidPilePath, cmd.Process.Pid); err != nil {
		params.Logger.Error().Err(err).Msg("Failed to write process pid file")
		_ = utils.KillProcessGroup(cmd.Process)
		return nil, err
	}

	rpcProcess := &pb.Process{
		Name:                     params.Name,
		ExecutablePath:           params.ExecutablePath,
		Pid:                      pid,
		Args:                     params.Args,
		Cwd:                      params.Cwd,
		LogFilePath:              params.LogFilePath,
		ErrFilePath:              params.ErrFilePath,
		PidFilePath:              params.PidPilePath,
		AutoRestart:              params.AutoRestart,
		CronRestart:              params.CronRestart,
		Env:                      utils.CloneStringMap(params.Env),
		MaxRestarts:              params.MaxRestarts,
		MinUptimeMs:              params.MinUptimeMS,
		RestartDelayMs:           params.RestartDelayMS,
		ExpBackoffRestartDelayMs: params.ExpBackoffRestartDelayMS,
		MaxMemoryRestart:         params.MaxMemoryRestart,
		HealthCheckUrl:           params.HealthCheckURL,
		HealthCheckIntervalMs:    params.HealthCheckIntervalMS,
		HealthCheckTimeoutMs:     params.HealthCheckTimeoutMS,
		Watch:                    params.Watch,
		WatchPaths:               params.WatchPaths,
		WatchIntervalMs:          params.WatchIntervalMS,
	}

	return rpcProcess, nil
}
