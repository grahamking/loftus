package main

import (
    "fmt"
    "os"
    "exp/inotify"
    "log"
    "strings"
    "path/filepath"
)

const (
    INTERESTING = inotify.IN_MODIFY | inotify.IN_CREATE | inotify.IN_DELETE
)

type ChangeAgent interface {
    Created(filename string)
    Modified(filename string)
    Deleted(filename string)
    ShouldWatch(filename string) bool
}

func main() {

    if len(os.Args) != 2 {
        fmt.Println("USAGE: bup <dir_to_sync|--server>")
        os.Exit(1)
    }

    if os.Args[1] == "--server" {
        start_server("127.0.0.1:8007")
    } else {
        start_client()
    }
}

// Watch directories, called sync methods on backend, etc
func start_client() {

    dir := strings.TrimRight(os.Args[1], "/")
    watcher, _ := inotify.NewWatcher()

    backend := &GitBackend{RootDir: dir}

    addWatches(watcher, dir, backend)
    fmt.Println("Watching:", dir)


    for {
        select {
        case ev := <-watcher.Event:

            if ev.Mask & inotify.IN_MODIFY != 0 {
                backend.Modified(ev.Name)

            } else if ev.Mask & inotify.IN_CREATE != 0 {
                if ev.Mask & inotify.IN_ISDIR != 0 && backend.ShouldWatch(ev.Name) {
                    fmt.Println("Added watch", ev.Name)
                    watcher.AddWatch(ev.Name, INTERESTING)
                }
                backend.Created(ev.Name)

            } else if ev.Mask & inotify.IN_DELETE != 0 {
                backend.Deleted(ev.Name)
            }

        case err := <-watcher.Error:
            fmt.Println("error:", err)
        }
    }
}

// Add inotify watches on dir and all sub-dirs
func addWatches(
    watcher *inotify.Watcher,
    rootDir string,
    backend ChangeAgent) {

    addSingleWatch := func(path string, info os.FileInfo, err error) error {
        if info.IsDir() && backend.ShouldWatch(path) {
            fmt.Println("Watching", path)
            watcher.AddWatch(path, INTERESTING)
        }
        return nil
    }

    err := filepath.Walk(rootDir, addSingleWatch)
    if err != nil {
        log.Fatal(err)
    }
}
