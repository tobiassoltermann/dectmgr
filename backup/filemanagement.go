package backup

import (
	"crypto/md5"
	"dectmgr/misc"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

type HistoryItem struct {
	Md5Hash     string    `json:"hash"`
	config      string    `json:"-"`
	IPAddress   string    `json:"ipaddress"`
	Hostname    string    `json:"hostname"`
	MasterIP    string    `json:"masterip"`
	LastChanged time.Time `json:"lastchanged"`
	LastUpdate  time.Time `json:"lastupdate"`
}

func (b *HistoryItem) SetConfig(config string) {
	b.config = config
	b.Md5Hash = fmt.Sprintf("%x", md5.Sum([]byte(config)))

	// Find hostname in the new config
	reHostname := regexp.MustCompile("config change CMD0 /name ([^\\s]*)\r?")
	hostnameResult := reHostname.FindStringSubmatch(b.config)
	if len(hostnameResult) >= 2 {
		b.Hostname = hostnameResult[1]
	} else {
		b.Hostname = "unknown"
	}

	reMasterIP := regexp.MustCompile("config change GW-DECT MASTER .* /mode ACTIVE")
	masterIPResult := reMasterIP.FindStringSubmatch(b.config)
	if len(masterIPResult) == 0 {
		reRadioMasterIP := regexp.MustCompile(`config change GW-DECT RADIO .*/master ([^\s]*)`)
		resultRadioMasterIP := reRadioMasterIP.FindStringSubmatch(string(b.config))
		if len(resultRadioMasterIP) == 0 {
			b.MasterIP = "unknown"
		} else {
			b.MasterIP = resultRadioMasterIP[1]
		}
	} else {
		b.MasterIP = b.IPAddress
	}

}

type ConfigObject struct {
	HardwareID string         `json:"hardwareid"`
	History    []*HistoryItem `json:"history"`
}

type backupManager struct {
	appconfig misc.AppConfiguration
}

func NewBackupmanager(config misc.AppConfiguration) backupManager {
	return backupManager{config}
}

func FileExists(filename string) bool {
	if _, err := os.Stat(filename); os.IsNotExist(err) {
		return false
	}
	return true
}

func formatTime(t time.Time) string {
	return t.Format("2006-01-02T15:04:05.371Z")
}

func (b backupManager) LoadConfig(hwid string) (ConfigObject, error) {
	log.Debug("[%s] Load config", hwid)
	devicePath := filepath.Join(b.appconfig.BackupDestination, hwid)
	infoFile := filepath.Join(devicePath, "info.json")
	// Check if info file exists
	if FileExists(infoFile) {
		// Read info file
		bytes, err := ioutil.ReadFile(infoFile)
		if err != nil {
			log.Error("[%s] Error reading infofile. %v", err)
			return ConfigObject{}, errors.New("File for hwid " + hwid + " does not exist")
		}
		// Parse info file
		var config ConfigObject = ConfigObject{}
		decoder := json.NewDecoder(strings.NewReader(string(bytes)))
		err = decoder.Decode(&config)
		if err != nil {
			log.Error("[%s] Could not parse infofile")
			return ConfigObject{}, errors.New("File for hwid " + hwid + " found, but couldn't be parsed")
		}

		// Info file parsed. Fill up with configuration data
		for key, value := range config.History {
			oldMd5Hash := value.Md5Hash
			configFile, err := b.GetConfigFile(config.HardwareID, strconv.Itoa(key))
			if err != nil {
				log.Warning("[%s] Entry for config in revision %v is present while config file itself isn't.", hwid, key)
			}
			value.SetConfig(configFile)
			newMd5Hash := value.Md5Hash
			if newMd5Hash != oldMd5Hash {
				log.Warning("[%s] Config hashes don't match! Old config hash '%v', new config hash '%v'.", oldMd5Hash, newMd5Hash)
			}
		}

		return config, nil
	}
	return ConfigObject{}, errors.New("Config for " + hwid + " doesn't exist")
}

func (b backupManager) Search(token string) (result []string) {
	allItems := b.getAllHardwareIDs()

	for _, item := range allItems {
		config, err := b.LoadConfig(item)
		crtRes := false
		if err == nil {
			for _, historyItem := range config.History {
				tmp := config.HardwareID + "|" + historyItem.Hostname + "|" + historyItem.IPAddress + "|" + formatTime(historyItem.LastChanged) + "|" + formatTime(historyItem.LastUpdate) + "|" + historyItem.MasterIP + "|" + historyItem.Md5Hash
				log.Debug("tmp is '%v'", tmp)
				if CaseInsensitiveContains(tmp, token) {
					//if strings.Contains(tmp, token) {
					crtRes = true
				}
			}
			if crtRes {
				log.Debug("[search '%v'] adding item '%v'", token, item)
				result = append(result, config.HardwareID)
			}
		}
	}
	RemoveDuplicates(&result)

	return result
}

func (b backupManager) getAllHardwareIDs() (result []string) {
	files, _ := ioutil.ReadDir(b.appconfig.BackupDestination)
	for _, f := range files {
		result = append(result, f.Name())
	}
	return
}

func (b backupManager) SaveConfig(config ConfigObject) {
	log.Debug("[%s] Save config", config.HardwareID)
	hwid := config.HardwareID
	devicePath := filepath.Join(b.appconfig.BackupDestination, hwid)
	infoFile := filepath.Join(devicePath, "info.json")

	file, err := os.Create(infoFile)
	if err != nil {
		log.Error("[%s] Error open info file for write mode: %v", hwid, err)
		return
	}
	encoder := json.NewEncoder(file)
	err = encoder.Encode(config)
	if err != nil {
		log.Error("[%s] Error encoding info file to json: %v", hwid, err)
		return
	}
	for index, historyItem := range config.History {
		configFolder := filepath.Join(devicePath, strconv.Itoa(index))
		if !FileExists(configFolder) {
			os.Mkdir(configFolder, 0777)
		}
		newFilepath := filepath.Join(configFolder, "config.txt")
		ioutil.WriteFile(newFilepath, []byte(historyItem.config), 0777)
	}

	// Walk all existing config folders and keep only the maximum number of backups configured in the config file
	existingFiles, err := ioutil.ReadDir(devicePath)
	if err != nil {
		log.Error("[%s] Error listing existing configurations: %s", hwid, err)
	} else {
		for _, file := range existingFiles {
			fileName := file.Name()
			if fileName != "info.json" {
				fileNo, err2 := strconv.Atoi(fileName)
				if err2 != nil {
					log.Error("[%s] Cannot convert file name %s to number.", hwid, fileName)
				} else {
					if fileNo >= b.appconfig.MaxNoBackups {
						removeFilesPath := filepath.Join(devicePath, fileName)
						log.Debug("[%s] Remove files %s ", removeFilesPath)
						os.RemoveAll(removeFilesPath)
					}
				}
			}
		}
	}

	file.Close()
}

func (b backupManager) CreateConfig(hwid string) ConfigObject {
	backup := ConfigObject{}
	backup.HardwareID = hwid
	devicePath := filepath.Join(b.appconfig.BackupDestination, hwid)
	err := os.MkdirAll(devicePath, 0777)
	if err != nil {
		log.Critical("[%s] Could not create config directory '%v'", hwid, devicePath)
	}
	return backup
}

func (b backupManager) CreateHistoryEntry(config string) HistoryItem {

	now := time.Now()
	historyItem := HistoryItem{}
	historyItem.SetConfig(config)
	historyItem.LastUpdate = now
	historyItem.LastChanged = now
	return historyItem
}

func (b backupManager) GetConfigFile(hwid string, revision string) (string, error) {
	log.Debug("[%s] Get config revision %v", hwid, revision)
	devicePath := filepath.Join(b.appconfig.BackupDestination, hwid)
	configPath := filepath.Join(devicePath, revision, "config.txt")
	log.Debug("[%s] Check file existence: %v", hwid, configPath)
	if FileExists(configPath) {
		bytesFile, err := ioutil.ReadFile(configPath)
		if err != nil {
			return "", err
		}
		return string(bytesFile), nil
	} else {
		return "", errors.New("Doesn't exist")
	}
}
func (b backupManager) InsertConfig(hwid string, itemToAdd HistoryItem) {
	log.Debug("[%s] Check if device folder exists", hwid)
	backup, err := b.LoadConfig(hwid)
	if err != nil {
		log.Info("[%s] Config doesn't exist yet, create one.", hwid)
		backup = b.CreateConfig(hwid)
	}
	if len(backup.History) <= 0 {
		// This is the first config anyway, just store it and we're done.
		log.Info("[%s] No config there yet. Append new config")
		backup.History = append(backup.History, &itemToAdd)
	} else {
		// We already have some other configs.
		latestConfig := backup.History[0]
		log.Info("[%s] Check if the incoming config is newer than the latest stored one", hwid)
		log.Debug("[%s] MD5. new [%v], old [%v]", hwid, itemToAdd.Md5Hash, latestConfig.Md5Hash)
		if itemToAdd.Md5Hash == latestConfig.Md5Hash {
			log.Debug("[%s] Config has NOT changed. Update timestamp", hwid)
			latestConfig.LastChanged = time.Now()
		} else {
			log.Debug("[%s] Config HAS changed. Insert the new config", hwid)
			// Append new config to the front
			backup.History = insertAt(backup.History, &itemToAdd, 0)
			// and remove the oldest config
			backup.History = backup.History[:len(backup.History)-1]
		}
	}
	b.SaveConfig(backup)
}

func insertAt(s []*HistoryItem, item *HistoryItem, i int) []*HistoryItem {
	s = append(s, nil)
	copy(s[i+1:], s[i:])
	s[i] = item
	return s
}

func CaseInsensitiveContains(s, substr string) bool {
	s, substr = strings.ToUpper(s), strings.ToUpper(substr)
	return strings.Contains(s, substr)
}

func RemoveDuplicates(xs *[]string) {
	found := make(map[string]bool)
	j := 0
	for i, x := range *xs {
		if !found[x] {
			found[x] = true
			(*xs)[j] = (*xs)[i]
			j++
		}
	}
	*xs = (*xs)[:j]
}
