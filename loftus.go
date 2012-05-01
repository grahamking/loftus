package main

import (
	"exp/inotify"
	"flag"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const (
	INTERESTING      = inotify.IN_MODIFY | inotify.IN_CREATE | inotify.IN_DELETE | inotify.IN_MOVE
	DEFAULT_SYNC_DIR = "/loftus/"
	DEFAULT_LOG_DIR  = "/.loftus/"
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

	if config.isCheck {
		runCheck(config)

	} else if config.isServer {
		startServer(config)

	} else {
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
		"127.0.0.1:8007",
		"address:post where server is listening")

	var isCheck = flag.Bool("check", false, "Check we are setup correctly")

	defaultLog := os.Getenv("HOME") + DEFAULT_LOG_DIR
	var logDir = flag.String("log", defaultLog, "Log directory")

	var stdout = flag.Bool("stdout", false, "Log to stdout")

	flag.Parse()

	return &Config{
		isServer:   *isServer,
		isCheck:    *isCheck,
		serverAddr: *serverAddr,
		syncDir:    *syncDir,
		logDir:     *logDir,
		stdout:     *stdout}
}

// Watch directories, called sync methods on backend, etc
func startClient(config *Config) {

	syncDir := config.syncDir
	log.Println("Synchronising: ", syncDir)

	syncDir = strings.TrimRight(syncDir, "/")
	backend := NewGitBackend(config)

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

// Utility function to inform user about something - for example file changes
func Info(msg string) {
	cmd := exec.Command("loftus_info", msg)
	cmd.Run()
}

// Utility function to warn user about something - for example a git error
func Warn(msg string) {
	cmd := exec.Command("loftus_alert", msg)
	cmd.Run()
}
