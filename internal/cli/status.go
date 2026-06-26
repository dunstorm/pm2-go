/*
Copyright © 2022 NAME HERE <EMAIL ADDRESS>
*/
package cli

import (
	"github.com/dunstorm/pm2-go/internal/utils"
	"github.com/spf13/cobra"
)

type daemonStatusView struct {
	Running bool  `json:"running"`
	PID     int32 `json:"pid,omitempty"`
}

// statusCmd represents the status command
var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Display status of daemon",
	Long:  `Display status of daemon`,
	Run: func(cmd *cobra.Command, args []string) {
		logger := master.GetLogger()
		jsonOutput, err := cmd.Flags().GetBool("json")
		if err != nil {
			logger.Fatal().Msg(err.Error())
		}
		pid, err := utils.ReadPidFile("daemon.pid")
		if err != nil {
			if jsonOutput {
				writeJSON(daemonStatusView{Running: false})
				return
			}
			logger.Info().Msg("PM2 Daemon Not Running")
			return
		}
		process, isRunning := utils.IsProcessRunning(pid)
		if jsonOutput {
			status := daemonStatusView{Running: isRunning}
			if isRunning {
				status.PID = int32(process.Pid)
			}
			writeJSON(status)
			return
		}
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
	statusCmd.Flags().Bool("json", false, "Print daemon status as JSON")
}
