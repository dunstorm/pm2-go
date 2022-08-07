/*
Copyright © 2022 NAME HERE <EMAIL ADDRESS>

*/
package cmd

import (
	"os"

	pb "github.com/dunstorm/pm2-go/proto"
	"github.com/dunstorm/pm2-go/utils"
	"github.com/spf13/cobra"
)

// flushCmd represents the flush command
var flushCmd = &cobra.Command{
	Use:   "flush",
	Short: "flush [options] [api]",
	Long:  `flush logs`,
	Run: func(cmd *cobra.Command, args []string) {
		master.SpawnDaemon()

		logger := master.GetLogger()

		flushProcess := func(process *pb.Process) {
			logger.Info().Msg(process.LogFilePath)
			logger.Info().Msg(process.ErrFilePath)

			// remove file contents
			utils.RemoveFileContents(process.LogFilePath)
			utils.RemoveFileContents(process.ErrFilePath)
		}

		if len(args) == 0 || args[0] == "all" {
			db := master.ListProcess()
			if len(db) == 0 {
				logger.Warn().Msg("No processes found")
				return
			}
			logger.Info().Msg("Flushing:")
			for _, process := range db {
				flushProcess(process)
			}
			logger.Info().Msg("Logs flushed")
			return
		}

		// check if args[0] is a file
		// get file extension
		// if it's a json file, parse it and start the app
		if _, err := os.Stat(args[0]); err == nil && args[0][len(args[0])-5:] == ".json" {
			logger.Info().Msg("Flushing:")
			err = master.FlushFile(args[0], flushProcess)
			if err != nil {
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

		// logs
		logger.Info().Msg("Flushing:")
		logger.Info().Msg(process.LogFilePath)
		logger.Info().Msg(process.ErrFilePath)

		// remove file contents
		utils.RemoveFileContents(process.LogFilePath)
		utils.RemoveFileContents(process.ErrFilePath)

		logger.Info().Msg("Logs flushed")
	},
}

func init() {
	rootCmd.AddCommand(flushCmd)

	// Here you will define your flags and configuration settings.

	// Cobra supports Persistent Flags which will work for this command
	// and all subcommands, e.g.:
	// flushCmd.PersistentFlags().String("foo", "", "A help for foo")

	// Cobra supports local flags which will only run when this command
	// is called directly, e.g.:
	// flushCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")
}
