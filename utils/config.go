package utils

import "os"

type Config struct {
	LogRotate         bool `json:"logrotate"`
	LogRotateSize     int  `json:"logrotate_size"`
	LogRotateMaxFiles int  `json:"logrotate_max_files"`
}

// find or create config file
func FindOrCreateConfigFile() string {
	configFile := os.Getenv("HOME") + "/.pm2-go/config.json"
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
