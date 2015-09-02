package main

import (
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
)

const (
	configJSONPath = "config.json"
)

var (
	webhookBinPath string
	portParameter  string
	hooksPath      string
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

	portParameter = fmt.Sprintf("-port=%v", int64(config["port"].(float64)))
	hooksPath = config["hooks_path"].(string)
	webhookBinPath = config["hook_bin"].(string)
}

func main() {
	cmd := exec.Command(webhookBinPath, "-hooks", hooksPath, "-verbose", portParameter)
	stdout, _ := cmd.StdoutPipe()
	err := cmd.Start()
	if err != nil {
		panic(err)
	}
	io.Copy(os.Stdout, stdout)
	cmd.Wait()
}
