package main

import (
	"dectmgr/backup"
	"dectmgr/misc"
	"fmt"
)

func main() {
	appconfig, err := misc.ReadConfig()
	if err != nil {
		fmt.Println("ERROR reading config file: ", err)
	}
	ws := backup.NewWebservice(appconfig)
	ws.Setup()
	ws.Run()
}
