/*
Copyright © 2022 NAME HERE <EMAIL ADDRESS>
*/
package cli

import (
	"fmt"
	"os"
	"strconv"
	"sync"

	"github.com/dunstorm/pm2-go/internal/logstore"
	"github.com/dunstorm/pm2-go/internal/utils"
	"github.com/fatih/color"

	"github.com/spf13/cobra"
)

// logsCmd represents the logs command
var logsCmd = &cobra.Command{
	Use:   "logs [options] [id|name|namespace]",
	Short: "Stream logs file",
	Long:  `Stream logs file`,
	Run: func(cmd *cobra.Command, args []string) {
		if len(args) < 1 {
			cmd.Usage()
			return
		}

		tail, _ := cmd.Flags().GetInt("lines")

		// check if args[0] is a file
		// get file extension
		// if it's a json file, parse it and start the app
		if isJSONFilePath(args[0]) {
			master.StartFile(args[0])
			return
		}

		logger := master.GetLogger()

		// if you can find the app in the database
		process := master.FindProcess(args[0])
		if process != nil {
			logPrefix := strconv.Itoa(int(process.Id)) + "|" + process.Name + "| "

			green := color.New(color.FgGreen).SprintFunc()
			red := color.New(color.FgRed).SprintFunc()

			cyanBold := color.New(color.FgCyan, color.Bold)
			cyanBold.Printf("[TAILING] Tailing last %d lines for [%s] process (change the value with --lines option)\n", tail, process.Name)

			combinedLogPath := logstore.CombinedPath(process.LogFilePath)
			if _, err := os.Stat(combinedLogPath); err == nil {
				color.Cyan("%s last %d lines", combinedLogPath, tail)
				entries, err := logstore.ReadEntries(combinedLogPath, tail)
				if err != nil {
					logger.Error().Msg(err.Error())
					return
				}
				for _, entry := range entries {
					printCombinedLogEntry(logPrefix, green, red, entry)
				}

				var wg sync.WaitGroup
				wg.Add(1)
				go func() {
					if err := logstore.TailEntries(combinedLogPath, func(entry logstore.Entry) {
						printCombinedLogEntry(logPrefix, green, red, entry)
					}); err != nil {
						logger.Error().Msg(err.Error())
					}
				}()
				wg.Wait()
				return
			}

			outLastModified := utils.GetLastModified(process.LogFilePath)
			errLastModified := utils.GetLastModified(process.ErrFilePath)

			printStdoutLogs := func() {
				// print stdout logs
				color.Green("%s last %d lines", process.LogFilePath, tail)
				logs, err := utils.GetLogs(process.LogFilePath, tail)
				if err != nil {
					logger.Error().Msg(err.Error())
					return
				}
				utils.PrintLogs(logs, logPrefix, green)
			}

			printStderrLogs := func() {
				// print error logs
				color.Red("%s last %d lines", process.ErrFilePath, tail)
				logs, err := utils.GetLogs(process.ErrFilePath, tail)
				if err != nil {
					logger.Error().Msg(err.Error())
					return
				}
				utils.PrintLogs(logs, logPrefix, red)
			}

			if errLastModified.Before(outLastModified) {
				printStderrLogs()
				fmt.Println()
				printStdoutLogs()
			} else {
				printStdoutLogs()
				fmt.Println()
				printStderrLogs()
			}

			// to run it indefinitely
			var wg sync.WaitGroup
			wg.Add(1)
			go utils.Tail(logPrefix, green, process.LogFilePath, os.Stdout)
			go utils.Tail(logPrefix, red, process.ErrFilePath, os.Stdout)
			wg.Wait()
		}

		logger.Error().Msgf("Process or Namespace %s not found", args[0])
	},
}

func printCombinedLogEntry(logPrefix string, green func(a ...interface{}) string, red func(a ...interface{}) string, entry logstore.Entry) {
	prefixColor := green
	if entry.Stream == logstore.StderrStream {
		prefixColor = red
	}
	fmt.Println(prefixColor(logPrefix), logstore.FormatEntry(entry))
}

func init() {
	rootCmd.AddCommand(logsCmd)

	// Here you will define your flags and configuration settings.

	// Cobra supports Persistent Flags which will work for this command
	// and all subcommands, e.g.:
	// logsCmd.PersistentFlags().String("foo", "", "A help for foo")

	// Cobra supports local flags which will only run when this command
	// is called directly, e.g.:
	// logsCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")
}
