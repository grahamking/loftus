package main

import (
	"flag"
	"loftus/inotify"
	"log"
	"os"
	"path/filepath"
	"strings"
)

const (
	INTERESTING = inotify.IN_MODIFY | inotify.IN_CREATE | inotify.IN_DELETE | inotify.IN_MOVE

	DEFAULT_SYNC_DIR = "/loftus"

	CMD_ALERT = "loftus_alert"
	CMD_INFO  = "loftus_info"

	SUGGEST_CMD_ALERT = "#!/bin/bash\nzenity --warning --title=loftus --text=\"$1\""
	SUGGEST_CMD_INFO  = "#!/bin/bash\nnotify-send loftus \"$1\""
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
}

type Client struct {
	backend  Backend
	rootDir  string
	watcher  *inotify.Watcher
	external External
	incoming chan string
}

func main() {

	config := confFromFlags()

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

	flag.Parse()

	return &Config{
		isServer:   *isServer,
		serverAddr: *serverAddr,
		syncDir:    *syncDir}
}

// Watch directories, called sync methods on backend, etc
func startClient(config *Config) {

	syncDir := config.syncDir
	syncDir = strings.TrimRight(syncDir, "/")
	log.Println("Synchronising:", syncDir)

	external := &RealExternal{}
	backend := NewGitBackend(config, external)

	CheckEverything(external, syncDir, backend, config)

	watcher, _ := inotify.NewWatcher()

	incomingChannel := make(chan string)

	client := Client{
		rootDir:  syncDir,
		backend:  backend,
		watcher:  watcher,
		external: external,
		incoming: incomingChannel,
	}
	client.addWatches()

	go udpListen(incomingChannel)
	go tcpListen(config.serverAddr, incomingChannel)
	client.run()
}

// Main loop
func (self *Client) run() {

	// Always start with a sync to bring us up to date
	err := self.backend.Sync()
	if err != nil && err.(*GitError).status != 1 {
		self.warn(err.Error())
	}

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

// Utility function to warn user about something - for example a git error
func (self *Client) warn(msg string) {
	self.external.Exec("", CMD_ALERT, msg)
	//cmd := exec.Command(CMD_ALERT, msg)
	//cmd.Run()
}
