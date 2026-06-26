package __

import (
	"errors"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/aptible/supercronic/cronexpr"
	status "google.golang.org/grpc/status"
	durationpb "google.golang.org/protobuf/types/known/durationpb"
	timestamppb "google.golang.org/protobuf/types/known/timestamppb"
)

type CPUMemoryStats struct {
	CPU         string
	Memory      string
	MemoryBytes int64
}

func (p *Process) UpdateStatus(status string) {
	p.ProcStatus.Status = status
}

func (p *Process) SetStatus(status string) {
	p.ProcStatus.Status = status
}

func (p *Process) SetStopSignal(stopSignal bool) {
	p.StopSignal = stopSignal
}

func (p *Process) ResetPid() {
	p.Pid = 0
	p.ProcStatus.ParentPid = 0
}

func (p *Process) UpdateUptime() {
	p.ProcStatus.Uptime = durationpb.New(time.Since(p.ProcStatus.StartedAt.AsTime()).Truncate(time.Second))
}

func (p *Process) InitStartedAt() {
	p.ProcStatus.StartedAt = timestamppb.New(time.Now())
}

func (p *Process) InitUptime() {
	p.ProcStatus.Uptime = durationpb.New(0)
}

func (p *Process) IncreaseRestarts() {
	p.ProcStatus.Restarts++
}

func (p *Process) ResetRestarts() {
	p.ProcStatus.Restarts = 0
}

func (p *Process) ResetCPUMemory() {
	p.ProcStatus.Cpu = "0.0%"
	p.ProcStatus.Memory = "0.0MB"
}

func (p *Process) UpdateCPUMemory() {
	_, _ = p.UpdateCPUMemoryStats()
}

func (p *Process) ReadCPUMemoryStats() (CPUMemoryStats, error) {
	if p.Pid == 0 {
		return CPUMemoryStats{
			CPU:    "0.0%",
			Memory: "0.0MB",
		}, nil
	}
	// launch command and read content
	cmd := exec.Command("ps", "-p", fmt.Sprintf("%d", p.Pid), "-o", "pcpu,rss")
	output, err := cmd.Output()
	if err != nil {
		return CPUMemoryStats{}, err
	}
	// output separator can be multiple whitespaces
	// fix: error `parsing "": invalid syntax` in `strconv.ParseFloat`
	outputLines := strings.Split(strings.TrimSpace(string(output)), "\n")
	if len(outputLines) < 2 {
		return CPUMemoryStats{}, errors.New("missing process metrics")
	}
	outputSplit := strings.Fields(outputLines[1])
	if len(outputSplit) < 2 {
		return CPUMemoryStats{}, errors.New("invalid process metrics")
	}

	// convert string to float
	memory, err := strconv.ParseFloat(outputSplit[1], 64)
	if err != nil {
		return CPUMemoryStats{}, err
	}
	memoryBytes := int64(memory * 1024)
	return CPUMemoryStats{
		CPU:         fmt.Sprint(outputSplit[0], "%"),
		Memory:      fmt.Sprintf("%.1fMB", memory/1024),
		MemoryBytes: memoryBytes,
	}, nil
}

func (p *Process) UpdateCPUMemoryStats() (int64, error) {
	if p.Pid == 0 {
		return 0, nil
	}

	stats, err := p.ReadCPUMemoryStats()
	if err != nil {
		return 0, err
	}
	p.ProcStatus.Cpu = stats.CPU
	p.ProcStatus.Memory = stats.Memory
	return stats.MemoryBytes, nil
}

func (p *Process) UpdateNextStartAt() error {
	if p.CronRestart != "" {
		expr, err := cronexpr.Parse(p.CronRestart)
		if err != nil {
			p.CronRestart = ""
			return status.Errorf(400, "invalid cron expression: %v", err)
		}
		p.NextStartAt = timestamppb.New(expr.Next(time.Now()))
	}
	return nil
}
