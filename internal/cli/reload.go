package cli

import (
	"os"

	"github.com/dunstorm/pm2-go/internal/app"
	"github.com/dunstorm/pm2-go/internal/utils"
	"github.com/spf13/cobra"
)

var reloadCmd = &cobra.Command{
	Use:   "reload [options] <id|name|all|json>",
	Short: "Gracefully reload a process",
	Long:  `Gracefully reload a process`,
	Run: func(cmd *cobra.Command, args []string) {
		if len(args) < 1 {
			cmd.Usage()
			return
		}

		logger := master.GetLogger()
		updateEnv, err := cmd.Flags().GetBool("update-env")
		if err != nil {
			logger.Fatal().Msg(err.Error())
		}
		envName, err := cmd.Flags().GetString("env")
		if err != nil {
			logger.Fatal().Msg(err.Error())
		}
		signalName, err := cmd.Flags().GetString("signal")
		if err != nil {
			logger.Fatal().Msg(err.Error())
		}
		killTimeout, err := cmd.Flags().GetInt32("kill-timeout")
		if err != nil {
			logger.Fatal().Msg(err.Error())
		}

		var env map[string]string
		if updateEnv {
			env = utils.EnvironmentMap(os.Environ())
		}
		options := app.RestartOptions{
			Env:           env,
			Graceful:      true,
			Signal:        signalName,
			KillTimeoutMS: killTimeout,
		}

		if args[0] == "all" {
			db := master.ListProcess()
			if len(db) == 0 {
				logger.Warn().Msg("No processes found")
				return
			}
			for _, process := range db {
				master.GetLogger().Info().Msgf("Applying action reloadProcessId on app [%d](pid: [ %d ])", process.Id, process.Pid)
				master.RestartProcessWithOptions(process, options)
			}
			renderProcessList()
			return
		}

		if isJSONFilePath(args[0]) {
			err = master.StartFileWithOptions(args[0], app.StartFileOptions{
				Env:           env,
				EnvName:       envName,
				UseCurrentEnv: updateEnv,
				Graceful:      true,
				Signal:        signalName,
				KillTimeoutMS: killTimeout,
			})
			if err == nil {
				renderProcessList()
			} else {
				logger.Fatal().Msg(err.Error())
			}
			return
		}

		process := master.FindProcess(args[0])
		if process != nil && process.Name != "" {
			master.RestartProcessWithOptions(process, options)
			renderProcessList()
			return
		}
		logger.Error().Msgf("Process or Namespace %s not found", args[0])
	},
}

func init() {
	rootCmd.AddCommand(reloadCmd)
	reloadCmd.Flags().Bool("update-env", false, "Update process environment from the current shell before reloading")
	reloadCmd.Flags().String("env", "", "Use env_<name> values from an ecosystem file")
	reloadCmd.Flags().String("signal", "SIGTERM", "Signal sent before force-killing a process")
	reloadCmd.Flags().Int32("kill-timeout", 1600, "Milliseconds to wait before force-killing a process")
}
