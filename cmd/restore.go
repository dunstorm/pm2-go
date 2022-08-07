/*
Copyright © 2022 NAME HERE <EMAIL ADDRESS>

*/
package cmd

import (
	"strings"

	pb "github.com/dunstorm/pm2-go/proto"
	"github.com/dunstorm/pm2-go/utils"
	"github.com/spf13/cobra"
)

// restoreCmd represents the restore command
var restoreCmd = &cobra.Command{
	Use:   "restore",
	Short: "restore previously dumped processes",
	Long:  `restore previously dumped processes`,
	Run: func(cmd *cobra.Command, args []string) {
		logger := master.GetLogger()

		dumpFileName := "dump.json"
		if len(args) > 0 {
			dumpFileName = args[0]
		}

		// add .json if not exists
		if !strings.HasSuffix(dumpFileName, ".json") {
			dumpFileName = dumpFileName + ".json"
		}
		dumpFilePath := utils.GetDumpFilePath(dumpFileName)

		allProcesses := []*pb.Process{}
		err := utils.LoadObject(dumpFilePath, &allProcesses)
		if err != nil {
			logger.Error().Msg(err.Error())
			return
		}
		logger.Info().Msgf("Restoring processes located in %s", dumpFilePath)
		master.RestoreProcess(allProcesses)
		renderProcessList()
	},
}

func init() {
	rootCmd.AddCommand(restoreCmd)

	// Here you will define your flags and configuration settings.

	// Cobra supports Persistent Flags which will work for this command
	// and all subcommands, e.g.:
	// restoreCmd.PersistentFlags().String("foo", "", "A help for foo")

	// Cobra supports local flags which will only run when this command
	// is called directly, e.g.:
	// restoreCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")
}
