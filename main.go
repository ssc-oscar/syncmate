package main

import "github.com/hrz6976/syncmate/cmd"

var VERSION string = "<unknown>"
var BUILD_TIME string = "<unknown>"
var COMMIT_HASH string = "<unknown>"

func main() {
	cmd.Version = VERSION
	cmd.BuildTime = BUILD_TIME
	cmd.CommitHash = COMMIT_HASH
	cmd.Execute()
}
