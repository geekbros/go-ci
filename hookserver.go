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
		Channel         string `json:"channel"`
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

	attachment struct {
		Fallback  string            `json:"fallback"`
		Color     string            `json:"color"`
		Title     string            `json:"title"`
		TitleLink string            `json:"title_link"`
		Text      string            `json:"text"`
		Fields    map[string]string `json:"fields"`
	}

	slackMessage struct {
		Channel     string       `json:"string"`
		Attachments []attachment `json:"attachments"`
	}
)

func getSlackMessageGood(r *githubResponse) slackMessage {
	return slackMessage{
		Channel: cfg.Channel,
		Attachments: []attachment{
			attachment{
				Fallback:  "Build succeeded",
				Color:     "good",
				Title:     "Push to " + r.Repository.Name,
				TitleLink: r.HeadCommit.URL,
				Fields: map[string]string{
					"Author":  r.HeadCommit.Committer.Name,
					"Message": r.HeadCommit.Message,
				},
			},
		},
	}
}

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
	filledMessage, _ := json.Marshal(getSlackMessageGood(r))
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
