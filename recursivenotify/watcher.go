package recursive

import (
	"io/ioutil"
	"os"
	"path/filepath"

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
	if !isDirectory {
		return w.Watcher.Add(name)
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
	go func() {
		for {
			select {
			case ev := <-w.Events:
				new.Events <- ev
			case err := <-w.Errors:
				new.Errors <- err
			}
		}
	}()
	return new, nil
}

func isDirectory(path string) (bool, error) {
	fileInfo, err := os.Stat(path)
	return fileInfo.IsDir(), err
}
