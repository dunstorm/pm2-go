package server

import (
	"net"
	"net/http"
	"net/rpc"
	"os"
	"time"

	"github.com/dunstorm/pm2-go/utils"
	log "github.com/sirupsen/logrus"
)

func New() {
	// Log as JSON instead of the default ASCII formatter.
	log.SetFormatter(&log.TextFormatter{})

	// Output to stdout instead of the default stderr
	// Can be any io.Writer, see below for File example
	log.SetOutput(os.Stdout)

	// Only log the warning severity or above.
	log.SetLevel(log.DebugLevel)

	handler := new(API)

	// Publish the receivers methods
	err := rpc.Register(handler)
	if err != nil {
		log.Fatal("Format of service API isn't correct. ", err)
	}

	// Register a HTTP handler
	rpc.HandleHTTP()

	// Listen to TPC connections on port 9001
	listener, e := net.Listen("tcp", ":9001")
	if e != nil {
		log.Fatal("Listen error: ", e)
	}
	log.Printf("Serving RPC server on port %d", 9001)

	go func() {
		for {
			for index, p := range database {
				if p.ProcStatus.Status == "online" {
					if _, ok := utils.IsProcessRunning(p.Pid); !ok {
						p.UpdateUptime()
						p.ResetPid()
						p.UpdateStatus("stopped")
						database[index] = p
					} else {
						p.UpdateUptime()
					}
				}
			}
			time.Sleep(1 * time.Second)
		}
	}()

	// Start accept incoming HTTP connections
	err = http.Serve(listener, nil)
	if err != nil {
		log.Fatal("Error serving: ", err)
	}
}
