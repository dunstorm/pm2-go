package utils

import (
	"os"
	"path/filepath"
)

type Config struct {
	LogRotate         bool `json:"logrotate"`
	LogRotateSize     int  `json:"logrotate_size"`
	LogRotateMaxFiles int  `json:"logrotate_max_files"`
}

// find or create config file
func FindOrCreateConfigFile() string {
	userHomeDir, err := os.UserHomeDir()
	if err != nil {
		// Fallback or error handling if user home directory cannot be found
		// For simplicity here, we'll panic, but a real app might use a default local path.
		panic("could not get user home directory: " + err.Error())
	}
	configDir := filepath.Join(userHomeDir, ".pm2-go")
	configFile := filepath.Join(configDir, "config.json")

	// Ensure the directory exists
	if _, err := os.Stat(configDir); os.IsNotExist(err) {
		// Create the directory with 0755 permissions (similar to GetMainDirectory)
		if err := os.MkdirAll(configDir, 0755); err != nil {
			panic("could not create config directory: " + err.Error())
		}
	}

	if _, err := os.Stat(configFile); os.IsNotExist(err) {
		err := SaveObject(configFile, Config{
			LogRotate:         false,
			LogRotateSize:     10485760,
			LogRotateMaxFiles: 10,
		})
		if err != nil {
			panic(err)
		}
	}
	return configFile
}

// get config
func GetConfig() Config {
	var config Config
	err := LoadObject(FindOrCreateConfigFile(), &config)
	if err != nil {
		panic(err)
	}
	return config
}

// save config
func SaveConfig(config Config) {
	err := SaveObject(FindOrCreateConfigFile(), config)
	if err != nil {
		panic(err)
	}
}
