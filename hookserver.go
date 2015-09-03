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
	message        = `>>>*%v* pushed to *%v*\n *Message*: \"%v\".\n *Repo URL*: %v.\n *Commit URL*: %v.\n *Build status*: %v.`

	buildOK   = ":white_check_mark:"
	buildFail = ":sos:"
)

var (
	cfg config
)

type (
	config struct {
		Port            int    `json:"port"`
		HooksPath       string `json:"hooks_path"`
		NotificationURL string `json:"notification_url"`
		Repos           []struct {
			Path    string   `json:"path"`
			Scripts []string `json:"scripts"`
		} `json:"gopath_local_repos"`
	}

	githubResponse struct {
		HeadCommit struct {
			Timestamp string `json:"timestamp"`
			URL       string `json:"url"`
			Message   string `json:"message"`
			Committer struct {
				Name string `json:"name"`
			} `json:"committer"`
		} `json:"head_commit"`
		Repository struct {
			Name string `json:"full_name"`
			URL  string `json:"url"`
		} `json:"repository"`
	}
)

func init() {
	file, err := os.Open(configJSONPath)
	defer file.Close()
	if err != nil {
		panic("Can't open config file.")
	}
	configContent, err := ioutil.ReadAll(file)
	fmt.Println(string(configContent))
	if err != nil {
		panic("Can't read config file.")
	}
	err = json.Unmarshal(configContent, &cfg)
	if err != nil {
		panic("Can't parse config file as json: " + err.Error())
	}
	fmt.Printf("%+v\n", cfg)
}

func redeploy(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()
	content, _ := ioutil.ReadAll(r.Body)
	resp := &githubResponse{}
	json.Unmarshal(content, resp)
	notify(resp)

}

func notify(r *githubResponse) {
	filledMessage := fmt.Sprintf(
		message,
		r.HeadCommit.Committer.Name,
		r.Repository.Name,
		r.HeadCommit.Message,
		r.Repository.URL,
		r.HeadCommit.URL,
		":suspect:",
	)
	data := fmt.Sprintf(`payload={"channel": "#godev", "text": "%v"}`, filledMessage)
	req, _ := http.NewRequest("POST", cfg.NotificationURL, bytes.NewBufferString(data))
	req.Header.Add("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Add("Content-Length", strconv.Itoa(len(data)))
	_, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Println(err)
	}
}

func main() {
	http.HandleFunc(`/hooks/redeploy`, redeploy)
	http.ListenAndServe(fmt.Sprintf(":%v", cfg.Port), nil)
}
