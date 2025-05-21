/*
Copyright © 2022 NAME HERE <EMAIL ADDRESS>

*/
package cmd

import (
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
)

// restartCmd represents the restart command
var restartCmd = &cobra.Command{
	Use:   "restart [options] <id|name|namespace|all|json|stdin...>",
	Short: "Restart a process",
	Long:  `Restart a process`,
	Run: func(cmd *cobra.Command, args []string) {
		if len(args) < 1 {
			cmd.Usage()
			return
		}

		logger := master.GetLogger()

		if args[0] == "all" {
			db := master.ListProcess()
			if len(db) == 0 {
				logger.Warn().Msg("No processes found")
				return
			}
			for _, process := range db {
				master.GetLogger().Info().Msgf("Applying action restartProcessId on app [%d](pid: [ %d ])", process.Id, process.Pid)
				master.RestartProcess(process)
			}
			renderProcessList()
			return
		}

		// check if args[0] is a file
		// get file extension
		// if it's a json file, parse it and start the app
		if fi, err := os.Stat(args[0]); err == nil && !fi.IsDir() && filepath.Ext(args[0]) == ".json" {
			err = master.StartFile(args[0]) // StartFile here implies it handles restarts for processes in the file.
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
			master.RestartProcess(process)
			renderProcessList()
			return
		} else {
			logger.Error().Msgf("Process or Namespace %s not found", args[0])
		}
	},
}

func init() {
	rootCmd.AddCommand(restartCmd)

	// Here you will define your flags and configuration settings.

	// Cobra supports Persistent Flags which will work for this command
	// and all subcommands, e.g.:
	// restartCmd.PersistentFlags().String("foo", "", "A help for foo")

	// Cobra supports local flags which will only run when this command
	// is called directly, e.g.:
	// restartCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")
}
