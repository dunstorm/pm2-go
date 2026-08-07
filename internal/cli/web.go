package cli

import (
	"fmt"
	"net/http"
	"os"

	"github.com/dunstorm/pm2-go/internal/web"
	"github.com/spf13/cobra"
)

var webCmd = &cobra.Command{
	Use:   "web [start]",
	Short: "Start the optional web dashboard",
	Long:  `Start the optional web dashboard`,
	Args: func(cmd *cobra.Command, args []string) error {
		if len(args) == 0 {
			return nil
		}
		if len(args) == 1 && args[0] == "start" {
			return nil
		}
		return fmt.Errorf("unsupported web action %q", args[0])
	},
	Run: func(cmd *cobra.Command, args []string) {
		logger := master.GetLogger()

		host, err := cmd.Flags().GetString("host")
		if err != nil {
			logger.Fatal().Msg(err.Error())
		}
		port, err := cmd.Flags().GetInt("port")
		if err != nil {
			logger.Fatal().Msg(err.Error())
		}
		token, err := cmd.Flags().GetString("token")
		if err != nil {
			logger.Fatal().Msg(err.Error())
		}
		allowRemote, err := cmd.Flags().GetBool("allow-remote")
		if err != nil {
			logger.Fatal().Msg(err.Error())
		}
		readOnly, err := cmd.Flags().GetBool("read-only")
		if err != nil {
			logger.Fatal().Msg(err.Error())
		}
		secureCookies, err := cmd.Flags().GetBool("secure-cookies")
		if err != nil {
			logger.Fatal().Msg(err.Error())
		}
		noDaemon, err := cmd.Flags().GetBool("no-daemon")
		if err != nil {
			logger.Fatal().Msg(err.Error())
		}
		daemonPort, err := cmd.Flags().GetInt("daemon-port")
		if err != nil {
			logger.Fatal().Msg(err.Error())
		}
		devAssets, err := cmd.Flags().GetString("dev-assets")
		if err != nil {
			logger.Fatal().Msg(err.Error())
		}
		devReload, err := cmd.Flags().GetBool("dev-reload")
		if err != nil {
			logger.Fatal().Msg(err.Error())
		}

		if token == "" {
			token = os.Getenv("PM2_GO_WEB_TOKEN")
		}
		if !noDaemon {
			master.SpawnDaemon()
		}

		server, err := web.NewServer(web.Config{
			Host:          host,
			Port:          port,
			Token:         token,
			AllowRemote:   allowRemote,
			ReadOnly:      readOnly,
			SecureCookies: secureCookies,
			AssetDir:      devAssets,
			DevReload:     devReload,
		}, web.NewGRPCProcessSource(daemonPort))
		if err != nil {
			logger.Fatal().Msg(err.Error())
		}

		if server.TokenGenerated() {
			logger.Warn().Msgf("Web token: %s", server.Token())
		}
		logger.Info().Msgf("Web dashboard listening on http://%s", server.Addr())
		if err := http.ListenAndServe(server.Addr(), server.Handler()); err != nil {
			logger.Fatal().Msg(err.Error())
		}
	},
}

func init() {
	rootCmd.AddCommand(webCmd)
	webCmd.Flags().String("host", web.DefaultHost, "Host interface for the web server")
	webCmd.Flags().Int("port", web.DefaultPort, "Port for the web server")
	webCmd.Flags().String("token", "", "Access token for the web UI; defaults to PM2_GO_WEB_TOKEN or a generated token")
	webCmd.Flags().Bool("allow-remote", false, "Allow binding to a non-loopback host")
	webCmd.Flags().Bool("read-only", false, "Disable lifecycle actions in the web UI")
	webCmd.Flags().Bool("secure-cookies", false, "Always mark session cookies Secure for TLS-terminating proxy deployments")
	webCmd.Flags().Bool("no-daemon", false, "Do not start the pm2-go daemon before serving")
	webCmd.Flags().Int("daemon-port", web.DefaultDaemonPort, "Local pm2-go daemon gRPC port")
	webCmd.Flags().String("dev-assets", "", "Serve web assets from this directory instead of embedded assets")
	webCmd.Flags().Bool("dev-reload", false, "Reload the browser when dev assets change")
}
