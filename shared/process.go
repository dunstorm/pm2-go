package shared

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

type ProcStatus struct {
	Status    string
	StartedAt time.Time
	Uptime    time.Duration
	Restarts  int
	CPU       string
	Memory    string
}

// make a Process type
type Process struct {
	ID             int
	Name           string
	Args           []string
	ExecutablePath string
	Pid            int
	AutoRestart    bool
	PidFilePath    string
	LogFilePath    string
	ErrFilePath    string
	ProcStatus     *ProcStatus

	toStop  bool
	process *os.Process
}

func (p *Process) UpdateStatus(status string) {
	p.ProcStatus.Status = status
}

func (p *Process) GetProcess() *os.Process {
	return p.process
}

func (p *Process) SetProcess(process *os.Process) {
	p.process = process
}

func (p *Process) UpdateProcess(pid int) {
	process, err := os.FindProcess(pid)
	if err == nil {
		p.SetProcess(process)
	}
}

func (p *Process) SetToStop(toStop bool) {
	p.toStop = toStop
}

func (p *Process) GetToStop() bool {
	return p.toStop
}

func (p *Process) UpdatePid(pid int) {
	p.Pid = pid
	p.process, _ = os.FindProcess(pid)
}

func (p *Process) ResetPid() {
	p.Pid = -1
}

func (p *Process) UpdateUptime() {
	p.ProcStatus.Uptime = time.Since(p.ProcStatus.StartedAt).Truncate(time.Second)
}

func (p *Process) InitStartedAt() {
	p.ProcStatus.StartedAt = time.Now()
}

func (p *Process) InitUptime() {
	p.ProcStatus.Uptime = 0
}

func (p *Process) IncreaseRestarts() {
	p.ProcStatus.Restarts++
}

func (p *Process) ResetRestarts() {
	p.ProcStatus.Restarts = 0
}

func (p *Process) ResetCPUMemory() {
	p.ProcStatus.CPU = "0.0%"
	p.ProcStatus.Memory = "0.0MB"
}

func (p *Process) UpdateCPUMemory() {
	if p.Pid == 0 {
		return
	}
	// launch command and read content
	cmd := exec.Command("ps", "-p", fmt.Sprintf("%d", p.Pid), "-o", "pcpu, rss")
	output, err := cmd.Output()
	if err != nil {
		return
	}
	outputSplit := strings.Split(strings.TrimSpace(strings.Split(string(output), "\n")[1]), " ")

	p.ProcStatus.CPU = fmt.Sprint(outputSplit[0], "%")

	// convert string to float
	memory, _ := strconv.ParseFloat(outputSplit[1], 64)
	memory = memory / 1024
	p.ProcStatus.Memory = fmt.Sprintf("%.1fMB", memory)
}
