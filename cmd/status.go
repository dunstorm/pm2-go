/*
Copyright © 2022 NAME HERE <EMAIL ADDRESS>

*/
package cmd

import (
	"github.com/dunstorm/pm2-go/utils"
	"github.com/spf13/cobra"
)

// statusCmd represents the status command
var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "A brief description of your command",
	Long: `A longer description that spans multiple lines and likely contains examples
and usage of using your command. For example:

Cobra is a CLI library for Go that empowers applications.
This application is a tool to generate the needed files
to quickly create a Cobra application.`,
	Run: func(cmd *cobra.Command, args []string) {
		logger := master.GetLogger()
		pid, err := utils.ReadPidFile("daemon.pid")
		if err != nil {
			logger.Info().Msg("PM2 Daemon Not Running")
			return
		}
		process, isRunning := utils.IsProcessRunning(pid)
		if isRunning {
			logger.Info().Msg("PM2 Daemon Running")
			logger.Info().Msgf("PID: %d", process.Pid)
		} else {
			logger.Info().Msg("PM2 Daemon Not Running")
		}
	},
}

func init() {
	rootCmd.AddCommand(statusCmd)

	// Here you will define your flags and configuration settings.

	// Cobra supports Persistent Flags which will work for this command
	// and all subcommands, e.g.:
	// statusCmd.PersistentFlags().String("foo", "", "A help for foo")

	// Cobra supports local flags which will only run when this command
	// is called directly, e.g.:
	// statusCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")
}
