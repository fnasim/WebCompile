package main

/*
    WebCompile v0.1
	//github.com/fnasim/webcompile

	Design: On startup, create N (ConcurrentConatiners) docker containers. At each run, container is
	discarded and recreated. So any given time we should have N containers available
	in the system in idle state. This makes execution blazing fast.

	More info: https://github.com/fnasim/WebCompile
*/

import (
	"io"
	"net/http"
	"log"
	"os/exec"
	"os"
	"time"
	"encoding/json"
	"fmt"
	"strings"
	"io/ioutil"
	"github.com/fsouza/go-dockerclient"
)

// globals
var DockerClient *docker.Client

type CompileResponse struct {
	Output string
	LatencyInMS int64
	ErrorString string
	Timeout bool
}

type ContainerInfo struct {
	ID string
	Path string
}

var AvailableContainers chan ContainerInfo

func start(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-type", "text/html")
	io.WriteString(w, 
		"<form method=POST action=compile>" +
		"<textarea name=code rows=10 cols=50>public class X { public static void Main() { int sum = 0; for(int i = 0; i < 1000; i++) sum += i; System.Console.WriteLine(\"Sum: \" + sum); } }</textarea><br>" +
		"<input type=submit></form>")
}

func compile(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-type", "application/json")
	
	if(len(CorsHeader) > 0) {
		w.Header().Set("Access-Control-Allow-Origin", CorsHeader)
	}
	// language := r.PostFormValue("language")
	language := "C#"
	code := r.PostFormValue("code")

	codeSize := len(code)
	if(codeSize <= 0 || codeSize > MaximumCodeSizeInBytes) {
		compileResponseError(w, "Code must be greater than 0 and less than max limit")
		return
	}

	out, err, elapsed, timedOut := executeCodeOnDocker(language, code)

	compileResponse(
		w,
		out,
		int64(elapsed / time.Millisecond),
		err,
		timedOut)
}

func stats(w http.ResponseWriter, r *http.Request) {
	// TODO:
	// server start date time
	// graph of requests, languages, browser, country
	// graph of docker latency, execution latency
	// number of active containers
	// # of errors and strings creating/deleting/executing containers
	io.WriteString(w, "Not implemented yet")
}


func executeCodeOnDocker(language, code string) (string, string, time.Duration, bool) {
    // get a prepped container
	log.Print("Acquiring container")

	container := <- AvailableContainers

	defer func() {
		go createNewContainer()
		go destroyContainer(container)
	}()

	codePath := container.Path + "/code.cs"

	fo, err := os.Create(codePath)
	if err != nil {
		return "", "Cannot create code file", time.Second, false
	}
	fo.WriteString(code)
	fo.Close()
	
	log.Print("Executing container from path: " + container.Path)
	
	startTime := time.Now()

	args := []string { "exec", container.ID, "/bin/bash", "-c", CommandMap[language]}
	cmd := exec.Command("docker", args...)

	stdin, _ := cmd.StdinPipe()
	stdout, _ := cmd.StdoutPipe()
	stderr, _ := cmd.StderrPipe()
	err = cmd.Start()

	if err != nil {
		log.Print("Command start: ", err)
	}

	timedOut := false

	timer := time.AfterFunc(ExecutionTimeout, func() {
		timedOut = true

		stdin.Close()
		stdout.Close()
		stderr.Close()

		err := cmd.Process.Kill()
		if err != nil {
			log.Print("Cannot kill process: ", err)
		}
	})

	out, _ := ioutil.ReadAll(stdout)
	errout, _ := ioutil.ReadAll(stderr)

	cmd.Wait()
	timer.Stop()

	elapsedTime := time.Since(startTime)

	return string(out), string(errout), elapsedTime, timedOut
}

func destroyContainer(container ContainerInfo) {	
	log.Print("Destroy " + container.ID)
	err := DockerClient.KillContainer(docker.KillContainerOptions { ID: container.ID })
	if err != nil {
		log.Print("KillContainer: ", err)
	}
	err = DockerClient.RemoveContainer(docker.RemoveContainerOptions { ID: container.ID })
	if err != nil {
		log.Print("RemoveContainer: ", err)
	}
	os.RemoveAll(container.Path)
}

func createNewContainer() {
	tmpPath, err := createTempPath()

	if err != nil {
		log.Print("Cannot create temp path (", tmpPath, "): ", err)
		return
	}

	cmd := "docker"
	args := []string { "run", "-d", "-v", tmpPath + ":/home/code/source", "-t", DockerImage,"/bin/bash"}
	
	startTime := time.Now()
	out, err := exec.Command(cmd, args...).Output()
	elapsedTime := time.Since(startTime)

	if err != nil {
		log.Print("CreateContainer: ", out, err)
		return
	}
	containerId := strings.TrimSpace(string(out))

	log.Print("New container: " + containerId + " in " + elapsedTime.String())

	AvailableContainers <- ContainerInfo {ID: containerId, Path: tmpPath}
}

func compileResponse(w http.ResponseWriter, output string, latency int64, error string, timeout bool) {
	res := &CompileResponse {
        Output: output,
        LatencyInMS: latency,
    	ErrorString: error,
    	Timeout: timeout}
    resJson, _ := json.Marshal(res)

    response := string(resJson)
    io.WriteString(w, response)
}

func compileResponseError(w http.ResponseWriter, error string) {
	res := &CompileResponse {
        Output: "",
        LatencyInMS: 0,
    	ErrorString: error,
    	Timeout: false,
    }
    resJson, _ := json.Marshal(res)

    response := string(resJson)
    io.WriteString(w, response)
}

func InitializeDocker() {
	client, err := docker.NewClientFromEnv()
	if err != nil {
		log.Fatal("Cannot create docker client: ", err)
	}

	DockerClient = client

	imgs, err := client.ListImages(docker.ListImagesOptions{ All: false })
	if err != nil {
		log.Fatal("Error on listing docker images", err)
	}

	// list images
	for _, img := range imgs {
		log.Print(fmt.Sprintf("[ID %s] [Name %s]", img.ID, img.RepoTags))
	}

	// list running containers
	containers, err :=  client.ListContainers(docker.ListContainersOptions {All: false})
	for _, container := range containers {
		log.Print(fmt.Sprintf("[ID %s] [Image %s] [Command %s]", container.ID, container.Image, container.Command))
	}

	for i := 0; i < ConcurrentContainers; i++ {
		go createNewContainer()
	}
}

func StartWebServer() {
	log.Print("Path for new folders: " + PathForCodeStorage)
	log.Print("Listening on " + Port)

	mux := http.NewServeMux();
	mux.HandleFunc("/", start)
	mux.HandleFunc("/compile", compile)

	if(EnableStatsEndpoint) {
		mux.HandleFunc("/stats", stats)
	}

	err := http.ListenAndServe(":" + Port, mux)
	
	if err != nil {
		log.Fatal("ListenAndServe failed: ", err)
	}
}

func Shutdown() {
	log.Print("BYE!")
	close(AvailableContainers)

	// destroy all active containers
	for container := range AvailableContainers {
		destroyContainer(container)
	}
}

func main() {
	log.Print("Starting up")
	defer Shutdown()

	HandleOSInterrupt(Shutdown)
	InitializeConfig()
	InitializeDocker()
	StartWebServer()
}