/*
Copyright © 2022 NAME HERE <EMAIL ADDRESS>

*/
package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/fatih/color"
	"github.com/jedib0t/go-pretty/v6/table"
	"github.com/spf13/cobra"
)

// describeCmd represents the describe command
var describeCmd = &cobra.Command{
	Use:   "describe",
	Short: "describe all parameters of a process",
	Long:  `describe all parameters of a process`,
	Run: func(cmd *cobra.Command, args []string) {
		if len(os.Args) < 1 {
			cmd.Usage()
			return
		}

		master.SpawnDaemon()
		logger := master.GetLogger()

		process := master.FindProcess(args[0])
		if process == nil {
			logger.Error().Msg("Process not found")
			return
		}

		heading := color.New(color.FgWhite, color.BgWhite, color.Bold).PrintfFunc()
		// Describing process with id - name
		heading("Process with id %d - name %s", process.Id, process.Name)
		fmt.Println()

		cyanBold := color.New(color.FgCyan, color.Bold).SprintFunc()
		greenBold := color.New(color.FgGreen, color.Bold).SprintFunc()
		redBold := color.New(color.FgRed, color.Bold).SprintFunc()

		t := table.NewWriter()
		t.SetOutputMirror(os.Stdout)
		t.SetStyle(table.StyleLight)

		// status, name, restarts, autorestart, executable path, executable args, error log path, out log path, pid file path, cron expression, next launch time

		if process.ProcStatus.Status == "online" {
			t.AppendRow([]interface{}{cyanBold("status"), greenBold(process.ProcStatus.Status)})
		} else {
			t.AppendRow([]interface{}{cyanBold("status"), redBold(process.ProcStatus.Status)})
		}

		t.AppendRow(table.Row{
			cyanBold("name"), process.Name,
		})

		t.AppendRow(table.Row{
			cyanBold("restarts"), process.ProcStatus.Restarts,
		})

		t.AppendRow(table.Row{
			cyanBold("autorestart"), process.AutoRestart,
		})

		t.AppendRow(table.Row{
			cyanBold("executable path"), process.ExecutablePath,
		})

		t.AppendRow(table.Row{
			cyanBold("executable args"), strings.Join(process.Args, " "),
		})

		t.AppendRow(table.Row{
			cyanBold("error log path"), process.ErrFilePath,
		})

		t.AppendRow(table.Row{
			cyanBold("out log path"), process.LogFilePath,
		})

		t.AppendRow(table.Row{
			cyanBold("pid file path"), process.PidFilePath,
		})

		t.AppendRow(table.Row{
			cyanBold("cron expression"), process.CronRestart,
		})

		t.AppendRow(table.Row{
			cyanBold("next launch time"), process.NextStartAt.AsTime().Local().Format("2006-01-02 15:04:05 -07:00 MST"),
		})

		t.Render()
	},
}

func init() {
	rootCmd.AddCommand(describeCmd)

	// Here you will define your flags and configuration settings.

	// Cobra supports Persistent Flags which will work for this command
	// and all subcommands, e.g.:
	// describeCmd.PersistentFlags().String("foo", "", "A help for foo")

	// Cobra supports local flags which will only run when this command
	// is called directly, e.g.:
	// describeCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")
}
