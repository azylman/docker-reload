package main

import (
	"bytes"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/azylman/docker-reload/recursivenotify"
	"gopkg.in/fsnotify.v1"
)

func panicIfErr(err error) {
	if err != nil {
		panic(err)
	}
}

type Backend struct {
	container string
	envFile   string

	m    sync.Mutex
	proc *exec.Cmd

	http.Handler
}

func (b *Backend) StartBackend() bool {
	b.m.Lock()
	defer b.m.Unlock()

	build := exec.Command("docker", "build", ".")
	var stdout bytes.Buffer
	build.Stdout = io.MultiWriter(os.Stdout, &stdout)
	build.Stderr = os.Stderr
	if err := build.Run(); err != nil {
		return false
	}

	lines := strings.Split(stdout.String(), "\n")
	last := lines[len(lines)-2]
	pieces := strings.Split(last, " ")
	image := pieces[len(pieces)-1]

	port, err := getNewPort()
	if err != nil {
		return false
	}

	args := []string{
		"run",
		"-p",
		fmt.Sprintf("%s:%s", port, b.container),
	}
	if b.envFile != "" {
		args = append(args, "--env-file", b.envFile)
	}
	args = append(args, image)
	run := exec.Command("docker", args...)
	run.Stdout = os.Stdout
	run.Stderr = os.Stderr
	if err := run.Start(); err != nil {
		return false
	}

	url, err := url.Parse("http://localhost:" + port)
	if err != nil {
		log.Printf("failed to parse url: %s", err.Error())
		return false
	}
	if b.proc != nil {
		if err := b.proc.Process.Signal(syscall.SIGTERM); err != nil {
			log.Fatal(err)
		}
	}
	b.proc = run
	b.Handler = httputil.NewSingleHostReverseProxy(url)
	return true
}

func (b *Backend) ReloadOnChanges() {
	watcher, err := recursive.NewDebouncedWatcher(time.Second)
	panicIfErr(err)
	defer watcher.Close()
	watcher.Add(".")

	for {
		select {
		case ev := <-watcher.Events:
			event := ""
			switch ev.Op {
			case fsnotify.Create:
				event = "create"
			case fsnotify.Write:
				event = "write"
			case fsnotify.Remove:
				event = "remove"
			case fsnotify.Rename:
				event = "rename"
			case fsnotify.Chmod:
				event = "chmod"
			}
			fmt.Println("")
			log.Printf("got %s event for %s", event, ev.Name)
			if ok := b.StartBackend(); ok {
				fmt.Println("")
				log.Println("successfully reloaded")
			}
		case err := <-watcher.Errors:
			log.Println("error watching: %s", err.Error())
		}
	}
}

func (b *Backend) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	b.m.Lock()
	defer b.m.Unlock()

	b.Handler.ServeHTTP(rw, r)
}

func main() {
	ports := flag.String("p", "", "optional port bindings")
	envFile := flag.String("env-file", "", "optional env file")
	flag.Parse()

	pieces := strings.Split(*ports, ":")

	backend := &Backend{
		container: pieces[1],
		envFile:   *envFile,
	}
	backend.StartBackend()

	go backend.ReloadOnChanges()
	http.Handle("/", backend)
	panicIfErr(http.ListenAndServe(":"+pieces[0], nil))
}

func getNewPort() (string, error) {
	listener, err := net.Listen("tcp", ":0")
	if err != nil {
		return "", err
	}

	addr := listener.Addr()
	listener.Close()
	pieces := strings.Split(addr.String(), ":")
	return pieces[len(pieces)-1], nil
}
