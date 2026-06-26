package __

import "testing"

func TestUpdateCPUMemoryStatsSkipsStoppedProcess(t *testing.T) {
	process := &Process{
		Pid: 0,
		ProcStatus: &ProcStatus{
			Cpu:    "0.0%",
			Memory: "0.0MB",
		},
	}

	memoryBytes, err := process.UpdateCPUMemoryStats()
	if err != nil {
		t.Fatalf("update CPU/memory stats: %v", err)
	}
	if memoryBytes != 0 {
		t.Fatalf("expected zero memory bytes for stopped process, got %d", memoryBytes)
	}
	if process.ProcStatus.Cpu != "0.0%" {
		t.Fatalf("expected stopped process CPU to stay 0.0%%, got %q", process.ProcStatus.Cpu)
	}
	if process.ProcStatus.Memory != "0.0MB" {
		t.Fatalf("expected stopped process memory to stay 0.0MB, got %q", process.ProcStatus.Memory)
	}
}
