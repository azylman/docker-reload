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

	"github.com/azylman/docker-reload/recursivenotify"
)

func panicIfErr(err error) {
	if err != nil {
		panic(err)
	}
}

type Backend struct {
	container string

	m    sync.Mutex
	proc *exec.Cmd

	http.Handler
}

func (b *Backend) StartBackend() bool {
	b.m.Lock()
	defer b.m.Unlock()

	build := exec.Command("docker", "build", ".")
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	build.Stdout = io.MultiWriter(os.Stdout, &stdout)
	build.Stderr = io.MultiWriter(os.Stderr, &stderr)
	if err := build.Run(); err != nil {
		// Don't treat this as an error, just leave the old backend running
		log.Printf("error building docker image:\n%s\n%s", stdout.String(), stderr.String())
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

	run := exec.Command("docker", "run", "-p", fmt.Sprintf("%s:%s", port, b.container), image)
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
	watcher, err := recursive.NewWatcher()
	panicIfErr(err)
	defer watcher.Close()
	watcher.Add(".")

	for {
		select {
		case <-watcher.Events:
			if ok := b.StartBackend(); ok {
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
	flag.Parse()

	pieces := strings.Split(*ports, ":")

	backend := &Backend{
		container: pieces[1],
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
