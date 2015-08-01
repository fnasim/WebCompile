// Configuration

package main

import (
	"time"
	"log"
	"os"
)

// # of concurrent containers to start
const ConcurrentContainers = 5

// docker image to use
const DockerImage = "mono"

// maximum code execution time
const ExecutionTimeout = 5 * time.Second

// limit code size HTTP POST
const MaximumCodeSizeInBytes = 8 * 1024

// allow requests from other sites, empty to disable
const CorsHeader = "*"

// run webcompile server on this port
const Port = "8000"

// enable /stats to show statistics
const EnableStatsEndpoint = true

// command to run on the container to compile code
var CommandMap map[string]string

// local path which is mounted on the docker image for writing code
var PathForCodeStorage string // current working directoy or exact path, should be writable
var PathSuffix = "runs"

func InitializeConfig() {
	CommandMap = make(map[string]string)
	CommandMap["C#"] = "mcs -out:/home/code/code.exe /home/code/source/code.cs; startTime=$(date +%s%3N);  if [[ $? == 0 ]]; then  mono /home/code/code.exe; endTime=$(date +%s%3N); diff=$(($endTime-$startTime)); echo Code Execution: $diff ms; fi";

	AvailableContainers = make(chan ContainerInfo, ConcurrentContainers)

	pwd, err := os.Getwd()

	if err != nil {
		log.Fatal("Cannot get present directory")
		return
	}

	PathForCodeStorage = pwd + "/" + PathSuffix
}