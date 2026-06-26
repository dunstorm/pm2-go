package web

import (
	"time"

	pb "github.com/dunstorm/pm2-go/proto"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type processView struct {
	ID                int32    `json:"id"`
	Name              string   `json:"name"`
	Status            string   `json:"status"`
	PID               int32    `json:"pid"`
	ParentPID         int32    `json:"parent_pid"`
	CPU               string   `json:"cpu"`
	Memory            string   `json:"memory"`
	Uptime            string   `json:"uptime"`
	Restarts          int32    `json:"restarts"`
	ExecutablePath    string   `json:"executable_path"`
	Args              []string `json:"args"`
	Cwd               string   `json:"cwd"`
	AutoRestart       bool     `json:"auto_restart"`
	HealthConfigured  bool     `json:"health_configured"`
	Watch             bool     `json:"watch"`
	NextStartAt       string   `json:"next_start_at,omitempty"`
	LastHealthCheckAt string   `json:"last_health_check_at,omitempty"`
	LastWatchCheckAt  string   `json:"last_watch_check_at,omitempty"`
}

func processViews(processes []*pb.Process) []processView {
	views := make([]processView, 0, len(processes))
	for _, process := range processes {
		if process == nil {
			continue
		}
		views = append(views, newProcessView(process))
	}
	return views
}

func newProcessView(process *pb.Process) processView {
	view := processView{
		ID:                process.Id,
		Name:              process.Name,
		PID:               process.Pid,
		ExecutablePath:    process.ExecutablePath,
		Args:              append([]string(nil), process.Args...),
		Cwd:               process.Cwd,
		AutoRestart:       process.AutoRestart,
		HealthConfigured:  process.HealthCheckUrl != "",
		Watch:             process.Watch,
		NextStartAt:       timestampString(process.NextStartAt),
		LastHealthCheckAt: timestampString(process.LastHealthCheckAt),
		LastWatchCheckAt:  timestampString(process.LastWatchCheckAt),
	}

	if process.ProcStatus != nil {
		view.Status = process.ProcStatus.Status
		view.ParentPID = process.ProcStatus.ParentPid
		view.CPU = process.ProcStatus.Cpu
		view.Memory = process.ProcStatus.Memory
		view.Restarts = process.ProcStatus.Restarts
		view.Uptime = durationString(process.ProcStatus.Uptime)
	}
	if view.Status == "" {
		view.Status = "unknown"
	}
	if view.CPU == "" {
		view.CPU = "0.0%"
	}
	if view.Memory == "" {
		view.Memory = "0.0MB"
	}
	return view
}

func timestampString(timestamp *timestamppb.Timestamp) string {
	if timestamp == nil {
		return ""
	}
	value := timestamp.AsTime()
	if value.IsZero() {
		return ""
	}
	return value.UTC().Format(time.RFC3339)
}

func durationString(duration *durationpb.Duration) string {
	if duration == nil {
		return "0s"
	}
	value := duration.AsDuration()
	if value <= 0 {
		return "0s"
	}
	return value.Round(time.Second).String()
}
