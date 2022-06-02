package cmd

import (
	"os"

	"github.com/dunstorm/pm2-go/app"
	"github.com/spf13/cobra"
)

// startCmd represents the start command
var startCmd = &cobra.Command{
	Use:   "start [options] [name|namespace|file|ecosystem|id...]",
	Short: "start and daemonize an app",
	Long:  `start and daemonize an app`,
	Run: func(cmd *cobra.Command, args []string) {
		master.SpawnDaemon()
		if len(args) < 1 {
			cmd.Usage()
			return
		}

		// check if args[0] is a file
		if _, err := os.Stat(args[0]); err == nil {
			master.StartFile(args[0])
			return
		}

		// if you can find the app in the database, start it
		process := master.FindProcess(args[0])
		if process.Name != "" {
			master.RestartProcess(process)
			return
		}

		// add process to the database
		process = master.SpawnNewProcess(app.SpawnParams{
			ExecutablePath: args[0],
			Args:           args[1:],
		})
		master.AddProcess(process)
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
