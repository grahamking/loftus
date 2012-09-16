// Run a series of checks on the environment, aborting if any errors
package main

import (
	"errors"
	"log"
	"os"
	"os/exec"
)

// Run a series of checks on the environment, aborting if any errors
func CheckEverything(external External, syncDir string, backend *GitBackend, config *Config) {

	abortOnErr := func(err error) {
		if err == nil {
			return
		}
		log.Println(err)
		external.Exec("", CMD_ALERT, err.Error())
		os.Exit(1)
	}

	abortOnErr(checkDir(syncDir))
	abortOnErr(checkIsRepo(backend))
	checkHelperScripts() // Information only, no errors

	// Only check the connection if one is configured
	if checkRemoteConfig(config) {
		abortOnErr(checkRemoteConnection(config.serverAddr))
	}
}

// Check sync directory is accessible.
// If any error is returned program should abort.
func checkDir(syncDir string) error {

	info, err := os.Stat(syncDir)
	if err != nil {
		return err
	}

	if !info.IsDir() {
		return errors.New(syncDir + " is not a directory")
	}

	return nil
}

// Is the given directory a git repository?
func checkIsRepo(backend *GitBackend) error {
	err := backend.git("status")
	if err != nil {
		return errors.New(backend.rootDir + " is not a git repository")
	}
	return nil
}

// Check the alert and info helper scripts are present
func checkHelperScripts() {
	var path, msg string
	var err error

	path, err = exec.LookPath(CMD_ALERT)
	if err != nil {
		msg = "Could not find executable '" + CMD_ALERT + "' in your path. This is needed if you run loftus in the background.\n"
		msg += "Suggested contents:\n---\n" + SUGGEST_CMD_ALERT + "\n---"
		log.Println(msg)
	} else {
		log.Println("Found alert helper:", path)
	}

	path, err = exec.LookPath(CMD_INFO)
	if err != nil {
		msg = "Could not find executable '" + CMD_INFO + "' in your path. This is needed if you run loftus in the background.\n"
		msg += "Suggested contents:\n---\n" + SUGGEST_CMD_INFO + "\n---"
		log.Println(msg)
	} else {
		log.Println("Found info helper:", path)
	}
}

// Check if a remote server is configured
func checkRemoteConfig(config *Config) bool {

	if len(config.serverAddr) != 0 {
		return true
	}

	msg := "No sync server (--address) defined. "
	msg += "Unless all your machines are on the same local network, "
	msg += "you will need to specify --address=... for sync to work."
	log.Println(msg)

	return false
}

// Can we see the remote server?
func checkRemoteConnection(serverAddr string) error {

	log.Println("Connecting to sync server at", serverAddr)

	conn := getRemoteConnection(serverAddr, false)
	if conn == nil {
		return errors.New("Cannot connect to sync server: " + serverAddr)
	}

	err := tcpSend(conn, "Test\n")
	if err != nil {
		return errors.New("Cannot send data to remote server. " + err.Error())
	}

	return nil
}
