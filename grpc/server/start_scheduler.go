package server

import (
	"os"
	"strconv"
	"sync"
	"time"

	pb "github.com/dunstorm/pm2-go/proto"
	"github.com/dunstorm/pm2-go/shared"
	"github.com/dunstorm/pm2-go/utils"
)

func updateProcessMap(handler *Handler, processId int32, p *os.Process) {
	handler.processes[processId] = p
}

func restartProcess(handler *Handler, p *pb.Process) {
	handler.logger.Info().Msgf("Restarting process %s", p.Name)
	p.IncreaseRestarts()
	newProcess, err := shared.SpawnNewProcess(shared.SpawnParams{
		Name:           p.Name,
		Args:           p.Args,
		ExecutablePath: p.ExecutablePath,
		AutoRestart:    p.AutoRestart,
		Cwd:            p.Cwd,
		Logger:         handler.logger,
		Scripts:        p.Scripts,
		CronRestart:    p.CronRestart,
	})
	if err != nil {
		p.AutoRestart = false
		p.SetStopSignal(true)
		p.SetStatus("stopped")
		p.ResetPid()
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

func startScheduler(handler *Handler) {
	var wg sync.WaitGroup

	// sync process
	syncProcess := func(p *pb.Process) {
		if p.ProcStatus.Status == "online" {
			if _, ok := utils.IsProcessRunning(p.Pid); !ok {
				handler.mu.Lock()
				defer handler.mu.Unlock()

				p.UpdateUptime()
				p.ResetPid()
				p.UpdateStatus("stopped")
				p.ResetCPUMemory()
				updateProcessMap(handler, p.Id, nil)

				// restart process if auto restart is enabled and process is not stopped
				if p.AutoRestart && !p.GetStopSignal() {
					restartProcess(handler, p)
				}
			} else {
				p.UpdateUptime()
			}
		} else if p.NextStartAt != nil && p.NextStartAt.AsTime().Before(time.Now()) {
			handler.logger.Debug().Msgf("Process %s is scheduled to start at %s", p.Name, p.NextStartAt.AsTime())
			restartProcess(handler, p)
			p.UpdateNextStartAt()
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
