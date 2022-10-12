/*
Copyright © 2022 NAME HERE <EMAIL ADDRESS>

*/
package cmd

import (
	"fmt"

	"github.com/dunstorm/pm2-go/utils"
	"github.com/spf13/cobra"
)

// configCmd represents the config command
var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Set/Get configuration values",
	Long: `Set/Get configuration values. For example:
	
	pm2-go config set logrotate true
	pm2-go config get logrotate`,
	Run: func(cmd *cobra.Command, args []string) {
		// find or create config file
		utils.FindOrCreateConfigFile()

		// get config
		config := utils.GetConfig()

		// check if args are set
		if len(args) == 0 {
			// print config
			fmt.Println("log_rotate:", config.LogRotate)
			fmt.Println("log_rotate_max_files:", config.LogRotateMaxFiles)
			fmt.Println("log_rotate_size:", config.LogRotateSize)
			return
		}
	},
}

var configSetCmd = &cobra.Command{
	Use:   "set",
	Short: "Set configuration values",
	Long: `Set configuration values. For example:

	pm2-go config set logrotate true
	pm2-go config set logrotate_max_files 10
	pm2-go config set logrotate_size 10M`,
	Run: func(cmd *cobra.Command, args []string) {
		// find or create config file
		utils.FindOrCreateConfigFile()

		// get config
		config := utils.GetConfig()

		// check if args are set
		if len(args) < 2 {
			fmt.Println("Please provide a key and value")
			return
		}

		logger := master.GetLogger()

		// set config
		switch args[0] {
		case "logrotate":
			// check if value is yes or no
			config.LogRotate = utils.ParseBool(args[1])
			logger.Info().Msgf("LogRotate has been set to %v", config.LogRotate)
		case "logrotate_max_files":
			maxFiles := utils.ParseInt(args[1])
			if maxFiles < 0 {
				logger.Error().Msg("logrotate_max_files must be a positive integer")
				return
			}
			config.LogRotateMaxFiles = maxFiles
			logger.Info().Msgf("LogRotateMaxFiles has been set to %d", config.LogRotateMaxFiles)
		case "logrotate_size":
			config.LogRotateSize = utils.ParseSize(args[1])
			logger.Info().Msgf("LogRotateSize has been set to %d bytes", config.LogRotateSize)
		default:
			logger.Error().Msgf("Unknown key")
			logger.Info().Msgf("Available keys: logrotate, logrotate_max_files, logrotate_size")
			return
		}

		// save config
		utils.SaveConfig(config)

		logger.Debug().Msgf("Config saved")
	},
}

func init() {
	rootCmd.AddCommand(configCmd)
	configCmd.AddCommand(configSetCmd)

	// Here you will define your flags and configuration settings.

	// Cobra supports Persistent Flags which will work for this command
	// and all subcommands, e.g.:
	// configCmd.PersistentFlags().String("foo", "", "A help for foo")

	// Cobra supports local flags which will only run when this command
	// is called directly, e.g.:
	// configCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")
}
