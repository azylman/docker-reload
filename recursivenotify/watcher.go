package recursive

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"gopkg.in/fsnotify.v1"
)

type Watcher struct {
	Events chan fsnotify.Event
	Errors chan error
	*fsnotify.Watcher
}

func (w *Watcher) Add(name string) error {
	name, err := filepath.Abs(name)
	if err != nil {
		return err
	}
	isDirectory, err := isDirectory(name)
	if err != nil {
		return err
	}
	w.Watcher.Add(name)
	if !isDirectory {
		return nil
	} else {
		files, err := ioutil.ReadDir(name)
		if err != nil {
			return err
		}
		for _, file := range files {
			if err := w.Add(filepath.Join(name, file.Name())); err != nil {
				return err
			}
		}
	}
	return nil
}

func NewWatcher() (*Watcher, error) {
	w, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}
	new := &Watcher{
		Events:  make(chan fsnotify.Event),
		Errors:  make(chan error),
		Watcher: w,
	}
	go proxyEvents(w.Events, new.Events)
	go proxyErrors(w.Errors, new.Errors)
	return new, nil
}

func NewDebouncedWatcher(interval time.Duration) (*Watcher, error) {
	w, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}
	events := make(chan fsnotify.Event)
	new := &Watcher{
		Events:  make(chan fsnotify.Event),
		Errors:  make(chan error),
		Watcher: w,
	}
	go proxyEvents(w.Events, events)
	go debounceEvents(events, new.Events, interval)
	go proxyErrors(w.Errors, new.Errors)
	return new, nil
}

func proxyErrors(in <-chan error, out chan<- error) {
	for ev := range in {
		out <- ev
	}
}

func proxyEvents(in <-chan fsnotify.Event, out chan<- fsnotify.Event) {
	for ev := range in {
		out <- ev
	}
}

func debounceEvents(in <-chan fsnotify.Event, out chan<- fsnotify.Event, interval time.Duration) {
	var m sync.Mutex
	var ev *fsnotify.Event
	for {
		select {
		case e := <-in:
			// Don't do anything for hidden files and files in .git
			if filepath.Base(e.Name)[0] == '.' || strings.Contains(e.Name, ".git") {
				continue
			}
			ev = &e
			continue
		case <-time.After(interval):
			m.Lock()
			if ev != nil {
				out <- *ev
				ev = nil
			}
			m.Unlock()
		}
	}
}

func isDirectory(path string) (bool, error) {
	fileInfo, err := os.Stat(path)
	return fileInfo.IsDir(), err
}
