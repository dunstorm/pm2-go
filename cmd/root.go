/*
Copyright © 2022 NAME HERE <EMAIL ADDRESS>

*/
package cmd

import (
	"fmt"
	"os"

	app "github.com/dunstorm/pm2-go/app"
	"github.com/spf13/cobra"
)

var master = app.New()

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "pm2-go",
	Short: "Production process manager written in Go",
	Long: `PM2-GO is a production process manager for any application with a built-in load balancer (WIP).
It allows you to keep applications alive forever, to reload them without downtime and to facilitate common system admin tasks.`,
	// Uncomment the following line if your bare application
	// has an action associated with it:
	Run: func(cmd *cobra.Command, args []string) {
		if daemon, _ := cmd.PersistentFlags().GetBool("daemon"); daemon {
			master.SpawnDaemon()
			return
		}
		if version, _ := cmd.PersistentFlags().GetBool("version"); version {
			fmt.Println("0.1.1")
			return
		}
		cmd.Usage()
	},
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	err := rootCmd.Execute()
	if err != nil {
		os.Exit(1)
	}
}

func init() {
	// Here you will define your flags and configuration settings.
	// Cobra supports persistent flags, which, if defined here,
	// will be global for your application.

	// rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is $HOME/.pm2-go.yaml)")

	// Cobra also supports local flags, which will only run
	// when this action is called directly.
	rootCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")
	rootCmd.PersistentFlags().BoolP("daemon", "d", false, "Run as daemon")
	rootCmd.PersistentFlags().BoolP("version", "v", false, "Print pm2 version")
	logsCmd.PersistentFlags().IntP("lines", "l", 15, "Number of lines to tail")
}
