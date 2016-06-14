package misc

import (
	"encoding/json"
	"os"
)

type AppConfiguration struct {
	ConfigBackupURL   string
	ListenPort        int
	BackupDestination string
	MaxNoBackups      int
	LogLevel          string
}

func ReadConfig() (AppConfiguration, error) {
	file, _ := os.Open("config.json")
	decoder := json.NewDecoder(file)
	config := AppConfiguration{}
	err := decoder.Decode(&config)
	return config, err
}
