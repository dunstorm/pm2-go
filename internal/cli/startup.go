package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/dunstorm/pm2-go/internal/utils"
	"github.com/spf13/cobra"
)

func systemdUnit(unitName string, userMode bool) (string, error) {
	executable, err := os.Executable()
	if err != nil {
		return "", err
	}
	executable, err = filepath.Abs(executable)
	if err != nil {
		return "", err
	}

	wantedBy := "multi-user.target"
	if userMode {
		wantedBy = "default.target"
	}

	return fmt.Sprintf(`[Unit]
Description=PM2-GO process manager (%s)
After=network.target

[Service]
Type=forking
Environment=PM2_GO_HOME=%s
ExecStart=%s -d
ExecStop=%s kill
Restart=on-failure

[Install]
WantedBy=%s
`, unitName, utils.GetMainDirectory(), executable, executable, wantedBy), nil
}

var startupCmd = &cobra.Command{
	Use:   "startup",
	Short: "Generate a systemd unit for pm2-go",
	Long:  `Generate a systemd unit for pm2-go`,
	Run: func(cmd *cobra.Command, args []string) {
		logger := master.GetLogger()
		unitName, err := cmd.Flags().GetString("unit-name")
		if err != nil {
			logger.Fatal().Msg(err.Error())
		}
		output, err := cmd.Flags().GetString("output")
		if err != nil {
			logger.Fatal().Msg(err.Error())
		}
		userMode, err := cmd.Flags().GetBool("user")
		if err != nil {
			logger.Fatal().Msg(err.Error())
		}

		unit, err := systemdUnit(unitName, userMode)
		if err != nil {
			logger.Fatal().Msg(err.Error())
		}
		if output == "" {
			fmt.Print(unit)
			return
		}
		if err := os.WriteFile(output, []byte(unit), 0644); err != nil {
			logger.Fatal().Msg(err.Error())
		}
		logger.Info().Msgf("Wrote systemd unit to %s", output)
	},
}

func init() {
	rootCmd.AddCommand(startupCmd)
	startupCmd.Flags().String("unit-name", "pm2-go", "Name used in the generated systemd unit description")
	startupCmd.Flags().String("output", "", "Write the generated unit to a file instead of stdout")
	startupCmd.Flags().Bool("user", false, "Generate a user service unit")
}
