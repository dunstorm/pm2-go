package cmd

import (
	"os"

	"github.com/dunstorm/pm2-go/shared"
	"github.com/spf13/cobra"
)

// startCmd represents the start command
var startCmd = &cobra.Command{
	Use:   "start [options] [name|namespace|file|ecosystem|id...]",
	Short: "Start and daemonize an app",
	Long:  `Start and daemonize an app`,
	Run: func(cmd *cobra.Command, args []string) {
		master.SpawnDaemon()
		if len(args) < 1 {
			cmd.Usage()
			return
		}

		logger := master.GetLogger()

		// check if args[0] is a file
		// get file extension
		// if it's a json file, parse it and start the app
		if _, err := os.Stat(args[0]); err == nil && args[0][len(args[0])-5:] == ".json" {
			err = master.StartFile(args[0])
			if err == nil {
				renderProcessList()
			} else {
				logger.Fatal().Msg(err.Error())
			}
			return
		}

		// if you can find the app in the database, start it
		process := master.FindProcess(args[0])
		if process.Name != "" {
			master.GetLogger().Info().Msgf("Applying action startProcessId on app [%d](pid: [ %d ])", process.ID, process.Pid)
			master.RestartProcess(process)
			renderProcessList()
			return
		}

		// add process to the database
		process = shared.SpawnNewProcess(shared.SpawnParams{
			ExecutablePath: args[0],
			Args:           args[1:],
			Logger:         logger,
		})
		master.GetLogger().Info().Msgf("Applying action addProcessName on app [%s](pid: [ %d ])", process.Name, process.Pid)
		master.AddProcess(process)

		renderProcessList()
	},
}

func init() {
	rootCmd.AddCommand(startCmd)

	// Here you will define your flags and configuration settings.

	// Cobra supports Persistent Flags which will work for this command
	// and all subcommands, e.g.:
	// startCmd.PersistentFlags().String("foo", "", "A help for foo")

	// Cobra supports local flags which will only run when this command
	// is called directly, e.g.:
	// startCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")
}
