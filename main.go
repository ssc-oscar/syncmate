package main

import (
	"fmt"

	"github.com/hrz6976/syncmate/cmd"
	"github.com/sirupsen/logrus"
)

var VERSION string = "<unknown>"
var BUILD_TIME string = "<unknown>"
var COMMIT_HASH string = "<unknown>"

func init() {
	// Set the log level to Debug
	logrus.SetLevel(logrus.DebugLevel)
}

func main() {
	cmd.RootCmd.Version = fmt.Sprintf("%s\r\nbuilt: %s\r\ncommit: %s", VERSION, BUILD_TIME, COMMIT_HASH)
	cmd.Execute()
}
