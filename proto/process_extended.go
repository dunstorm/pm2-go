package __

import (
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
	if p.Pid == 0 {
		return
	}
	// launch command and read content
	cmd := exec.Command("ps", "-p", fmt.Sprintf("%d", p.Pid), "-o", "pcpu,rss")
	output, err := cmd.Output()
	if err != nil {
		return
	}
	// output separator can be multiple whitespaces
	// fix: error `parsing "": invalid syntax` in `strconv.ParseFloat`
	outputSplit := strings.Fields(strings.TrimSpace(strings.Split(string(output), "\n")[1]))

	p.ProcStatus.Cpu = fmt.Sprint(outputSplit[0], "%")

	// convert string to float
	memory, _ := strconv.ParseFloat(outputSplit[1], 64)
	memory = memory / 1024
	p.ProcStatus.Memory = fmt.Sprintf("%.1fMB", memory)
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
