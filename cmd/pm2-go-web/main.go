package main

import (
	"flag"
	"fmt"
	"net/http"
	"os"

	"github.com/dunstorm/pm2-go/internal/web"
)

func main() {
	host := flag.String("host", web.DefaultHost, "Host interface for the web server")
	port := flag.Int("port", web.DefaultPort, "Port for the web server")
	token := flag.String("token", "", "Access token for the web UI; defaults to PM2_GO_WEB_TOKEN or a generated token")
	daemonPort := flag.Int("daemon-port", web.DefaultDaemonPort, "Local pm2-go daemon gRPC port")
	allowRemote := flag.Bool("allow-remote", false, "Allow binding to a non-loopback host")
	flag.Parse()

	webToken := *token
	if webToken == "" {
		webToken = os.Getenv("PM2_GO_WEB_TOKEN")
	}

	server, err := web.NewServer(web.Config{
		Host:        *host,
		Port:        *port,
		Token:       webToken,
		AllowRemote: *allowRemote,
	}, web.NewGRPCProcessSource(*daemonPort))
	if err != nil {
		fmt.Fprintf(os.Stderr, "pm2-go-web: %v\n", err)
		os.Exit(1)
	}

	if server.TokenGenerated() {
		fmt.Fprintf(os.Stderr, "pm2-go-web token: %s\n", server.Token())
	}
	fmt.Fprintf(os.Stderr, "pm2-go-web listening on http://%s\n", server.Addr())
	if err := http.ListenAndServe(server.Addr(), server.Handler()); err != nil {
		fmt.Fprintf(os.Stderr, "pm2-go-web: %v\n", err)
		os.Exit(1)
	}
}
