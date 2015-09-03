package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"strconv"
)

const (
	configJSONPath = "./config.json"
)

var (
	webhookBinPath  string
	port            string
	hooksPath       string
	notificationURL string
	repos           []string
)

func init() {
	file, err := os.Open(configJSONPath)
	defer file.Close()
	if err != nil {
		panic("Can't open config file.")
	}
	configContent, err := ioutil.ReadAll(file)
	if err != nil {
		panic("Can't read config file.")
	}

	config := make(map[string]interface{})
	json.Unmarshal(configContent, &config)

	fmt.Printf("%+v\n", config)

	port = fmt.Sprintf("%v", int64(config["port"].(float64)))
	hooksPath = config["hooks_path"].(string)
	webhookBinPath = config["hook_bin"].(string)
	repos = config["gopath_local_repos"].([]string)
}

func redeploy(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()
	content, err := ioutil.ReadAll(r.Body)
	fmt.Println("Content: ", string(content), ", Error: ", err)
	//notify(w, r)
}

func notify(w http.ResponseWriter, r *http.Request) {
	data := `payload={"channel": "#godev", "text": "This is posted to #godev and comes from a bot named webhookbot."}`
	req, _ := http.NewRequest("POST", notificationURL, bytes.NewBufferString(data))
	r.Header.Add("Content-Type", "application/x-www-form-urlencoded")
	r.Header.Add("Content-Length", strconv.Itoa(len(data)))
	_, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Println(err)
	}
}

func main() {
	http.HandleFunc(`/hooks/redeploy`, redeploy)
	http.ListenAndServe(":9000", nil)
}
