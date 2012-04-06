package main

import (
    "fmt"
    "os"
    "log"
    "strings"
    "net"
    "bufio"
    "time"
    "exp/inotify"
    "path/filepath"
)

const (
    INTERESTING = inotify.IN_MODIFY | inotify.IN_CREATE | inotify.IN_DELETE
    ADDR = "127.0.0.1:8007"
)

type ChangeAgent interface {
    Created(filename string)
    Modified(filename string)
    Deleted(filename string)
    ShouldWatch(filename string) bool
    Fetch()
    RegisterPushHook(func())
}

func main() {

    if len(os.Args) != 2 {
        fmt.Println("USAGE: bup <dir_to_sync|--server>")
        os.Exit(1)
    }

    if os.Args[1] == "--server" {
        startServer(ADDR)
    } else {
        startClient()
    }
}

// Watch directories, called sync methods on backend, etc
func startClient() {

    rootDir := strings.TrimRight(os.Args[1], "/")
    backend := &GitBackend{RootDir: rootDir}
    watcher, _ := inotify.NewWatcher()

    client := Client{rootDir: rootDir, backend: backend, watcher: watcher}
    client.addWatches()
    log.Println("Watching:", rootDir)

    // Always start with a fetch to bring us up to date
    backend.Fetch()

    go client.listenRemote()
    client.run()
}

type Client struct {
    backend ChangeAgent
    rootDir string
    watcher *inotify.Watcher
    remote net.Conn
}

// Connect to server and listen for messages, which mean we have to fetch
// new data from remote (the backend does that for us)
func (self *Client) listenRemote() {

    for {
        self.remote = getRemoteConnection()
        defer self.remote.Close()
        log.Println("Connected to remote")

        bufRead := bufio.NewReader(self.remote)
        for {
            content, err := bufRead.ReadString('\n')
            if err != nil {
                log.Println("Remote read error - re-connecting")
                self.remote.Close()
                break
            }
            log.Println("Remote sent: " + content)

            self.backend.Fetch()
        }
    }
}

// Get a connection to remote server which tells us when to pull
func getRemoteConnection() net.Conn {

    var conn net.Conn
    var err error
    for {
        conn, err = net.Dial("tcp", ADDR)
        if err == nil {
            break
        }
        time.Sleep(10 * time.Second)
    }
    return conn
}

// Connect
// Read
// if err break

func (self *Client) run() {

    self.backend.RegisterPushHook(func() {
        self.remote.Write([]byte("Updated\n"))
    })

    for {
        select {
        case ev := <-self.watcher.Event:

            if ev.Mask & inotify.IN_MODIFY != 0 {
                self.backend.Modified(ev.Name)

            } else if ev.Mask & inotify.IN_CREATE != 0 {

                if ev.Mask & inotify.IN_ISDIR != 0 &&
                   self.backend.ShouldWatch(ev.Name) {

                    log.Println("Added watch", ev.Name)
                    self.watcher.AddWatch(ev.Name, INTERESTING)
                }
                self.backend.Created(ev.Name)

            } else if ev.Mask & inotify.IN_DELETE != 0 {
                self.backend.Deleted(ev.Name)
            }

        case err := <-self.watcher.Error:
            log.Println("error:", err)
        }
    }
}

// Add inotify watches on rootDir and all sub-dirs
func (self *Client) addWatches() {

    addSingleWatch := func(path string, info os.FileInfo, err error) error {
        if info.IsDir() && self.backend.ShouldWatch(path) {
            log.Println("Watching", path)
            self.watcher.AddWatch(path, INTERESTING)
        }
        return nil
    }

    err := filepath.Walk(self.rootDir, addSingleWatch)
    if err != nil {
        log.Fatal(err)
    }
}
