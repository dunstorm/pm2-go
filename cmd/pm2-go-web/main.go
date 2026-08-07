package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/dunstorm/pm2-go/internal/web"
)

func main() {
	host := flag.String("host", web.DefaultHost, "Host interface for the web server")
	port := flag.Int("port", web.DefaultPort, "Port for the web server")
	token := flag.String("token", "", "Access token for the web UI; defaults to PM2_GO_WEB_TOKEN or a generated token")
	daemonPort := flag.Int("daemon-port", web.DefaultDaemonPort, "Local pm2-go daemon gRPC port")
	allowRemote := flag.Bool("allow-remote", false, "Allow binding to a non-loopback host")
	readOnly := flag.Bool("read-only", false, "Disable lifecycle actions in the web UI")
	secureCookies := flag.Bool("secure-cookies", false, "Always mark session cookies Secure for TLS-terminating proxy deployments")
	devAssets := flag.String("dev-assets", "", "Serve web assets from this directory instead of embedded assets")
	devReload := flag.Bool("dev-reload", false, "Reload the browser when dev assets change")
	flag.Parse()

	webToken := *token
	if webToken == "" {
		webToken = os.Getenv("PM2_GO_WEB_TOKEN")
	}

	server, err := web.NewServer(web.Config{
		Host:          *host,
		Port:          *port,
		Token:         webToken,
		AllowRemote:   *allowRemote,
		ReadOnly:      *readOnly,
		SecureCookies: *secureCookies,
		AssetDir:      *devAssets,
		DevReload:     *devReload,
	}, web.NewGRPCProcessSource(*daemonPort))
	if err != nil {
		fmt.Fprintf(os.Stderr, "pm2-go-web: %v\n", err)
		os.Exit(1)
	}

	if server.TokenGenerated() {
		fmt.Fprintf(os.Stderr, "pm2-go-web token: %s\n", server.Token())
	}
	fmt.Fprintf(os.Stderr, "pm2-go-web listening on http://%s\n", server.Addr())
	if err := server.HTTPServer().ListenAndServe(); err != nil {
		fmt.Fprintf(os.Stderr, "pm2-go-web: %v\n", err)
		os.Exit(1)
	}
}
