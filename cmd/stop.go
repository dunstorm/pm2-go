/*
Copyright © 2022 NAME HERE <EMAIL ADDRESS>

*/
package cmd

import (
	"os"

	"github.com/spf13/cobra"
)

// stopCmd represents the stop command
var stopCmd = &cobra.Command{
	Use:   "stop [options] <id|name|namespace|all|json|stdin...>",
	Short: "stop a process",
	Long:  `stop a process`,
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
			err = master.StopFile(args[0])
			if err == nil {
				renderProcessList()
			} else {
				logger.Fatal().Msg(err.Error())
			}
			return
		}

		// if you can find the app in the database, start it
		process := master.FindProcess(args[0])
		if process.Name == "" {
			logger.Error().Msgf("Process or namespace %s not found", args[0])
			return
		}

		logger.Info().Msgf("Applying action stopProcessId on app [%s]", args[0])
		master.StopProcessByName(args[0])
	},
}

func init() {
	rootCmd.AddCommand(stopCmd)

	// Here you will define your flags and configuration settings.

	// Cobra supports Persistent Flags which will work for this command
	// and all subcommands, e.g.:
	// stopCmd.PersistentFlags().String("foo", "", "A help for foo")

	// Cobra supports local flags which will only run when this command
	// is called directly, e.g.:
	// stopCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")
}