/*
Copyright © 2022 NAME HERE <EMAIL ADDRESS>

*/
package cmd

import (
	"os"

	"github.com/fatih/color"
	"github.com/jedib0t/go-pretty/v6/table"
	"github.com/jedib0t/go-pretty/v6/text"
	"github.com/spf13/cobra"
)

func renderProcessList() {
	t := table.NewWriter()

	cyanBold := color.New(color.FgCyan, color.Bold).SprintFunc()
	t.AppendHeader(table.Row{
		cyanBold("#"),
		cyanBold("name"),
		cyanBold("pid"),
		cyanBold("status"),
		cyanBold("uptime"),
		cyanBold("↺"),
		cyanBold("cpu"),
		cyanBold("memory"),
	})
	t.SetOutputMirror(os.Stdout)
	t.SetIndexColumn(1)

	t.SetStyle(table.StyleLight)
	t.Style().Format.Header = text.FormatLower

	greenBold := color.New(color.FgGreen, color.Bold).SprintFunc()
	redBold := color.New(color.FgRed, color.Bold).SprintFunc()

	for i, p := range master.ListProcs() {
		p.UpdateCPUMemory()
		if p.ProcStatus.Status == "online" {
			p.ProcStatus.Status = greenBold("online")
		} else {
			p.ProcStatus.Status = redBold(p.ProcStatus.Status)
		}
		t.AppendRow(table.Row{
			i, p.Name, p.Pid, p.ProcStatus.Status, p.ProcStatus.Uptime, p.ProcStatus.Restarts, p.ProcStatus.CPU, p.ProcStatus.Memory,
		})
	}

	t.Render()
}

// lsCmd represents the ls command
var lsCmd = &cobra.Command{
	Use:   "ls",
	Short: "list all processes",
	Long:  "list all processes",
	Run: func(cmd *cobra.Command, args []string) {
		master.SpawnDaemon()
		renderProcessList()
	},
}

func init() {
	rootCmd.AddCommand(lsCmd)

	// Here you will define your flags and configuration settings.

	// Cobra supports Persistent Flags which will work for this command
	// and all subcommands, e.g.:
	// lsCmd.PersistentFlags().String("foo", "", "A help for foo")

	// Cobra supports local flags which will only run when this command
	// is called directly, e.g.:
	// lsCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")
}
