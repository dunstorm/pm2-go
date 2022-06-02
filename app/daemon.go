package app

import (
	"fmt"
	"os"
	"path"

	"github.com/dunstorm/pm2-go/rpc/server"
	"github.com/dunstorm/pm2-go/utils"
	log "github.com/sirupsen/logrus"
)

func init() {
	// Log as JSON instead of the default ASCII formatter.
	log.SetFormatter(&log.TextFormatter{})

	// Output to stdout instead of the default stderr
	// Can be any io.Writer, see below for File example
	log.SetOutput(os.Stdout)

	// Only log the warning severity or above.
	log.SetLevel(log.DebugLevel)
}

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

func (a *App) SpawnDaemon() {
	if isDaemonRunning() && !wasReborn() {
		return
	}
	log.Info("Spawning PM2 daemon with pm2_home=", utils.GetMainDirectory())

	daemonPidFile := path.Join(utils.GetMainDirectory(), "daemon.pid")
	daemonLogFile := path.Join(utils.GetMainDirectory(), "daemon.log")

	logFile, err := os.OpenFile(daemonLogFile, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0640)
	if err != nil {
		log.Fatal(err)
		return
	}
	nullFile, err := os.Open(os.DevNull)
	if err != nil {
		log.Fatal(err)
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
			log.Fatal("You're not allowed to run using go run")
			os.Exit(0)
		}

		fullCommand := []string{binPath}
		fullCommand = append(fullCommand, "-d")
		process, err := os.StartProcess(binPath, fullCommand, &attr)
		if err == nil {
			log.Info("Daemon PID: ", process.Pid)

			// write pid to file
			// write daemon pid
			err = utils.WritePidToFile(daemonPidFile, process.Pid)
			if err != nil {
				log.Error(err.Error())
				return
			}

			// detaches the process
			err = process.Release()
			if err != nil {
				log.Error(err.Error())
				return
			}
		} else {
			log.Error(err.Error())
			return
		}

		log.Info("PM2 Successfully daemonized")
	}

	if wasReborn() {
		server.New()
	}
}
