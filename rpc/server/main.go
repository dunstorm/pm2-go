package server

import (
	"net"
	"net/http"
	"net/rpc"
	"os"
	"sync"
	"time"

	"github.com/dunstorm/pm2-go/shared"
	"github.com/dunstorm/pm2-go/utils"
	"github.com/rs/zerolog"
)

func New() {
	logger := zerolog.New(os.Stderr).With().Timestamp().Logger()
	handler := &API{
		logger:   &logger,
		database: make([]shared.Process, 0),
	}

	// Publish the receivers methods
	err := rpc.Register(handler)
	if err != nil {
		handler.logger.Fatal().Msgf("Format of service API isn't correct. %s", err)
		return
	}

	// Register a HTTP handler
	rpc.HandleHTTP()

	// Listen to TPC connections on port 9001
	listener, e := net.Listen("tcp", "localhost:9001")
	if e != nil {
		handler.logger.Fatal().Msgf("Listen error: %s", e)
		return
	}
	handler.logger.Info().Msgf("Serving RPC server on port %d", 9001)

	var wg sync.WaitGroup

	// sync process
	syncProcess := func(p shared.Process) {
		if p.ProcStatus.Status == "online" || p.ProcStatus.Status == "stopping" {
			if _, ok := utils.IsProcessRunning(p.Pid); !ok {
				p.UpdateUptime()
				p.ResetPid()
				p.UpdateStatus("stopped")

				handler.mu.Lock()
				handler.database[p.ID] = p
				handler.mu.Unlock()

				if p.AutoRestart && !p.GetToStop() {
					p.IncreaseRestarts()
					newProcess := shared.SpawnNewProcess(shared.SpawnParams{
						Name:           p.Name,
						Args:           p.Args,
						ExecutablePath: p.ExecutablePath,
						AutoRestart:    p.AutoRestart,
						Cwd:            p.Cwd,
						Logger:         handler.logger,
						Scripts:        p.Scripts,
					})

					p.Pid = newProcess.Pid
					p.UpdateStatus("online")
					p.SetProcess(newProcess.GetProcess())
					p.InitUptime()
					p.InitStartedAt()
					p.UpdateProcess(p.Pid)

					go p.GetProcess().Wait()
					handler.mu.Lock()
					handler.database[p.ID] = p
					handler.mu.Unlock()
				}
			} else {
				p.UpdateUptime()
			}
		}
		wg.Done()
	}

	go func() {
		for {
			for _, p := range handler.database {
				wg.Add(1)
				go syncProcess(p)
			}
			wg.Wait()
			time.Sleep(500 * time.Millisecond)
		}
	}()

	// Start accept incoming HTTP connections
	err = http.Serve(listener, nil)
	if err != nil {
		handler.logger.Fatal().Msgf("Error serving: %s", err)
	}
}
