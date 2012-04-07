package main

import (
    "os"
    "log"
    "strings"
    "net"
    "bufio"
    "time"
    "flag"
    "exp/inotify"
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
    Fetch()
    Sync()
    RegisterPushHook(func())
}

func main() {

    config := confFromFlags()

    os.Mkdir(config.logDir, 0750)
    os.Mkdir(config.syncDir, 0750)

    if config.isServer {
        startServer(config.serverAddr)
    } else {
        startClient(config.syncDir, config.logDir, config.serverAddr)
    }
}

type Config struct {
    isServer bool
    serverAddr string
    syncDir string
    logDir string
}

// Parse commands line flags in to a configuration object
func confFromFlags() *Config {

    defaultSync := os.Getenv("HOME") + "/bup/"
    var syncDir = flag.String("dir", defaultSync, "Synchronise this directory. Must already be a git repo with a remote (i.e. 'git pull' works)")

    var isServer = flag.Bool("server", false, "Be the server")
    var serverAddr = flag.String("address", "127.0.0.1:8007", "address:post where server is listening")

    defaultLog := os.Getenv("HOME") + "/.bup/"
    var logDir = flag.String("log", defaultLog, "Log directory")

    flag.Parse()

    return &Config{
        isServer: *isServer,
        serverAddr: *serverAddr,
        syncDir: *syncDir,
        logDir: *logDir}
}

// Watch directories, called sync methods on backend, etc
func startClient(syncDir string, logDir string, serverAddr string) {

    log.Println("Synchronising: ", syncDir)

    syncDir = strings.TrimRight(syncDir, "/")
    backend := NewGitBackend(syncDir, logDir)

    watcher, _ := inotify.NewWatcher()

    client := Client{rootDir: syncDir, backend: backend, watcher: watcher}
    client.addWatches()

    // Always start with a sync and fetch to bring us up to date
    backend.Sync()
    backend.Fetch()

    go client.listenRemote(serverAddr)
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
func (self *Client) listenRemote(serverAddr string) {

    for {
        self.remote = getRemoteConnection(serverAddr)
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
func getRemoteConnection(serverAddr string) net.Conn {

    var conn net.Conn
    var err error
    for {
        conn, err = net.Dial("tcp", serverAddr)
        if err == nil {
            break
        }
        time.Sleep(10 * time.Second)
    }
    return conn
}

func (self *Client) run() {

    self.backend.RegisterPushHook(func() {
        if self.remote != nil {
            self.remote.Write([]byte("Updated\n"))
        }
    })

    for {
        select {
        case ev := <-self.watcher.Event:

            log.Println(ev)

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
