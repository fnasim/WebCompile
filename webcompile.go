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
	"os/signal"
	"github.com/kjk/betterguid"
	"time"
	"encoding/json"
	"fmt"
	"strings"
	"io/ioutil"
	"github.com/fsouza/go-dockerclient"
)

// Configuration

// # of concurrent containers to start
const ConcurrentContainers = 5

// docker image to use
const DockerImage = "mono"

// maximum code execution time
const ExecutionTimeout = 5 * time.Second

// limit code size
const MaximumCodeSizeInBytes = 8 * 1024;

// allow requests from other sites, empty to disable
const CorsHeader = "*";

// run the compile server on this port
const Port = "8000";

// command to run on the container to compile code
const ShellCommand = "mcs -out:/home/code/code.exe /home/code/source/code.cs; startTime=$(date +%s%3N);  if [[ $? == 0 ]]; then  mono /home/code/code.exe; endTime=$(date +%s%3N); diff=$(($endTime-$startTime)); echo Code Execution: $diff ms; fi";

// local path which is mounted on the docker image for writing code
var PathForCodeStorage string // current working directoy or exact path, should be writable
var PathSuffix = "runs";

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
	code := r.PostFormValue("code")

	codeSize := len(code)
	if(codeSize <= 0 || codeSize > MaximumCodeSizeInBytes) {
		compileResponseError(w, "Code is malformed. May be max limit")
		return
	}

	out, err, elapsed, timedOut := executeCodeOnDocker(code)

	compileResponse(
		w,
		out,
		int64(elapsed / time.Millisecond),
		err,
		timedOut)
}

func executeCodeOnDocker(code string) (string, string, time.Duration, bool) {
    // get a prepped container containers
	log.Print("Acquiring container")

	containerInfo := <- AvailableContainers
	containerId, runPath := containerInfo.ID, containerInfo.Path

	defer func() {
		go createNewContainer()
		go destroyContainer(containerId, runPath)
	}()

	codePath := runPath + "/code.cs"

	fo, err := os.Create(codePath)
	if err != nil {
		return "", "Cannot create code file", time.Second, false
	}
	fo.WriteString(code)
	fo.Close()
	
	log.Print("Running container from path: " + runPath)
	
	startTime := time.Now()

	args := []string { "exec", containerId, "/bin/bash", "-c", ShellCommand}
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

func destroyContainer(containerId string, path string) {
	log.Print("Destroy " + containerId)
	err := DockerClient.KillContainer(docker.KillContainerOptions { ID: containerId })
	if err != nil {
		log.Print("KillContainer: ", err)
	}
	err = DockerClient.RemoveContainer(docker.RemoveContainerOptions { ID: containerId })
	if err != nil {
		log.Print("RemoveContainer: ", err)
	}
	os.RemoveAll(path)
}

func createNewContainer() {
	tmpPath := createTempPath()
	cmd := "docker"
	args := []string { "run", "-d", "-v", tmpPath + ":/home/code/source", "-t", DockerImage,"/bin/bash"}
	
	startTime := time.Now()
	out, err := exec.Command(cmd, args...).Output()
	elapsedTime := time.Since(startTime)

	if err != nil {
		log.Print("CreateContainer: ", err)
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

func main() {

	log.Print("Starting up")

	AvailableContainers = make(chan ContainerInfo, ConcurrentContainers)

	pwd, err := os.Getwd()

	if err != nil {
		log.Fatal("Cannot get present directory")
		return
	}

	PathForCodeStorage = pwd + "/" + PathSuffix

	client, err := docker.NewClientFromEnv()
	if err != nil {
		log.Fatal("Cannot create docker client: ", err)
	}

	DockerClient = client

	imgs, err := client.ListImages(docker.ListImagesOptions{ All: false })
	if err != nil {
		log.Fatal(err)
	}
	for _, img := range imgs {
		log.Print(fmt.Sprintf("[ID %s] [Name %s]", img.ID, img.RepoTags))
	}

	// list of running containers
	containers, err :=  client.ListContainers(docker.ListContainersOptions {All: false})
	for _, container := range containers {
		log.Print(fmt.Sprintf("[ID %s] [Image %s] [Command %s]", container.ID, container.Image, container.Command))
	}

	for i := 0; i < ConcurrentContainers; i++ {
		go createNewContainer()
	}
	defer destroyContainers()

	log.Print("Path for new folders: " + PathForCodeStorage)
	log.Print("Listening on " + Port)

	sig := make(chan os.Signal, 1)
  	signal.Notify(sig, os.Interrupt)
  	go func() {
    	<-sig
    	log.Print("BYE")
    	destroyContainers()
    	os.Exit(0)
  	}()

	mux := http.NewServeMux();
	mux.HandleFunc("/", start)
	mux.HandleFunc("/compile", compile)

	err = http.ListenAndServe(":" + Port, mux)
	
	if err != nil {
		log.Fatal("ListenAndServe failed: ", err)
	}
}

func destroyContainers() {
	close(AvailableContainers)

	// destroy all active containers
	for containerInfo := range AvailableContainers {
		destroyContainer(containerInfo.ID, containerInfo.Path)
	}
}

func createTempPath() string {
	uuid := betterguid.New()
	date := time.Now().Format("2006-01-15")
	runPath := PathForCodeStorage + "/" + date + uuid

	err := os.Mkdir(runPath, 0777)
	if err != nil {
		log.Print("Could not create temporary path: ", runPath)
		log.Print(err)
		return ""
	}

	return runPath
}

func cp(dst, src string) error {
	s, err := os.Open(src)
	if err != nil {
		return err
	}
	// no need to check errors on read only file, we already got everything
	// we need from the filesystem, so nothing can go wrong now.
	defer s.Close()
	d, err := os.Create(dst)
	if err != nil {
		return err
	}
	if _, err := io.Copy(d, s); err != nil {
		d.Close()
		return err
	}
	return d.Close()
}
