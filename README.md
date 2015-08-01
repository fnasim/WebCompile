# WebCompile 0.1

This is my first program written in Go which compiles C# code (as a proof of concept) on a docker 'mono' image in a docker sandboxed container. The http server exposes "/"" which is a simple text area to demonstrate the capability. /compile is where the magic happens. I was curious how to build such a system and found [CompileBox](https://github.com/remoteinterview/compilebox) which is pretty cool! Decided to write one myself with a few borrowed ideas.

> This is not production ready and a bunch of features should be implemented before using it in production (see later).

## Design

### Startup
* create N docker containers to reduce latency of container creation time in compilation step
* each container has /home/temp which is mounted to ./runs/TEMP where the user posted code is saved
* http server listens to default port 8000 for compile requests

### Compilation
* get handle of an available container and defer concurrent requests to destroy this container and create new, the requester waits until a container is available
* source code POSTed is copied to a temporary folder in ./runs/ (already mounted on the container)
* code compilation and execution are executed on the container
* stdout and stderr are captured and returned to client as JSON response (Output and ErrorString fields)
* timeout can be set and in case of a timeout Timeout: true is returned as part of JSON response

### JSON response

```javascript
{"Output":"Sum: 499500\nCode Execution: 23 ms\n",
"LatencyInMS":582,
"ErrorString":"",
"Timeout":false}
```

## Setup

* Install Go 1.4+
* Install docker (Mac is OK, use boot2docker)
* Install docker image 'mono:latest'
* Create new a directory 'runs' with write access
* Edit config.go and set configuration options
* go build -o webcompile *.go (will need to "go get" a few packages)
* ./webcompile

## Test

http://localhost:8000/

## TODO

* use stdin to send code to container. Avoids temporary path, mounting, copying and delete
* use web security libraries to enable throttling
* dump logs to disk
* use go docker client library for 'create' and 'exec'
* better error handling
* support more languages. v0.1 only supports C# as a proof of concept
* optimize container properties (limit memory?)
* fault tolerance to recover after errors creating docker containers
* add tests, do scale testing
* use [bocker](https://github.com/p8952/bocker) instead of docker? :)
* time out if cannot acquire container
* if client cancels request, cancel wait/execution gracefully
* collect stats and optionally expose them on /stats
