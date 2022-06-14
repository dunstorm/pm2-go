/*
Copyright © 2022 NAME HERE <EMAIL ADDRESS>

*/
package cmd

import (
	"strings"

	"github.com/dunstorm/pm2-go/shared"
	"github.com/dunstorm/pm2-go/utils"
	"github.com/spf13/cobra"
)

// dumpCmd represents the dump command
var dumpCmd = &cobra.Command{
	Use:   "dump",
	Short: "dump all processes for resurrecting them later",
	Long:  `dump all processes for resurrecting them later`,
	Run: func(cmd *cobra.Command, args []string) {
		dumpFileName := "dump.json"
		if len(args) > 0 {
			dumpFileName = args[0]
		}

		// add .json if not exists
		if !strings.HasSuffix(dumpFileName, ".json") {
			dumpFileName = dumpFileName + ".json"
		}

		master.SpawnDaemon()
		logger := master.GetLogger()
		logger.Info().Msg("Saving current process list...")
		allProcesses := []shared.Process{}
		for _, process := range master.GetDB() {
			allProcesses = append(allProcesses, *process)
		}
		dumpFilePath := utils.GetDumpFilePath(dumpFileName)
		err := utils.SaveObject(dumpFilePath, allProcesses)
		if err != nil {
			logger.Error().Msg(err.Error())
			return
		}
		logger.Info().Msg("Successfully saved in " + dumpFilePath)
	},
}

func init() {
	rootCmd.AddCommand(dumpCmd)

	// Here you will define your flags and configuration settings.

	// Cobra supports Persistent Flags which will work for this command
	// and all subcommands, e.g.:
	// dumpCmd.PersistentFlags().String("foo", "", "A help for foo")

	// Cobra supports local flags which will only run when this command
	// is called directly, e.g.:
	// dumpCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")
}
