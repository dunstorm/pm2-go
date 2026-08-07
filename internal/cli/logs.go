/*
Copyright © 2022 NAME HERE <EMAIL ADDRESS>
*/
package cli

import (
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/dunstorm/pm2-go/internal/logstore"
	"github.com/dunstorm/pm2-go/internal/utils"
	pb "github.com/dunstorm/pm2-go/proto"
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
				color.Cyan("%s merged last %d lines", combinedLogPath, tail)
				entries, tailCursor, err := mergedLogEntries(process, combinedLogPath, tail)
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
					if err := logstore.TailEntriesFromCursor(combinedLogPath, tailCursor, func(entry logstore.Entry) {
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

type sortableLogEntry struct {
	entry      logstore.Entry
	occurredAt time.Time
	order      int
}

func mergedLogEntries(process *pb.Process, combinedLogPath string, tail int) ([]logstore.Entry, logstore.TailCursor, error) {
	combinedEntries, tailCursor, err := logstore.ReadEntriesWithCursor(combinedLogPath, tail)
	if err != nil {
		return nil, logstore.TailCursor{}, err
	}
	if tail <= 0 || len(combinedEntries) >= tail {
		return combinedEntries, tailCursor, nil
	}

	legacyEntries, err := legacyLogEntries(process, tail)
	if err != nil {
		return nil, logstore.TailCursor{}, err
	}
	if len(legacyEntries) == 0 {
		return combinedEntries, tailCursor, nil
	}

	combinedKeys := make(map[string]int, len(combinedEntries))
	for _, entry := range combinedEntries {
		combinedKeys[logEntryKey(entry)]++
	}

	records := make([]sortableLogEntry, 0, len(legacyEntries)+len(combinedEntries))
	order := 0
	for _, entry := range legacyEntries {
		key := logEntryKey(entry)
		if combinedKeys[key] > 0 {
			combinedKeys[key]--
			continue
		}
		records = append(records, sortableLogEntry{
			entry:      entry,
			occurredAt: logEntryTime(entry),
			order:      order,
		})
		order++
	}
	for _, entry := range combinedEntries {
		records = append(records, sortableLogEntry{
			entry:      entry,
			occurredAt: logEntryTime(entry),
			order:      order,
		})
		order++
	}

	sort.SliceStable(records, func(i, j int) bool {
		left := records[i]
		right := records[j]
		if !left.occurredAt.IsZero() && !right.occurredAt.IsZero() && !left.occurredAt.Equal(right.occurredAt) {
			return left.occurredAt.Before(right.occurredAt)
		}
		return left.order < right.order
	})
	if len(records) > tail {
		records = records[len(records)-tail:]
	}

	entries := make([]logstore.Entry, 0, len(records))
	for _, record := range records {
		entries = append(entries, record.entry)
	}
	return entries, tailCursor, nil
}

func legacyLogEntries(process *pb.Process, tail int) ([]logstore.Entry, error) {
	stdoutEntries, err := plainLogEntries(process.LogFilePath, logstore.StdoutStream, tail)
	if err != nil {
		return nil, err
	}
	stderrEntries, err := plainLogEntries(process.ErrFilePath, logstore.StderrStream, tail)
	if err != nil {
		return nil, err
	}
	return append(stdoutEntries, stderrEntries...), nil
}

func plainLogEntries(filename, stream string, tail int) ([]logstore.Entry, error) {
	lines, _, err := logstore.ReadLinesWithCursor(filename, tail)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	entries := make([]logstore.Entry, 0, len(lines))
	for _, line := range lines {
		timestamp, message, ok := strings.Cut(line, ": ")
		if !ok || len(timestamp) != len("2006-01-02 15:04:05") {
			entries = append(entries, logstore.Entry{
				Stream: stream,
				Line:   line,
			})
			continue
		}
		entries = append(entries, logstore.Entry{
			Timestamp: timestamp,
			Stream:    stream,
			Line:      message,
		})
	}
	return entries, nil
}

func logEntryKey(entry logstore.Entry) string {
	return entry.Stream + "\x00" + logEntrySecond(entry) + "\x00" + entry.Line
}

func logEntrySecond(entry logstore.Entry) string {
	if parsed, err := time.Parse(time.RFC3339Nano, entry.Timestamp); err == nil {
		return parsed.Local().Format("2006-01-02 15:04:05")
	}
	return entry.Timestamp
}

func logEntryTime(entry logstore.Entry) time.Time {
	if parsed, err := time.Parse(time.RFC3339Nano, entry.Timestamp); err == nil {
		return parsed.Local()
	}
	if parsed, err := time.ParseInLocation("2006-01-02 15:04:05", entry.Timestamp, time.Local); err == nil {
		return parsed
	}
	return time.Time{}
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
