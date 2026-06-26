package server

import (
	"os"
	"strconv"
	"sync"
	"time"

	processrunner "github.com/dunstorm/pm2-go/internal/process"
	"github.com/dunstorm/pm2-go/internal/utils"
	pb "github.com/dunstorm/pm2-go/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func updateProcessMap(handler *Handler, processId int32, p *os.Process) {
	handler.processes[processId] = p
}

func restartProcess(handler *Handler, p *pb.Process) {
	handler.logger.Info().Msgf("Restarting process %s", p.Name)
	p.IncreaseRestarts()
	p.RestartAt = nil
	p.SetStopSignal(false)
	newProcess, err := processrunner.SpawnNewProcess(processrunner.SpawnParams{
		Name:                     p.Name,
		Args:                     p.Args,
		ExecutablePath:           p.ExecutablePath,
		AutoRestart:              p.AutoRestart,
		Cwd:                      p.Cwd,
		Logger:                   handler.logger,
		CronRestart:              p.CronRestart,
		Env:                      p.Env,
		MaxRestarts:              p.MaxRestarts,
		MinUptimeMS:              p.MinUptimeMs,
		RestartDelayMS:           p.RestartDelayMs,
		ExpBackoffRestartDelayMS: p.ExpBackoffRestartDelayMs,
		MaxMemoryRestart:         p.MaxMemoryRestart,
	})
	if err != nil {
		p.AutoRestart = false
		p.SetStopSignal(true)
		p.SetStatus("stopped")
		p.ResetPid()
		updateProcessMap(handler, p.Id, nil)

		handler.logger.Error().Msgf("Error while restarting process %s: %s", p.Name, err)
		return
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

func nextAutoRestartDelay(p *pb.Process) time.Duration {
	if p.ExpBackoffRestartDelayMs > 0 {
		if p.CurrentRestartDelayMs <= 0 {
			p.CurrentRestartDelayMs = p.ExpBackoffRestartDelayMs
		} else {
			p.CurrentRestartDelayMs *= 2
		}
		return time.Duration(p.CurrentRestartDelayMs) * time.Millisecond
	}
	if p.RestartDelayMs > 0 {
		return time.Duration(p.RestartDelayMs) * time.Millisecond
	}
	return 0
}

func scheduleAutoRestart(handler *Handler, p *pb.Process, delay time.Duration) {
	p.RestartAt = timestamppb.New(time.Now().Add(delay))
	handler.logger.Info().Msgf("Scheduling restart for process %s in %s", p.Name, delay)
}

func handleAutoRestart(handler *Handler, p *pb.Process, uptime time.Duration) {
	if !p.AutoRestart || p.GetStopSignal() {
		return
	}

	if p.MinUptimeMs > 0 && uptime < time.Duration(p.MinUptimeMs)*time.Millisecond {
		p.UnstableRestarts++
	} else {
		p.UnstableRestarts = 0
		p.CurrentRestartDelayMs = 0
	}

	if p.MaxRestarts > 0 && p.UnstableRestarts > p.MaxRestarts {
		p.SetStopSignal(true)
		p.UpdateStatus("errored")
		handler.logger.Error().Msgf("Process %s exceeded max_restarts=%d", p.Name, p.MaxRestarts)
		return
	}

	delay := nextAutoRestartDelay(p)
	if delay > 0 {
		scheduleAutoRestart(handler, p, delay)
		return
	}
	restartProcess(handler, p)
}

func startScheduler(handler *Handler) {
	var wg sync.WaitGroup

	// sync process
	syncProcess := func(p *pb.Process) {
		if p.ProcStatus.Status == "online" {
			if _, ok := utils.IsProcessRunning(p.Pid); !ok {
				handler.mu.Lock()
				defer handler.mu.Unlock()

				uptime := time.Since(p.ProcStatus.StartedAt.AsTime())
				p.UpdateUptime()
				p.ResetPid()
				p.UpdateStatus("stopped")
				p.ResetCPUMemory()
				updateProcessMap(handler, p.Id, nil)

				// restart process if auto restart is enabled and process is not stopped
				handleAutoRestart(handler, p, uptime)
				handler.persistStateLocked()
			} else {
				p.UpdateUptime()
				if p.MaxMemoryRestart > 0 {
					memory, err := p.UpdateCPUMemoryStats()
					if err == nil && memory > p.MaxMemoryRestart {
						handler.mu.Lock()
						defer handler.mu.Unlock()

						handler.logger.Warn().Msgf("Process %s exceeded max_memory_restart=%d bytes", p.Name, p.MaxMemoryRestart)
						restartProcess(handler, p)
						handler.persistStateLocked()
					}
				}
			}
		} else if p.RestartAt != nil && p.RestartAt.AsTime().Before(time.Now()) {
			handler.mu.Lock()
			defer handler.mu.Unlock()

			p.RestartAt = nil
			restartProcess(handler, p)
			handler.persistStateLocked()
		} else if p.NextStartAt != nil && p.NextStartAt.AsTime().Before(time.Now()) {
			handler.mu.Lock()
			defer handler.mu.Unlock()

			handler.logger.Debug().Msgf("Process %s is scheduled to start at %s", p.Name, p.NextStartAt.AsTime())
			restartProcess(handler, p)
			p.UpdateNextStartAt()
			handler.persistStateLocked()
		}
		wg.Done()
	}

	// read config
	config := utils.GetConfig()

	// handle max log file, max log size
	handleMaxLog := func(p *pb.Process) {
		defer wg.Done()
		// if LogFilePath exceeds LogRotateSize, rename file and add logfilecount
		combinedLogFilePath := utils.FileSize(p.LogFilePath) + utils.FileSize(p.ErrFilePath)
		if config.LogRotate && combinedLogFilePath > int64(config.LogRotateSize) {
			err := utils.RenameFile(p.LogFilePath, p.LogFilePath+"."+strconv.Itoa(int(p.LogFileCount)))
			// if error rename file
			if err != nil {
				handler.logger.Error().Msgf("Error while renaming log file %s: %s", p.LogFilePath, err)
			}
			handler.logger.Info().Msgf("Renamed log file %s to %s", p.LogFilePath, p.LogFilePath+"."+strconv.Itoa(int(p.LogFileCount)))

			// do the same for error log file
			err = utils.RenameFile(p.ErrFilePath, p.ErrFilePath+"."+strconv.Itoa(int(p.LogFileCount)))
			// if error rename file
			if err != nil {
				handler.logger.Error().Msgf("Error while renaming log file %s: %s", p.ErrFilePath, err)
			}
			handler.logger.Info().Msgf("Renamed err file %s to %s", p.ErrFilePath, p.ErrFilePath+"."+strconv.Itoa(int(p.LogFileCount)))

			// if no error, increase logfilecount
			p.LogFileCount++

			// if LogFileCount exceeds LogRotateCount, delete oldest log file
			if p.LogFileCount >= int32(config.LogRotateMaxFiles) {
				// delete oldest log & err file
				err = os.Remove(p.LogFilePath + "." + strconv.Itoa(int(p.LogFileCount-int32(config.LogRotateMaxFiles))))
				if err != nil {
					handler.logger.Error().Msgf("Error while deleting log file %s: %s", p.LogFilePath+"."+strconv.Itoa(int(p.LogFileCount-int32(config.LogRotateMaxFiles))), err)
				}
				handler.logger.Info().Msgf("Deleted log file %s", p.LogFilePath+"."+strconv.Itoa(int(p.LogFileCount-int32(config.LogRotateMaxFiles))))

				err = os.Remove(p.ErrFilePath + "." + strconv.Itoa(int(p.LogFileCount-int32(config.LogRotateMaxFiles))))
				if err != nil {
					handler.logger.Error().Msgf("Error while deleting log file %s: %s", p.ErrFilePath+"."+strconv.Itoa(int(p.LogFileCount-int32(config.LogRotateMaxFiles))), err)
				}
				handler.logger.Info().Msgf("Deleted err file %s", p.ErrFilePath+"."+strconv.Itoa(int(p.LogFileCount-int32(config.LogRotateMaxFiles))))

				// decrease logfilecount
				p.LogFileCount = int32(config.LogRotateMaxFiles)
			}
		}

	}

	go func() {
		for {
			for _, p := range handler.databaseById {
				wg.Add(1)
				go syncProcess(p)

				if config.LogRotate {
					wg.Add(1)
					go handleMaxLog(p)
				}
			}
			wg.Wait()
			time.Sleep(500 * time.Millisecond)
		}
	}()
}
