package main

import (
	"flag"
	"log"
	"os"
	"strings"
	"time"
)

const (
	DEFAULT_SYNC_DIR = "/loftus"
	SYNC_IDLE_SECS   = 5

	CMD_ALERT = "loftus_alert"
	CMD_INFO  = "loftus_info"

	SUGGEST_CMD_ALERT = "#!/bin/bash\nzenity --warning --title=loftus --text=\"$1\""
	SUGGEST_CMD_INFO  = "#!/bin/bash\nnotify-send loftus \"$1\""
)

type Storage interface {

	// Perform all possible sanity checks, returning a user-helpful error
	Check() error

	// Can we contact remote storage server (i.e. git remote)
	IsOnline() bool

	// Update storage from remote storage server
	Pull() error

	// Add all files to the storage
	AddAll() error

	// Commit files to storage
	Commit(string) error

	// Send files to remote storage server
	Push() error
}

type Config struct {
	isServer   bool
	isCheck    bool
	serverAddr string
	syncDir    string
}

type Client struct {
	backend  Storage
	watch    chan string
	external External
	incoming chan string
	isOnline bool
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

// Watch directories, called sync methods on syncer, etc
func startClient(config *Config) {

	syncDir := config.syncDir
	syncDir = strings.TrimRight(syncDir, "/")
	log.Println("Synchronising:", syncDir)

	external := &RealExternal{}
	backend := NewGitBackend(config, external)

	CheckEverything(external, syncDir, backend, config)

	log.Println("Watching", syncDir, "and all sub-directories")
	watchChannel, err := Watch(syncDir)
	if err != nil {
		log.Fatal(err)
	}

	incomingChannel := make(chan string)

	client := Client{
		backend:  backend,
		watch:    watchChannel,
		external: external,
		incoming: incomingChannel,
		isOnline: true,
	}

	go udpListen(incomingChannel)
	go tcpListen(config.serverAddr, incomingChannel)
	client.run()
}

// Main loop
func (self *Client) run() {

	// Always start with a sync to bring us up to date
	err := self.Sync()
	if err != nil {
		self.warn(err.Error())
	}

	isSyncPending := false

	for {
		select {

		case <-self.watch:
			isSyncPending = true

		case <-self.incoming:
			log.Println("Remote update notification")
			self.Sync()

		case <-time.After(SYNC_IDLE_SECS * time.Second):
			if isSyncPending {
				isSyncPending = false
				self.Sync()
			}
		}
	}

}

// Run: git pull; git add --all ; git commit --all; git push
func (self *Client) Sync() error {

	log.Println("* Sync start")

	var err error

	//self.checkGitConnection()
	isOnline := self.backend.IsOnline()
	if isOnline != self.isOnline {

		self.isOnline = isOnline

		if isOnline {
			log.Println("Back online")
			self.info("Back online")
		} else {
			log.Println("Working offline")
			self.info("Could not connect to remote git repo. Working offline.")
		}
	}

	if self.isOnline {
		// Pull first to ensure a fast-forward when we push
		err = self.backend.Pull()
		if err != nil {
			log.Println("Returning error from Pull")
			return err
		}
	}

	//err = self.git("add", "--all")
	err = self.backend.AddAll()
	if err != nil {
		return err
	}

	// TODO: Build this from inotify. Backend might not know.
	//commitMsg := self.displayStatus("status", "--porcelain")

	//err = self.git("commit", "--all", "--message="+commitMsg)
	self.backend.Commit("TODO")
	if err != nil {
		return err
	}

	if self.isOnline { //&& self.isBehindRemote() {
		err = self.backend.Push()
		if err != nil {
			return err
		}
		self.broadcast()
	}

	log.Println("* Sync end")
	return nil
}

// Tell other loftus instances to update themselves, because something changed.
func (self *Client) broadcast() {
	msg := "Updated\n"
	if remoteConn != nil { // remoteConn is global in comms.go
		tcpSend(remoteConn, msg)
	}
	udpSend(msg)
}

// Utility function to warn user about something - for example a git error
func (self *Client) warn(msg string) {
	self.external.Exec("", CMD_ALERT, msg)
}

// Utility function to inform user about something - for example file changes
func (self *Client) info(msg string) {
	self.external.Exec("", CMD_INFO, msg)
}

