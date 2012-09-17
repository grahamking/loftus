package main

import (
	"flag"
	"log"
	"os"
	"strings"
)

const (
	DEFAULT_SYNC_DIR = "/loftus"

	CMD_ALERT = "loftus_alert"
	CMD_INFO  = "loftus_info"

	SUGGEST_CMD_ALERT = "#!/bin/bash\nzenity --warning --title=loftus --text=\"$1\""
	SUGGEST_CMD_INFO  = "#!/bin/bash\nnotify-send loftus \"$1\""
)

type Backend interface {
	Sync() error
	Changed(filename string)
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
	watch    chan string
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
	}

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
		case name := <-self.watch:
			self.backend.Changed(name)

		case <-self.incoming:
			log.Println("Remote update notification")
			self.backend.Sync()
		}

	}
}

// Utility function to warn user about something - for example a git error
func (self *Client) warn(msg string) {
	self.external.Exec("", CMD_ALERT, msg)
	//cmd := exec.Command(CMD_ALERT, msg)
	//cmd.Run()
}
