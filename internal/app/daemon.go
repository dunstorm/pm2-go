package app

import (
	"fmt"
	"os"
	"os/exec"
	"path"
	"time"

	"github.com/dunstorm/pm2-go/internal/grpc/server"
	"github.com/dunstorm/pm2-go/internal/utils"
	"github.com/rs/zerolog"
)

const daemonPort = 50051

func isDaemonRunning() bool {
	directory := utils.GetMainDirectory()
	// check if daemon.pid exists
	if _, err := os.Stat(path.Join(directory, "daemon.pid")); os.IsNotExist(err) {
		return false
	}
	// read daemon.pid and check if process is running
	pid, err := utils.ReadPidFile("daemon.pid")
	if err != nil {
		return false
	}
	// check if process is running by pid
	if _, running := utils.IsProcessRunning(pid); running {
		return utils.IsPortOpen(daemonPort)
	}
	return false
}

const (
	MARK_NAME  = "_GO_DAEMON"
	MARK_VALUE = "1"
)

func wasReborn() bool {
	return os.Getenv(MARK_NAME) == MARK_VALUE
}

func (app *App) SpawnDaemon() {
	if isDaemonRunning() && !wasReborn() {
		return
	}

	if wasReborn() {
		logger := zerolog.New(os.Stderr).With().Timestamp().Logger()
		app.logger = &logger
	}

	app.logger.Info().Msgf("Spawning PM2 daemon with pm2_home=%s", utils.GetMainDirectory())

	if !wasReborn() {
		if utils.IsPortOpen(daemonPort) {
			app.logger.Error().Msgf("PM2 daemon port %d is already in use", daemonPort)
			os.Exit(1)
		}

		daemonPidFile := path.Join(utils.GetMainDirectory(), "daemon.pid")
		daemonLogFile := path.Join(utils.GetMainDirectory(), "daemon.log")

		logFile, err := os.OpenFile(daemonLogFile, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0640)
		if err != nil {
			app.logger.Fatal().Msg(err.Error())
			return
		}
		defer logFile.Close()

		nullFile, err := os.Open(os.DevNull)
		if err != nil {
			app.logger.Fatal().Msg(err.Error())
			return
		}
		defer nullFile.Close()

		// create process
		var attr = os.ProcAttr{
			Dir: ".",
			Env: append(
				[]string{
					fmt.Sprintf("%s=%s", MARK_NAME, MARK_VALUE),
				}, os.Environ()...,
			),
			Files: []*os.File{
				nullFile,
				logFile,
				logFile,
			},
		}

		binPath, err := exec.LookPath(os.Args[0])
		if err != nil {
			app.logger.Error().Msg(err.Error())
			return
		}

		fullCommand := []string{binPath}
		fullCommand = append(fullCommand, "-d")
		process, err := os.StartProcess(binPath, fullCommand, &attr)
		if err == nil {
			app.logger.Info().Msgf("Daemon PID: %d", process.Pid)

			// write pid to file
			// write daemon pid
			err = utils.WritePidToFile(daemonPidFile, process.Pid)
			if err != nil {
				app.logger.Error().Msg(err.Error())
				return
			}

			// detaches the process
			err = process.Release()
			if err != nil {
				app.logger.Error().Msg(err.Error())
				return
			}
		} else {
			app.logger.Error().Msg(err.Error())
			return
		}

		// wait for 50051 port to open with a timeout of 2s
		found := false
		for i := 0; i < 200; i++ {
			if utils.IsPortOpen(daemonPort) {
				found = true
				break
			}
			time.Sleep(10 * time.Millisecond)
		}

		if !found {
			app.logger.Error().Msg("PM2 Failed to start")
			os.Exit(1)
		} else {
			app.logger.Info().Msg("PM2 Successfully daemonized")
		}
	}

	if wasReborn() {
		server.New(daemonPort)
	}
}
