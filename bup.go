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
    INTERESTING = inotify.IN_MODIFY | inotify.IN_CREATE | inotify.IN_DELETE | inotify.IN_MOVE
)

type ChangeAgent interface {
    Sync()
    Changed(filename string)
    ShouldWatch(filename string) bool
    RegisterPushHook(func())
}

type Config struct {
    isServer bool
    serverAddr string
    syncDir string
    logDir string
    stdout bool
}

type Client struct {
    backend ChangeAgent
    rootDir string
    watcher *inotify.Watcher
    remote net.Conn
    logger *log.Logger
}

func main() {

    config := confFromFlags()
    logDir := config.logDir
    log.Println("Logging to ", logDir)

    os.Mkdir(config.logDir, 0750)
    os.Mkdir(config.syncDir, 0750)

    if config.isServer {
        startServer(config)
    } else {
        startClient(config)
    }
}

func openLog(config *Config, name string) *log.Logger {

    if config.stdout {
        return log.New(os.Stdout, "", log.LstdFlags)
    }

    writer, err := os.OpenFile(
        config.logDir + name, os.O_RDWR|os.O_APPEND|os.O_CREATE, 0650)

    if err != nil {
        log.Fatal("Error opening log file", name, " in ", config.logDir, err)
    }

    return log.New(writer, "", log.LstdFlags)
}

// Parse commands line flags in to a configuration object
func confFromFlags() *Config {

    defaultSync := os.Getenv("HOME") + "/bup/"
    var syncDir = flag.String("dir", defaultSync, "Synchronise this directory. Must already be a git repo with a remote (i.e. 'git pull' works)")

    var isServer = flag.Bool("server", false, "Be the server")
    var serverAddr = flag.String("address", "127.0.0.1:8007", "address:post where server is listening")

    defaultLog := os.Getenv("HOME") + "/.bup/"
    var logDir = flag.String("log", defaultLog, "Log directory")

    var stdout = flag.Bool("stdout", false, "Log to stdout")

    flag.Parse()

    return &Config{
        isServer: *isServer,
        serverAddr: *serverAddr,
        syncDir: *syncDir,
        logDir: *logDir,
        stdout: *stdout}
}

// Watch directories, called sync methods on backend, etc
func startClient(config *Config) {

    syncDir := config.syncDir

    logger := openLog(config, "client.log")

    logger.Println("Synchronising: ", syncDir)

    syncDir = strings.TrimRight(syncDir, "/")
    backend := NewGitBackend(config)

    watcher, _ := inotify.NewWatcher()

    client := Client{
        rootDir: syncDir,
        backend: backend,
        watcher: watcher,
        logger: logger,
    }
    client.addWatches()

    // Always start with a sync to bring us up to date
    backend.Sync()

    go client.listenRemote(config.serverAddr)
    client.run()
}

// Connect to server and listen for messages, which mean we have to fetch
// new data from remote (the backend does that for us)
func (self *Client) listenRemote(serverAddr string) {

    for {
        self.remote = getRemoteConnection(serverAddr)
        defer self.remote.Close()
        self.logger.Println("Connected to remote")

        bufRead := bufio.NewReader(self.remote)
        for {
            content, err := bufRead.ReadString('\n')
            if err != nil {
                self.logger.Println("Remote read error - re-connecting")
                self.remote.Close()
                break
            }
            self.logger.Println("Remote sent: " + content)

            self.backend.Sync()
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

            self.logger.Println(ev)

            isCreate := ev.Mask & inotify.IN_CREATE != 0
            isDir := ev.Mask & inotify.IN_ISDIR != 0

            if isCreate && isDir && self.backend.ShouldWatch(ev.Name) {
                self.logger.Println("Adding watch", ev.Name)
                self.watcher.AddWatch(ev.Name, INTERESTING)
            }

            self.backend.Changed(ev.Name)

        case err := <-self.watcher.Error:
            self.logger.Println("error:", err)
        }
    }
}

// Add inotify watches on rootDir and all sub-dirs
func (self *Client) addWatches() {

    addSingleWatch := func(path string, info os.FileInfo, err error) error {
        if info.IsDir() && self.backend.ShouldWatch(path) {
            self.logger.Println("Watching", path)
            self.watcher.AddWatch(path, INTERESTING)
        }
        return nil
    }

    err := filepath.Walk(self.rootDir, addSingleWatch)
    if err != nil {
        self.logger.Fatal(err)
    }
}
