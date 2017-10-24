package main

import (
	"os"
	"flag"
	"net/http"
	"log"
	"encoding/json"
	"strconv"
)

const (
	ACTION_START_LISTEN string = "start"
	ACTION_GENPROFILE string = "genprofile"
)

var detection *Detector
var defaultLanguages []string

type Cfg struct {
	Action string
	XmlPath string
	ProfilePath string
	Profile string
	Languages []string
	Port uint
}

var defaultConfig = Cfg {
	Action: "start",
	XmlPath: "xml",
	ProfilePath: "profiles",
	Profile: "main",
	Languages: []string{"all"},
	Port: 3000,
}

type httpRequestStruct struct {
	Text      string   `json:"text"`
	Languages []string `json:"langs"`
	MaxTrials int	   `json:"max_trials"`
	MaxIterations uint8	   `json:"max_iterations"`
}

func init() {
	flag.StringVar(&defaultConfig.ProfilePath, "profile-path", "profiles", "Path to profiles directory")
	flag.StringVar(&defaultConfig.XmlPath, "xml-path", "xml", "Path to wiki abstract DB directory")
	flag.StringVar(&defaultConfig.Profile, "profile", "main", "Profile name to be loaded")
	flag.UintVar(&defaultConfig.Port, "port", 3000, "Port of HTTP listener")

	flag.Parse()
	defaultConfig.Action = flag.Arg(0)
	if len(defaultConfig.Action) == 0 {
		defaultConfig.Action = ACTION_START_LISTEN
	}


	detectConfig := DetectConfig{
		Profile: defaultConfig.Profile,
		XmlPath: defaultConfig.XmlPath,
		ProfilePath: defaultConfig.ProfilePath,
		Languages: defaultConfig.Languages,
	}

	detection = NewDetector(detectConfig)
	defaultLanguages = make([]string, len(detection.classes),len(detection.classes))

	counter := 0
	for lang, _ := range detection.classes {
		defaultLanguages[counter] = lang
		counter++
	}
}

func main() {

	if defaultConfig.Action == ACTION_GENPROFILE {
		detection.GenerateProfileFromWikiXML()
		detection.SaveProfiles()
		os.Exit(0)
	}

	if defaultConfig.Action == ACTION_START_LISTEN {
		http.HandleFunc("/", detectLanguageHandler)
		log.Fatal(http.ListenAndServe(":" + strconv.Itoa(int(defaultConfig.Port)), nil))
	}
}

func detectLanguageHandler(rw http.ResponseWriter, req *http.Request) {
	defer req.Body.Close()

	rw.Header().Set("Content-Type", "application/json")

	var err error

	requestData := httpRequestStruct{}
	decoder := json.NewDecoder(req.Body)
	err = decoder.Decode(&requestData)
	if err != nil {
		http.Error(rw, err.Error(), http.StatusBadRequest)
		return
	}

	if len(requestData.Languages) == 0 || (len(requestData.Languages) == 1 && requestData.Languages[0] == ALL_LANGUAGES )  {
		requestData.Languages = defaultLanguages
	}

	response := detection.Detect(requestData.Text, requestData.Languages, requestData.MaxTrials, requestData.MaxIterations);

	err = json.NewEncoder(rw).Encode(response)
	if err != nil {
		http.Error(rw, err.Error(), http.StatusInternalServerError)
		return
	}
}
