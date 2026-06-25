package process

import (
	"testing"

	"github.com/dunstorm/pm2-go/internal/utils"
	"github.com/rs/zerolog"
)

func TestSpawnNewProcess(t *testing.T) {
	zerolog.SetGlobalLevel(zerolog.Disabled)

	spawnedProcess, err := SpawnNewProcess(SpawnParams{
		ExecutablePath: "python3",
		Args:           []string{"../../examples/test.py"},
	})
	if err != nil {
		t.Error(err)
		return
	}

	if spawnedProcess == nil {
		t.Fatal("process is nil")
	}

	processFound, running := utils.IsProcessRunning(spawnedProcess.Pid)
	if !running {
		t.Fatal("process is not running")
	}
	processFound.Kill()
}
