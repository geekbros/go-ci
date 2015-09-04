package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path"
	"strconv"
	"strings"
)

const (
	configJSONPath = "./config.json"

	buildOK   = ":white_check_mark:"
	buildFail = ":sos:"
)

var (
	cfg    config
	gopath = os.Getenv("GOPATH")
)

type (
	// Config structs.
	repo struct {
		Path    string   `json:"path"`
		Scripts []string `json:"scripts"`
	}

	config struct {
		Port            int    `json:"port"`
		HooksPath       string `json:"hooks_path"`
		NotificationURL string `json:"notification_url"`
		Channel         string `json:"channel"`
		Repos           []repo `json:"gopath_local_repos"`
	}

	// GitHub webhook json response struct.
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

	// Specific Slack request structs.
	field struct {
		Title string `json:"title"`
		Value string `json:"value"`
		Short bool   `json:"short"`
	}

	attachment struct {
		Fallback  string  `json:"fallback"`
		Color     string  `json:"color"`
		Title     string  `json:"title"`
		TitleLink string  `json:"title_link"`
		Text      string  `json:"text"`
		Fields    []field `json:"fields"`
	}

	slackMessage struct {
		Channel     string       `json:"string"`
		Attachments []attachment `json:"attachments"`
	}
)

func getSlackMessage(success bool, log *string, r *githubResponse) *slackMessage {
	var (
		fallback string
		color    string
		text     string
	)
	if success {
		fallback = "Build succeeded"
		color = "good"
	} else {
		fallback = "Build failed"
		color = "danger"
		text = *log
	}

	return &slackMessage{
		Channel: cfg.Channel,
		Attachments: []attachment{
			attachment{
				Fallback:  fallback,
				Color:     color,
				Text:      text,
				Title:     r.HeadCommit.Committer.Name + " pushed to " + r.Repository.Name,
				TitleLink: r.HeadCommit.URL,
				Fields: []field{
					field{"Message", r.HeadCommit.Message, true},
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
	if err != nil {
		panic("Can't read config file.")
	}
	err = json.Unmarshal(configContent, &cfg)
	if err != nil {
		panic("Can't parse config file as json: " + err.Error())
	}
}

func getRepo(repoName string) (r repo) {
	for _, v := range cfg.Repos {
		if strings.Contains(v.Path, repoName) {
			r = v
		}
	}
	return
}

func redeploy(w http.ResponseWriter, r *http.Request) {
	// Parse github hook response.
	defer r.Body.Close()
	content, _ := ioutil.ReadAll(r.Body)
	resp := &githubResponse{}
	json.Unmarshal(content, resp)

	currentDir, _ := os.Getwd()

	var (
		cmd      *exec.Cmd
		tempText string
		fullLog  string
		success  = true
	)

	// Find out which repo was changed.
	// Notify about failure if changed repo is not listed in config.
	repo := getRepo(resp.Repository.Name)
	if repo.Path == "" {
		fullLog = "Repo is not listed in config"
		notify(getSlackMessage(false, &fullLog, resp))
		return
	}

	// Go to repo's directory.
	// Notify about failure if changed repo can't be found locally.
	repoDir := path.Join(gopath, repo.Path)
	log.Println("Repo dir: ", repoDir)
	err := os.Chdir(repoDir)
	if err != nil {
		fullLog = "Can't change current dir to repo's dir."
		notify(getSlackMessage(false, &fullLog, resp))
		return
	}
	defer os.Chdir(currentDir)
	// Execute all repo's scripts.
	for _, s := range repo.Scripts {
		cmd = exec.Command("./" + s)
		stdout, _ := cmd.StdoutPipe()
		err = cmd.Start()

		// Can't execute script - notify about fail and stop.
		if err != nil {
			fullLog = "Can't execute script " + s
			notify(getSlackMessage(false, &fullLog, resp))
			return
		}
		buf := bufio.NewScanner(stdout)
		for buf.Scan() {
			tempText = buf.Text()
			fullLog += tempText
		}
		err = cmd.Wait()
		// Script executed with error - notify about fail and stop.
		if err != nil {
			log.Println("Failed while executing " + s)
			log.Println("Error message: ", err.Error())
			success = false
			notify(getSlackMessage(success, &fullLog, resp))
			return
		}
		// Everything is OK - notify about success and continue executing other scripts.
		notify(getSlackMessage(success, &fullLog, resp))
	}
}

func notify(s *slackMessage) {
	filledMessage, _ := json.Marshal(s)
	data := fmt.Sprintf(`payload=%v`, string(filledMessage))
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
