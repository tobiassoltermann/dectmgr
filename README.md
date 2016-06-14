# dectmgr
Provides an interface for automated backups of Ascom DECT infrastructure devices (IPBL/IPBS/IPBS2)

Dashboard lists all backups available for every device ever seen
![index](/readme/index.png)

A click on an item provides with all the backups from this device as well as a simple diff mechanism
![details](/readme/details.png)

# Installation

Checkout the repository, compile and run.

```bash
git clone https://github.com/tobiassoltermann/dectmgr.git
cd dectmgr
go build
```

On Windows, run
```
dectmgr.exe
```

On Linux/Unix, run
```
./dectmgr
```

All log goes to stdout. You can use an init script as follows:
```
description     "Config backup manager for Ascom IP-DECT"

stop on runlevel [!2345]

respawn

script
        cd /path/to/dectmgr
        ./dectmgr >> /var/log/dectmgr.log
end script
```

## Config

The configuration is based on a simple config file (config.json)
```json
{
	"ConfigBackupURL"	:	"http://YourIP:8080/backup/#h.txt",
	"ListenPort"		:	8080,
	"BackupDestination"	:	"backups/",
	"MaxNoBackups"		:	5,
	"Loglevel"			:	"INFO"
}

Option name | Description
----------- | -----------
ConfigBackupURL | An URL to the host running this tool. This value should have `/backup/#h.txt` postfixed to it. This address should be reachable by the antennas (keep an eye on the firewall rules).
ListenPort | Defines the port this tool should listen to. The port should correspond with the port specified in ConfigBackupURL.
BackupDestination | This is a relative folder path in which the backup data should be stored. The folder is created for you if it doesn't exist as soon as the first antenna is backupped.
MaxNoBackups | Older backups are deleted as soon as a newer arrives. This number specifies how many backup versions to keep.
Loglevel | Specifies the log level and is one of the following strings: "CRITICAL", "ERROR", "WARNING", "NOTICE", "INFO", "DEBUG".
```

# Configuration of Ascom devices

## Manual approach

Enter the destination URL as shown below directly in the web administration interface of your Ascom DECT infrastructure.
* In older software releases, the option shows up in : General -> Update
* In newer software releases, the option shows up in : Services -> Update

Don't forget to specify an Interval.

![index](/readme/ascom.png)

## DHCP option

For both options, you can specify DHCP options:

Option code | Sub-option code | Name | Description
----------- | --------------- | ---- | -----------
60 | 215 | updateurl | Specifies the update URL of your dectmgr. Don't forget to specify the port if it's different from 80.
60 | 216 | updatepollinterval | Specifies the interval in minutes at which the antenna contacts dectmgr
