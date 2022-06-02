/*
Copyright © 2022 NAME HERE <EMAIL ADDRESS>

*/
package cmd

import (
	"os"

	"github.com/jedib0t/go-pretty/v6/table"
	"github.com/spf13/cobra"
)

// lsCmd represents the ls command
var lsCmd = &cobra.Command{
	Use:   "ls",
	Short: "list all processes",
	Long:  "list all processes",
	Run: func(cmd *cobra.Command, args []string) {
		t := table.NewWriter()
		t.AppendHeader(table.Row{"#", "Name", "PID", "Status", "Uptime", "↺", "CPU", "Memory"})
		t.SetOutputMirror(os.Stdout)
		t.SetIndexColumn(1)

		t.Style().Box.PaddingLeft = " "
		t.Style().Box.PaddingRight = " "

		for i, p := range master.ListProcs() {
			p.UpdateCPUMemory()
			t.AppendRow(table.Row{
				i, p.Name, p.Pid, p.ProcStatus.Status, p.ProcStatus.Uptime, p.ProcStatus.Restarts, p.ProcStatus.CPU, p.ProcStatus.Memory,
			})
		}

		t.SetStyle(table.StyleLight)
		t.Render()
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
