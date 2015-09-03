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

	message = `
	%v: %v pushed to %v, message:"%v".
	Repo URL: %v.
	Commit URL: %v.
	Build status: %v
	`
)

var (
	webhookBinPath  string
	port            string
	hooksPath       string
	notificationURL string
	repos           []string
)

type (
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
			URL  string `json:"https://github.com/geekbros/go-ci"`
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

	config := make(map[string]interface{})
	err = json.Unmarshal(configContent, &config)
	if err != nil {
		panic("Can't parse config file as json: " + err.Error())
	}
	fmt.Printf("%+v\n", config)
	notificationURL = config["notification_url"].(string)
	port = fmt.Sprintf("%v", int64(config["port"].(float64)))
	hooksPath = config["hooks_path"].(string)
	webhookBinPath = config["hook_bin"].(string)
	for _, v := range config["gopath_local_repos"].([]interface{}) {
		repos = append(repos, v.(string))
	}
}

func redeploy(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()
	content, _ := ioutil.ReadAll(r.Body)
	resp := &githubResponse{}
	json.Unmarshal(content, resp)
	notify(resp)

}

func notify(r *githubResponse) {
	fmt.Println("In notify")
	filledMessage := fmt.Sprintf(
		message,
		r.HeadCommit.Timestamp,
		r.HeadCommit.Committer.Name,
		r.Repository.Name,
		r.HeadCommit.Message,
		r.Repository.URL,
		r.HeadCommit.URL,
		"OK",
	)

	data := fmt.Sprintf(`payload={"channel": "#godev", "text": "%v"}`, filledMessage)
	fmt.Println("Data: ", data)
	req, _ := http.NewRequest("POST", notificationURL, bytes.NewBufferString(data))
	req.Header.Add("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Add("Content-Length", strconv.Itoa(len(data)))
	_, err := http.DefaultClient.Do(req)

	if err != nil {
		log.Println(err)
	}
}

func main() {
	http.HandleFunc(`/hooks/redeploy`, redeploy)
	http.ListenAndServe(":9000", nil)
}

// func notify() {
// 	data := fmt.Sprintf(`payload={"channel": "#godev", "text": "hey guys"}`)
// 	req, _ := http.NewRequest("POST", notificationURL, bytes.NewBufferString(data))
// 	req.Header.Add("Content-Type", "application/x-www-form-urlencoded")
// 	req.Header.Add("Content-Length", strconv.Itoa(len(data)))
// 	_, err := http.DefaultClient.Do(req)
// 	if err != nil {
// 		log.Println(err)
// 	}
// }
//
// func main() {
// 	notify()
// }
