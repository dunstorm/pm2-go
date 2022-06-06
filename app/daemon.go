package app

import (
	"fmt"
	"net"
	"os"
	"path"
	"time"

	"github.com/dunstorm/pm2-go/rpc/server"
	"github.com/dunstorm/pm2-go/utils"
	"github.com/rs/zerolog"
)

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
		return true
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

func isPortOpen(port int) bool {
	conn, err := net.Dial("tcp", fmt.Sprintf("localhost:%d", port))
	if err != nil {
		return false
	}
	defer conn.Close()
	return true
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

	daemonPidFile := path.Join(utils.GetMainDirectory(), "daemon.pid")
	daemonLogFile := path.Join(utils.GetMainDirectory(), "daemon.log")

	logFile, err := os.OpenFile(daemonLogFile, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0640)
	if err != nil {
		app.logger.Fatal().Msg(err.Error())
		return
	}
	nullFile, err := os.Open(os.DevNull)
	if err != nil {
		app.logger.Fatal().Msg(err.Error())
		return
	}

	if !wasReborn() {
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

		binPath := os.Args[0]

		// check if substring present in string
		if utils.StringContains(binPath, "/var/folders") {
			app.logger.Fatal().Msg("You're not allowed to run using go run")
			os.Exit(0)
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

		// wait for 9001 port to open with a timeout of 2s
		found := false
		for i := 0; i < 200; i++ {
			if isPortOpen(9001) {
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
		server.New()
	}
}
