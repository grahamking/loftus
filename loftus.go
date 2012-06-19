package main

import (
	"flag"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
    "fmt"
	"loftus/inotify"
)

const (
	INTERESTING      = inotify.IN_MODIFY | inotify.IN_CREATE | inotify.IN_DELETE | inotify.IN_MOVE

	DEFAULT_SYNC_DIR = "/loftus"
	DEFAULT_LOG_DIR  = "/.loftus/"

    CMD_ALERT = "loftus_alert"
    CMD_INFO = "loftus_info"

    SUGGEST_CMD_ALERT = "#!/bin/bash\nzenity --warning --title='loftus' --text='$1'"
    SUGGEST_CMD_INFO = "#!/bin/bash\nnotify-send loftus '$1'"
)

type Backend interface {
	Sync() error
	Changed(filename string)
	ShouldWatch(filename string) bool
	RegisterPushHook(func())
}

type Config struct {
	isServer   bool
	isCheck    bool
	serverAddr string
	syncDir    string
	logDir     string
	stdout     bool
}

type Client struct {
	backend  Backend
	rootDir  string
	watcher  *inotify.Watcher
	incoming chan string
}

func main() {

	config := confFromFlags()
	log.Println("Logging to ", config.logDir)

	os.Mkdir(config.logDir, 0750)
    if ! config.stdout {
        logTo(config.logDir + "loftus.log")
    }

	if config.isServer {
        log.Println("Server mode")
		startServer(config)
	} else {
        log.Println("Client mode")
		startClient(config)
	}
}

// Parse commands line flags in to a configuration object
func confFromFlags() *Config {

	defaultSync := os.Getenv("HOME") + DEFAULT_SYNC_DIR
	var syncDir = flag.String(
		"dir",
		defaultSync,
		"Synchronise this directory. Must already be a git repo with a remote (i.e. 'git pull' works)")

	var isServer = flag.Bool("server", false, "Be the server")
	var serverAddr = flag.String(
		"address",
		"",
        "address:port where server is listening. e.g. an.example.com:8007")

	defaultLog := os.Getenv("HOME") + DEFAULT_LOG_DIR
	var logDir = flag.String("log", defaultLog, "Log directory")

	var stdout = flag.Bool("stdout", false, "Log to stdout")

	flag.Parse()

	return &Config{
		isServer:   *isServer,
		serverAddr: *serverAddr,
		syncDir:    *syncDir,
		logDir:     *logDir,
		stdout:     *stdout}
}

// Watch directories, called sync methods on backend, etc
func startClient(config *Config) {

	syncDir := config.syncDir
	syncDir = strings.TrimRight(syncDir, "/")
	log.Println("Synchronising:", syncDir)
    checkDir(syncDir)

	backend := NewGitBackend(config)
    checkIsRepo(backend)

    checkHelperScripts()

    checkRemoteConfig(config)

	watcher, _ := inotify.NewWatcher()

	incomingChannel := make(chan string)

	client := Client{
		rootDir:  syncDir,
		backend:  backend,
		watcher:  watcher,
		incoming: incomingChannel,
	}
	client.addWatches()

	// Always start with a sync to bring us up to date
	err := backend.Sync()
	if err != nil && err.(*GitError).status != 1 {
		Warn(err.Error())
	}

	go udpListen(incomingChannel)
	go tcpListen(config.serverAddr, incomingChannel)
	client.run()
}

// Set log output to given fullpath
func logTo(fullpath string) {

	writer, err := os.OpenFile(
		fullpath, os.O_RDWR|os.O_APPEND|os.O_CREATE, 0650)

	if err != nil {
		log.Fatal("Error opening log file", fullpath, err)
	}

    log.SetOutput(writer)
}

// Check sync directory is accessible
func checkDir(syncDir string) {

    info, err := os.Stat(syncDir)

    if err != nil {
        Warn(err.Error())
        fmt.Println(err)
        os.Exit(1)
    }

    if ! info.IsDir() {
        msg := syncDir + " is not a directory"
        Warn(msg)
        fmt.Println(msg)
        os.Exit(1)
    }
}

