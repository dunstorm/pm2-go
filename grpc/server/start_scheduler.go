package server

import (
	"os"
	"sync"
	"time"

	pb "github.com/dunstorm/pm2-go/proto"
	"github.com/dunstorm/pm2-go/shared"
	"github.com/dunstorm/pm2-go/utils"
)

func updateProcessMap(handler *Handler, processId int32, p *os.Process) {
	handler.processes[processId] = p
}

func startScheduler(handler *Handler) {
	var wg sync.WaitGroup

	// sync process
	syncProcess := func(p *pb.Process) {
		if p.ProcStatus.Status == "online" || p.ProcStatus.Status == "stopping" {
			if _, ok := utils.IsProcessRunning(p.Pid); !ok {
				p.UpdateUptime()
				p.ResetPid()
				p.UpdateStatus("stopped")
				updateProcessMap(handler, p.Id, nil)

				handler.mu.Lock()
				defer handler.mu.Unlock()

				if p.AutoRestart && !p.GetToStop() {
					p.IncreaseRestarts()
					newProcess, err := shared.SpawnNewProcess(shared.SpawnParams{
						Name:           p.Name,
						Args:           p.Args,
						ExecutablePath: p.ExecutablePath,
						AutoRestart:    p.AutoRestart,
						Cwd:            p.Cwd,
						Logger:         handler.logger,
						Scripts:        p.Scripts,
					})
					if err != nil {
						p.AutoRestart = false
						p.SetToStop(true)
						p.SetStatus("stopped")
						p.Pid = 0
						updateProcessMap(handler, p.Id, nil)

						handler.logger.Error().Msgf("Error while restarting process %s: %s", p.Name, err)
					}

					p.Pid = newProcess.Pid
					p.ProcStatus.ParentPid = int32(os.Getpid())
					p.UpdateStatus("online")

					// set new process
					process, _ := utils.GetProcess(p.Pid)
					updateProcessMap(handler, p.Id, process)

					p.InitUptime()
					p.InitStartedAt()

					go process.Wait()
				}
			} else {
				p.UpdateUptime()
			}
		}
		wg.Done()
	}

	go func() {
		for {
			for _, p := range handler.databaseById {
				wg.Add(1)
				go syncProcess(p)
			}
			wg.Wait()
			time.Sleep(500 * time.Millisecond)
		}
	}()
}
