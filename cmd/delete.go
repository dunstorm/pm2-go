/*
Copyright © 2022 NAME HERE <EMAIL ADDRESS>

*/
package cmd

import (
	"github.com/spf13/cobra"
)

// deleteCmd represents the delete command
var deleteCmd = &cobra.Command{
	Use:   "delete [options] <name|id|namespace|script|all|json|stdin...>",
	Short: "stop and delete a process from pm2 process list",
	Long:  `stop and delete a process from pm2 process list`,
	Run: func(cmd *cobra.Command, args []string) {
		if len(args) < 1 {
			cmd.Usage()
			return
		}

		logger := master.GetLogger()

		process := master.FindProcess(args[0])
		if process.ProcStatus == nil {
			logger.Error().Msgf("Process or Namespace %s not found", args[0])
			return
		}

		// stop process if alive
		if process.ProcStatus.Status == "online" {
			logger.Info().Msgf("Applying action stopProcessId on app [%s](pid: [ %d ])", process.Name, process.Pid)
			master.StopProcess(process)
		}

		// delete a process
		logger.Info().Msgf("Applying action deleteProcessId on app [%s]", process.Name)
		master.DeleteProcess(process)

		renderProcessList()
	},
}

func init() {
	rootCmd.AddCommand(deleteCmd)

	// Here you will define your flags and configuration settings.

	// Cobra supports Persistent Flags which will work for this command
	// and all subcommands, e.g.:
	// deleteCmd.PersistentFlags().String("foo", "", "A help for foo")

	// Cobra supports local flags which will only run when this command
	// is called directly, e.g.:
	// deleteCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")
}
