package backup

import (
	"dectmgr/misc"
	"encoding/json"
	"html/template"
	"io/ioutil"
	"net/http"
	"os"
	"path"
	"strconv"
	"strings"

	"github.com/gorilla/mux"
	"github.com/op/go-logging"
)

var log = logging.MustGetLogger("dectmgr")
var logFormat = logging.MustStringFormatter(
	"%{color}%{time:15:04:05.000} %{level:.3s} %{id:03x}%{color:reset} %{message}",
)

type webservice struct {
	appconfig misc.AppConfiguration
	fm        backupManager
	//router    *gin.Engine
	router    *mux.Router
	templates *template.Template
}

func (w *webservice) Setup() {
	//gin.SetMode(gin.ReleaseMode)
	logBackendStdout := logging.NewLogBackend(os.Stdout, "", 0)
	logLevelInt, errLogLevel := logging.LogLevel(w.appconfig.LogLevel)
	if errLogLevel != nil {
		logLevelInt = logging.DEBUG
	}
	logLeveled := logging.AddModuleLevel(logBackendStdout)
	logLeveled.SetLevel(logLevelInt, "")

	//backendFormatter := logging.NewBackendFormatter(logLeveled, logFormat)
	logging.SetBackend(logLeveled)
	logging.SetFormatter(logFormat)
	if errLogLevel != nil {
		log.Critical("Log level not understood, fallback to DEBUG")
	}
	log.Debug("Start logging. Log level: %s", logLevelInt.String())

	log.Debug("Instanciate HTTP")
	//w.router = gin.Default()
	w.router = mux.NewRouter()

	log.Debug("Register routes")

	w.router.HandleFunc("/update-IPBS2.htm", w.GetUpdateHandler(w.appconfig.ConfigBackupURL)).Methods("GET")
	w.router.HandleFunc("/update-IPBS.htm", w.GetUpdateHandler(w.appconfig.ConfigBackupURL)).Methods("GET")
	w.router.HandleFunc("/update-IPBL.htm", w.GetUpdateHandler(w.appconfig.ConfigBackupURL)).Methods("GET")
	w.router.HandleFunc("/backup/{hwid}", w.UploadConfigHandler(".")).Methods("PUT")

	w.templates = template.Must(template.ParseGlob("templates/*"))
	w.router.HandleFunc("/api/{hwid}/config/{revision}", func(rw http.ResponseWriter, req *http.Request) {
		hwid := mux.Vars(req)["hwid"]
		revision := mux.Vars(req)["revision"]
		log.Debug("[%s] GET Request for config of revision %s", hwid, revision)
		config, err := w.fm.GetConfigFile(hwid, revision)
		if err != nil {
			http.NotFound(rw, req)
		}
		rw.Write([]byte(config))
	})
	w.router.HandleFunc("/api/{hwid}/info", func(rw http.ResponseWriter, req *http.Request) {
		hwid := mux.Vars(req)["hwid"]
		log.Debug("[%s] GET request for info file", hwid)
		config, err := w.fm.LoadConfig(hwid)
		if err != nil {
			http.NotFound(rw, req)
		}
		jsonResponse, err := json.Marshal(config)
		if err != nil {
			http.Error(rw, err.Error(), http.StatusInternalServerError)
			return
		}

		rw.Header().Set("Content-Type", "application/json")
		rw.Write(jsonResponse)
	})
	w.router.HandleFunc("/api/all", func(rw http.ResponseWriter, req *http.Request) {
		token := req.URL.Query().Get("query")
		log.Debug("Search request for query '%v'", token)
		result := w.fm.Search(token)
		jsonResponse, err := json.Marshal(result)
		if err != nil {
			http.NotFound(rw, req)
		}
		rw.Header().Set("Content-Type", "application/json")
		rw.Write(jsonResponse)
	})

	// UI Stuff
	w.router.HandleFunc("/", w.UIPageHandler("index")).Methods("GET")
	w.router.HandleFunc("/index", w.UIPageHandler("index")).Methods("GET")
	w.router.HandleFunc("/details", w.UIPageHandler("details")).Methods("GET")
	w.router.PathPrefix("/").Handler(http.FileServer(http.Dir("./ui/")))

}

func NewWebservice(appconfig misc.AppConfiguration) *webservice {
	webservice := &webservice{}
	webservice.appconfig = appconfig
	webservice.fm = NewBackupmanager(appconfig)
	return webservice
}

type globalDataStruct struct {
	AppTitle   string
	AppVersion string
}
type requestDataStruct struct {
	MenuSelection string
	SubTemplate   string
}
type indexPageDataStruct struct {
}

func (w *webservice) getGlobalInfo() globalDataStruct {
	return globalDataStruct{
		AppTitle:   "DECT Backup Mgr",
		AppVersion: "0.1",
	}
}

func (w *webservice) UIPageHandler(section string) func(http.ResponseWriter, *http.Request) {
	return func(rw http.ResponseWriter, req *http.Request) {
		var details interface{}
		log.Debug("Section is: %v", section)
		switch section {
		case "details":
			hardwareID := req.URL.Query().Get("id")
			backup, err := w.fm.LoadConfig(hardwareID)
			if err != nil {
				log.Error("Error in UI Handler")
				http.Error(rw, "Not found", 404)
				return
			}
			details = backup

		case "index":
			details = struct {
				Something string
			}{
				Something: "else",
			}
		}

		data := struct {
			Globals globalDataStruct
			Request requestDataStruct
			Details interface{}
		}{
			Globals: w.getGlobalInfo(),
			Request: requestDataStruct{
				MenuSelection: section,
				SubTemplate:   section,
			},
			Details: details,
		}
		log.Debug("ExecuteTemplate %s", section)

		err := w.templates.ExecuteTemplate(rw, "mainTemplate", data)
		if err != nil {
			log.Error("Error executing template occurred: %v", err)
		}
	}
}

func (w *webservice) GetUpdateHandler(backupUrl string) func(http.ResponseWriter, *http.Request) {
	return func(rw http.ResponseWriter, req *http.Request) {
		log.Debug("New Updaterequest from: %s", req.RemoteAddr)
		response :=
			"mod cmd UP0 scfg " + backupUrl + "\n" +
				"config write\n" +
				"config activate\n"
		rw.Write([]byte(response))
	}
}

func (w *webservice) UploadConfigHandler(location string) func(http.ResponseWriter, *http.Request) {
	return func(rw http.ResponseWriter, req *http.Request) {

		filename := mux.Vars(req)["hwid"]
		extension := path.Ext(filename)
		hwid := filename[0 : len(filename)-len(extension)]
		ipaddress := strings.Split(req.RemoteAddr, ":")[0]
		log.Info("Incoming config from %s (%s)", hwid, req.RemoteAddr)

		//var config, err = ioutil.ReadAll(c.Request.Body)
		var config, err = ioutil.ReadAll(req.Body)
		newConfig := string(config[:])

		if err != nil {
			log.Error("Some error occurred with the current config. %s", ipaddress)
		}
		configObj := w.fm.CreateHistoryEntry(newConfig)
		configObj.IPAddress = ipaddress

		log.Debug("[%s] Config update", hwid)
		w.fm.InsertConfig(hwid, configObj)
		rw.Write([]byte(""))
	}
}

func (w *webservice) Run() {
	portNo := strconv.Itoa(w.appconfig.ListenPort)
	log.Info("Run HTTP server on port " + portNo)
	http.Handle("/", w.router) // listen and serve on 0.0.0.0:8080
	http.ListenAndServe(":"+portNo, nil)
}
