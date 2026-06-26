package server

import (
	"context"
	"errors"
	"os"
	"sort"
	"time"

	"github.com/dunstorm/pm2-go/internal/utils"
	pb "github.com/dunstorm/pm2-go/proto"
)

type persistedProcess struct {
	Name                     string            `json:"name"`
	Args                     []string          `json:"args,omitempty"`
	Scripts                  []string          `json:"scripts,omitempty"`
	ExecutablePath           string            `json:"executable_path"`
	AutoRestart              bool              `json:"autorestart"`
	Cwd                      string            `json:"cwd,omitempty"`
	CronRestart              string            `json:"cron_restart,omitempty"`
	Env                      map[string]string `json:"env,omitempty"`
	MaxRestarts              int32             `json:"max_restarts,omitempty"`
	MinUptimeMS              int32             `json:"min_uptime,omitempty"`
	RestartDelayMS           int32             `json:"restart_delay,omitempty"`
	ExpBackoffRestartDelayMS int32             `json:"exp_backoff_restart_delay,omitempty"`
	MaxMemoryRestart         int64             `json:"max_memory_restart,omitempty"`
	HealthCheckURL           string            `json:"health_check_url,omitempty"`
	HealthCheckIntervalMS    int32             `json:"health_check_interval,omitempty"`
	HealthCheckTimeoutMS     int32             `json:"health_check_timeout,omitempty"`
	Watch                    bool              `json:"watch,omitempty"`
	WatchPaths               []string          `json:"watch_paths,omitempty"`
	WatchIntervalMS          int32             `json:"watch_interval,omitempty"`
}

func (api *Handler) restoreState() {
	stateFile := utils.GetStateFilePath()
	if _, err := os.Stat(stateFile); errors.Is(err, os.ErrNotExist) {
		return
	}

	var savedProcesses []persistedProcess
	if err := utils.LoadObject(stateFile, &savedProcesses); err != nil {
		api.logger.Error().Err(err).Str("path", stateFile).Msg("Failed to load process state")
		return
	}

	for _, savedProcess := range savedProcesses {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		resp, err := api.SpawnProcess(ctx, savedProcess.spawnRequest())
		cancel()
		if err != nil {
			api.logger.Error().Err(err).Str("name", savedProcess.Name).Msg("Failed to restore process")
			continue
		}
		if resp == nil || !resp.Success {
			api.logger.Error().Str("name", savedProcess.Name).Msg("Failed to restore process")
		}
	}
}

func (api *Handler) persistStateLocked() {
	stateFile := utils.GetStateFilePath()
	if err := utils.SaveObject(stateFile, api.persistedProcessesLocked()); err != nil {
		api.logger.Error().Err(err).Str("path", stateFile).Msg("Failed to save process state")
	}
}

func (api *Handler) persistedProcessesLocked() []persistedProcess {
	ids := make([]int32, 0, len(api.databaseById))
	for id := range api.databaseById {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool {
		return ids[i] < ids[j]
	})

	processes := make([]persistedProcess, 0, len(ids))
	for _, id := range ids {
		process := api.databaseById[id]
		if !shouldPersistProcess(process) {
			continue
		}
		processes = append(processes, newPersistedProcess(process))
	}
	return processes
}

func shouldPersistProcess(process *pb.Process) bool {
	return process != nil &&
		process.ProcStatus != nil &&
		isRunningState(process.ProcStatus.Status) &&
		process.Pid != 0 &&
		!process.StopSignal
}

func newPersistedProcess(process *pb.Process) persistedProcess {
	return persistedProcess{
		Name:                     process.Name,
		Args:                     process.Args,
		Scripts:                  process.Scripts,
		ExecutablePath:           process.ExecutablePath,
		AutoRestart:              process.AutoRestart,
		Cwd:                      process.Cwd,
		CronRestart:              process.CronRestart,
		Env:                      utils.CloneStringMap(process.Env),
		MaxRestarts:              process.MaxRestarts,
		MinUptimeMS:              process.MinUptimeMs,
		RestartDelayMS:           process.RestartDelayMs,
		ExpBackoffRestartDelayMS: process.ExpBackoffRestartDelayMs,
		MaxMemoryRestart:         process.MaxMemoryRestart,
		HealthCheckURL:           process.HealthCheckUrl,
		HealthCheckIntervalMS:    process.HealthCheckIntervalMs,
		HealthCheckTimeoutMS:     process.HealthCheckTimeoutMs,
		Watch:                    process.Watch,
		WatchPaths:               process.WatchPaths,
		WatchIntervalMS:          process.WatchIntervalMs,
	}
}

func (process persistedProcess) spawnRequest() *pb.SpawnProcessRequest {
	return &pb.SpawnProcessRequest{
		Name:                     process.Name,
		Args:                     process.Args,
		Scripts:                  process.Scripts,
		ExecutablePath:           process.ExecutablePath,
		AutoRestart:              process.AutoRestart,
		Cwd:                      process.Cwd,
		CronRestart:              process.CronRestart,
		Env:                      process.Env,
		MaxRestarts:              process.MaxRestarts,
		MinUptimeMs:              process.MinUptimeMS,
		RestartDelayMs:           process.RestartDelayMS,
		ExpBackoffRestartDelayMs: process.ExpBackoffRestartDelayMS,
		MaxMemoryRestart:         process.MaxMemoryRestart,
		HealthCheckUrl:           process.HealthCheckURL,
		HealthCheckIntervalMs:    process.HealthCheckIntervalMS,
		HealthCheckTimeoutMs:     process.HealthCheckTimeoutMS,
		Watch:                    process.Watch,
		WatchPaths:               process.WatchPaths,
		WatchIntervalMs:          process.WatchIntervalMS,
	}
}
