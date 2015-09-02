# go-ci
Golang mini CI server for automatic building go code from git repo, and send messages to Slack #channel.

## What does it do:
* Listen for Git's pushes webhooks
* Make git sync, to get latest commits
* Run build command, to build and run executable
* Send to Slack build Success or Fail status
