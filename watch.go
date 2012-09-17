// inotify wrapper
package main

import (
	"loftus/inotify"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	INTERESTING = inotify.IN_MODIFY | inotify.IN_CREATE | inotify.IN_DELETE | inotify.IN_MOVE
)

type Watcher struct {
	watcher *inotify.Watcher
	changed chan string
}

// Start an inotify watch on all directories starting at 'root',
// sending filenames changed on reuturned channel.
func Watch(root string) (chan string, error) {

	inotifyWatcher, ierr := inotify.NewWatcher()
	if ierr != nil {
		return nil, ierr
	}

	w := &Watcher{inotifyWatcher, make(chan string)}

	err := w.watchDirs(root)
	if err != nil {
		return nil, err
	}

	go w.run()

	return w.changed, nil
}

// Watch all the directories starting from 'root'
func (self *Watcher) watchDirs(root string) error {

	addSingleWatch := func(path string, info os.FileInfo, err error) error {

		// Abort on any error
		if err != nil {
			return err
		}

		// Only process directories
		if info.IsDir() {
			// skip .git directories
			if isGit(path) {
				return filepath.SkipDir
			}
			log.Println("Watching", path)
			return self.watcher.AddWatch(path, INTERESTING)
		}

		return nil
	}

	return filepath.Walk(root, addSingleWatch)
}

// Listen if inotify events, group them, and send on self.changed channel.
// Run this in go-routine
func (self *Watcher) run() {

	affected := make(map[string]*inotify.Event)

	for {

		select {
		case ev := <-self.watcher.Event:
			log.Println(ev)

			// Coalesce events (inotify can fire many identical events)
			affected[ev.Name] = ev

		case <-time.After(100 * time.Millisecond):

			// Dispatch all captured events

			for name, event := range affected {

				log.Println("Dispatching", name)
				isCreate := event.Mask&inotify.IN_CREATE != 0
				isDir := event.Mask&inotify.IN_ISDIR != 0

				if isCreate && isDir && !isGit(name) {
					log.Println("Adding watch", name)
					self.watcher.AddWatch(name, INTERESTING)
				}

				self.changed <- name
			}

			affected = make(map[string]*inotify.Event)

		case err := <-self.watcher.Error:
			log.Println("error:", err)
		}
	}

}

func isGit(path string) bool {
	return strings.Contains(path, ".git")
}
