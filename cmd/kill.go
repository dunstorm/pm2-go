/*
Copyright © 2022 NAME HERE <EMAIL ADDRESS>

*/
package cmd

import (
	"fmt"

	"github.com/dunstorm/pm2-go/utils"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

// killCmd represents the kill command
var killCmd = &cobra.Command{
	Use:   "kill",
	Short: "A brief description of your command",
	Long: `A longer description that spans multiple lines and likely contains examples
and usage of using your command. For example:

Cobra is a CLI library for Go that empowers applications.
This application is a tool to generate the needed files
to quickly create a Cobra application.`,
	Run: func(cmd *cobra.Command, args []string) {
		pid, err := utils.ReadPidFile("daemon.pid")
		if err != nil {
			return
		}
		process, isRunning := utils.IsProcessRunning(pid)
		if isRunning {
			procs := master.ListProcs()
			if len(procs) > 0 {
				for _, p := range procs {
					if p.ProcStatus.Status == "online" {
						log.Info("Applying action stopProcessId on app [", p.Name, "](pid: [ ", p.Pid, " ])")
						master.StopProcessByIndex(p.ID)
					}
				}
			} else {
				log.Warn("No processes found")
			}
			err := process.Kill()
			if err != nil {
				fmt.Println(err)
				return
			}
			log.Info("PM2 Daemon Stopped")
		}
	},
}

func init() {
	rootCmd.AddCommand(killCmd)

	// Here you will define your flags and configuration settings.

	// Cobra supports Persistent Flags which will work for this command
	// and all subcommands, e.g.:
	// killCmd.PersistentFlags().String("foo", "", "A help for foo")

	// Cobra supports local flags which will only run when this command
	// is called directly, e.g.:
	// killCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")
}
