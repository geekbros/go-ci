package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
)

const (
	configJSONPath = "./config.json"

	buildOK   = ":white_check_mark:"
	buildFail = ":sos:"
)

var (
	cfg                 config
	gopath              = os.Getenv("GOPATH")
	workingDirectory, _ = os.Getwd()

	lock = sync.Mutex{}
)

// JSON handling structs
type (
	script struct {
		Script string `json:"script"`
		Wait   bool   `json:"wait"`
	}

	// Config structs.
	repo struct {
		Path    string   `json:"path"`
		Scripts []script `json:"scripts"`
	}

	config struct {
		Port            int    `json:"port"`
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
		Text        string       `json:"text"`
		Channel     string       `json:"string"`
		Attachments []attachment `json:"attachments"`
	}
)

type repoWorker struct {
	fullLog     *string
	resp        *githubResponse
	attachments []attachment
	repository  repo
}

func newRepoWorker(w http.ResponseWriter, r *http.Request) (worker repoWorker) {
	lock.Lock()
	defer lock.Unlock()
	defer r.Body.Close()
	// Parse github hook response.
	content, _ := ioutil.ReadAll(r.Body)
	resp := &githubResponse{}
	json.Unmarshal(content, resp)

	var (
		attachments []attachment
		fullLog     string
	)

	// Find out which repo was changed.
	// Notify about failure if changed repo is not listed in config.
	repo := getRepo(resp.Repository.Name)
	worker = repoWorker{fullLog: &fullLog, resp: resp, attachments: attachments, repository: repo}

	if repo.Path == "" {
		fullLog = "Repo is not listed in config"
		notify(getSlackMessage(false, &fullLog, "Build failed", resp, attachments))
		return
	}

	// Go to repo's directory.
	// Notify about failure if changed repo can't be found locally.
	repoDir := filepath.Join(gopath, repo.Path)
	err := os.Chdir(repoDir)

	if err != nil {
		fullLog = "Can't change current dir to repo's dir."
		notify(getSlackMessage(false, &fullLog, "Build failed", resp, attachments))
		return
	}
	return
}

// init initializes cfg with config.json content
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

// Redeploy is a http handler chain function, which
// is given info about push request to watched repository.
// It syncs it to it's remote's current state and executes all
// of it's scripts, listed in corresponding config sections.
func Redeploy(w http.ResponseWriter, r *http.Request) {
	worker := newRepoWorker(w, r)

	notify(&slackMessage{
		Text:    "Redeploying initiated...",
		Channel: cfg.Channel,
	})

	reload(worker)

	attachments, scriptsLog, success := executeScripts(worker.repository, worker.resp)
	*worker.fullLog += scriptsLog

	if len(attachments) > 0 {
		notify(getSlackMessage(success, worker.fullLog, "Script succeeded", worker.resp, attachments))
	}

	notify(&slackMessage{
		Text:    "Done redeploying.",
		Channel: cfg.Channel,
	})
}

// Restart is a http handler function.
// It syncs all repos from config to their remote's current state and executes all
// of their scripts, listed in corresponding config sections.
func Restart(w http.ResponseWriter, r *http.Request) {
	worker := newRepoWorker(w, r)

	notify(&slackMessage{
		Text:    "Restarting initiated...",
		Channel: cfg.Channel,
	})

	reload(worker)

	attachments, scriptsLog, success := executeScripts(worker.repository, worker.resp)
	*worker.fullLog += scriptsLog

	if len(attachments) > 0 {
		notify(getSlackMessage(success, worker.fullLog, "Restart succeeded", worker.resp, attachments))
	}

	notify(&slackMessage{
		Text:    "Done restarting.",
		Channel: cfg.Channel,
	})
}

func reload(worker repoWorker) {
	// Sync repo.
	// Notify if error occured.
	pull := exec.Command("git", "pull", "origin", "master")
	pull.Stdout = os.Stdout
	pull.Start()
	err := pull.Wait()
	if err != nil {
		log.Println("Syncing error: ", err.Error())
		*worker.fullLog += "Can't sync repo " + worker.resp.Repository.Name
		notify(getSlackMessage(false, worker.fullLog, "Build failed", worker.resp, worker.attachments))
		return
	}
}

func executeScripts(r repo, resp *githubResponse) (attachments []attachment, fullLog string, success bool) {
	var cmd *exec.Cmd

	// Execute all repo's scripts.
	for _, s := range r.Scripts {
		log.Println("Executing script ", s.Script, "...")
		commandTokens := strings.Split(s.Script, " ")
		if len(commandTokens) == 1 {
			cmd = exec.Command("./" + commandTokens[0])
		} else {
			// Case when concrete command given instead of script.
			cmd = exec.Command(commandTokens[0], commandTokens[1:]...)
		}

		stdout, _ := cmd.StdoutPipe()
		stderr, _ := cmd.StderrPipe()

		err := cmd.Start()

		// Can't execute script - notify about fail and stop.
		if err != nil {
			log.Println("Script execution error: ", err)
			fullLog += "Can't execute script " + s.Script + ". " + err.Error()
			attachments = append(attachments, getSlackAttachment(false, &fullLog, s.Script, resp))
			return
		}
		// Reading all cmd's output while it can.
		go func() {
			content, _ := ioutil.ReadAll(stdout)
			errContent, _ := ioutil.ReadAll(stderr)
			fullLog = string(content) + "\n" + string(errContent)
			log.Println("Log of "+s.Script+": ", fullLog)
		}()
		// Case when script is marked as "wait":"true" in config.
		if s.Wait {
			err = cmd.Wait()
		}
		// Script executed with error - notify about fail and stop.
		if err != nil {
			log.Println("Failed while executing " + s.Script)
			log.Println("Error message: ", err.Error())
			attachments = append(attachments, getSlackAttachment(false, &fullLog, s.Script, resp))
			return
		}
		// Everything is OK - notify about success and continue executing other scripts.
		log.Println("Done executing script ", s, " .")
		attachments = append(attachments, getSlackAttachment(true, &fullLog, s.Script, resp))
		success = true
	}
	return
}

func getRepo(repoName string) (r repo) {
	for _, v := range cfg.Repos {
		if strings.Contains(v.Path, repoName) {
			return v
		}
	}
	return
}

func getSlackAttachment(success bool, log *string, title string, r *githubResponse) attachment {
	var (
		fallback string
		color    string
		text     string
	)
	if success {
		fallback = "Script succeeded"
		color = "good"
	} else {
		fallback = "Script failed"
		color = "danger"
		text = *log
	}
	return attachment{
		Fallback: fallback,
		Color:    color,
		Text:     text,
		Title:    title,
	}
}

func getSlackMessage(success bool, log *string, title string, r *githubResponse, attachments []attachment) *slackMessage {
	var successMessage string
	if success {
		successMessage = "SUCCESS:tada:"
	} else {
		successMessage = "FAIL:interrobang:"
	}
	return &slackMessage{
		Text: fmt.Sprintf("\n_%v_\nAfter *%v* pushed to *%v*.\n*Latest commit message*: <%v|%v>\n*Log*: \n%v",
			successMessage, r.HeadCommit.Committer.Name, r.Repository.Name, r.HeadCommit.URL, r.HeadCommit.Message, *log),
		Channel:     cfg.Channel,
		Attachments: attachments,
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
	http.HandleFunc(`/hooks/redeploy`, Redeploy)
	http.HandleFunc(`/hooks/restart`, Restart)
	http.ListenAndServe(fmt.Sprintf(":%v", cfg.Port), nil)
}