// Is the given directory a git repository?
func checkIsRepo(backend *GitBackend) {
    err := backend.git("status")
    if err != nil {
        msg := backend.rootDir + " is not a git repository"
        Warn(msg)
        fmt.Println(msg)
        os.Exit(1)
    }
}

// Check the alert and info helper scripts are present
func checkHelperScripts() {
    var path, msg string
    var err error

    path, err = exec.LookPath(CMD_ALERT)
    if err != nil {
        msg = "Could not find executable '" + CMD_ALERT + "' in your path. This is needed if you run loftus in the background.\n"
        msg += "Suggested contents:\n---\n" + SUGGEST_CMD_ALERT +"\n---"
        log.Println(msg)
        fmt.Println(msg)
    } else {
        log.Println("Found alert helper:", path)
    }

    path, err = exec.LookPath(CMD_INFO)
    if err != nil {
        msg = "Could not find executable '" + CMD_INFO + "' in your path. This is needed if you run loftus in the background.\n"
        msg += "Suggested contents:\n---\n" + SUGGEST_CMD_INFO +"\n---"
        log.Println(msg)
        fmt.Println(msg)
    } else {
        log.Println("Found info helper:", path)
    }
}

// Check if a remote server is configured
func checkRemoteConfig(config *Config) {

    if config.serverAddr == "" {
        msg := "Server address missing from command line. "
        msg += "Unless all your machines are on the same local network, "
        msg += "you will need to specify --address=... for sync to work."
        log.Println(msg)
        fmt.Println("No sync server (--address) defined")
    } else {
        log.Println("Connecting to sync server at", config.serverAddr)
        checkRemoteConnection(config.serverAddr)
    }
}

// Can we see the remote server?
func checkRemoteConnection(serverAddr string) {

    var msg string

    conn := getRemoteConnection(serverAddr, false)
    if conn == nil {
        msg = "Cannot connect to sync server: " + serverAddr
        msg += ". Will keep trying."
        Warn(msg)
        fmt.Println(msg)
        return
    }

    err := tcpSend(conn, "Test\n")
    if err != nil {
        msg = "Cannot send data to remote server. " + err.Error()
        Warn(msg)
        fmt.Println(msg)
        os.Exit(1)
    }
}

// Main loop
func (self *Client) run() {

	// push hook will be called from a go routine
	self.backend.RegisterPushHook(func() {
		msg := "Updated\n"
		if remoteConn != nil { // remoteConn is global in comms.go
			tcpSend(remoteConn, msg)
		}
		udpSend(msg)
	})

	for {
		select {
		case ev := <-self.watcher.Event:

			log.Println(ev)

			isCreate := ev.Mask&inotify.IN_CREATE != 0
			isDir := ev.Mask&inotify.IN_ISDIR != 0

			if isCreate && isDir && self.backend.ShouldWatch(ev.Name) {
				log.Println("Adding watch", ev.Name)
				self.watcher.AddWatch(ev.Name, INTERESTING)
			}

			self.backend.Changed(ev.Name)

		case err := <-self.watcher.Error:
			log.Println("error:", err)

		case <-self.incoming:
			log.Println("Remote update notification")
			self.backend.Sync()
		}

	}
}

// Add inotify watches on rootDir and all sub-dirs
func (self *Client) addWatches() {

	addSingleWatch := func(path string, info os.FileInfo, err error) error {
		if info.IsDir() && self.backend.ShouldWatch(path) {
            //log.Println("Watching", path)
			self.watcher.AddWatch(path, INTERESTING)
		}
		return nil
	}

    log.Println("Watching", self.rootDir, "and all sub-directories")
	err := filepath.Walk(self.rootDir, addSingleWatch)
	if err != nil {
		log.Fatal(err)
	}
}

// Utility function to inform user about something - for example file changes
func Info(msg string) {
	cmd := exec.Command(CMD_INFO, msg)
	cmd.Run()
}

// Utility function to warn user about something - for example a git error
func Warn(msg string) {
	cmd := exec.Command(CMD_ALERT, msg)
	cmd.Run()
}
