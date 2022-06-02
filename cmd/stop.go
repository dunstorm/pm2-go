/*
Copyright © 2022 NAME HERE <EMAIL ADDRESS>

*/
package cmd

import (
	log "github.com/sirupsen/logrus"
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

		log.Info("Applying action stop on app [", args[0], "]")

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
