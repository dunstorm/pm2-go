package utils

import (
	"os"
	"syscall"
)

func SignalProcessGroup(process *os.Process, signal os.Signal) error {
	if process == nil {
		return nil
	}
	syscallSignal, ok := signal.(syscall.Signal)
	if !ok || process.Pid <= 0 {
		return process.Signal(signal)
	}
	if err := syscall.Kill(-process.Pid, syscallSignal); err != nil {
		return process.Signal(signal)
	}
	return nil
}

func KillProcessGroup(process *os.Process) error {
	return SignalProcessGroup(process, syscall.SIGKILL)
}
