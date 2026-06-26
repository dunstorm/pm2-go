package cli

import (
	"encoding/json"
	"os"

	pb "github.com/dunstorm/pm2-go/proto"
)

type processStatusView struct {
	Status    string `json:"status"`
	StartedAt string `json:"started_at,omitempty"`
	Uptime    string `json:"uptime"`
	Restarts  int32  `json:"restarts"`
	CPU       string `json:"cpu"`
	Memory    string `json:"memory"`
	ParentPID int32  `json:"parent_pid"`
}

type processView struct {
	ID                       int32             `json:"id"`
	Name                     string            `json:"name"`
	PID                      int32             `json:"pid"`
	Args                     []string          `json:"args,omitempty"`
	ExecutablePath           string            `json:"executable_path"`
	Cwd                      string            `json:"cwd,omitempty"`
	PIDFilePath              string            `json:"pid_file_path,omitempty"`
	LogFilePath              string            `json:"log_file_path,omitempty"`
	ErrFilePath              string            `json:"err_file_path,omitempty"`
	AutoRestart              bool              `json:"autorestart"`
	CronRestart              string            `json:"cron_restart,omitempty"`
	NextStartAt              string            `json:"next_start_at,omitempty"`
	RestartAt                string            `json:"restart_at,omitempty"`
	Env                      map[string]string `json:"env,omitempty"`
	MaxRestarts              int32             `json:"max_restarts,omitempty"`
	MinUptimeMS              int32             `json:"min_uptime,omitempty"`
	RestartDelayMS           int32             `json:"restart_delay,omitempty"`
	ExpBackoffRestartDelayMS int32             `json:"exp_backoff_restart_delay,omitempty"`
	MaxMemoryRestart         int64             `json:"max_memory_restart,omitempty"`
	Status                   string            `json:"status,omitempty"`
	ProcStatus               processStatusView `json:"proc_status"`
}

func newProcessView(process *pb.Process) processView {
	view := processView{
		ID:                       process.Id,
		Name:                     process.Name,
		PID:                      process.Pid,
		Args:                     process.Args,
		ExecutablePath:           process.ExecutablePath,
		Cwd:                      process.Cwd,
		PIDFilePath:              process.PidFilePath,
		LogFilePath:              process.LogFilePath,
		ErrFilePath:              process.ErrFilePath,
		AutoRestart:              process.AutoRestart,
		CronRestart:              process.CronRestart,
		Env:                      process.Env,
		MaxRestarts:              process.MaxRestarts,
		MinUptimeMS:              process.MinUptimeMs,
		RestartDelayMS:           process.RestartDelayMs,
		ExpBackoffRestartDelayMS: process.ExpBackoffRestartDelayMs,
		MaxMemoryRestart:         process.MaxMemoryRestart,
	}
	if process.NextStartAt != nil {
		view.NextStartAt = process.NextStartAt.AsTime().Format("2006-01-02T15:04:05Z07:00")
	}
	if process.RestartAt != nil {
		view.RestartAt = process.RestartAt.AsTime().Format("2006-01-02T15:04:05Z07:00")
	}
	if process.ProcStatus != nil {
		view.Status = process.ProcStatus.Status
		view.ProcStatus = processStatusView{
			Status:    process.ProcStatus.Status,
			Uptime:    process.ProcStatus.Uptime.AsDuration().String(),
			Restarts:  process.ProcStatus.Restarts,
			CPU:       process.ProcStatus.Cpu,
			Memory:    process.ProcStatus.Memory,
			ParentPID: process.ProcStatus.ParentPid,
		}
		if process.ProcStatus.StartedAt != nil {
			view.ProcStatus.StartedAt = process.ProcStatus.StartedAt.AsTime().Format("2006-01-02T15:04:05Z07:00")
		}
	}
	return view
}

func writeJSON(value interface{}) {
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(value); err != nil {
		master.GetLogger().Fatal().Msg(err.Error())
	}
}
