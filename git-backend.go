package main

import (
	"log"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

const (
	SYNC_IDLE_SECS    = 5
	MAX_SUMMARY_NAMES = 3
)

type GitBackend struct {
	external External
	gitPath  string

	rootDir string

	syncLock      sync.Mutex
	isSyncPending bool
	isSyncActive  bool // Ignore all events during sync
	isOnline      bool // Can we talk to remote git / ssh server?

	lastEvent time.Time

	pushHook func()
}

func NewGitBackend(config *Config, external External) *GitBackend {

	rootDir := config.syncDir

	gitPath, err := exec.LookPath("git")
	if err != nil {
		log.Fatal("Error looking for 'git' on path. ", err)
	}

	return &GitBackend{
		rootDir:  rootDir,
		gitPath:  gitPath,
		external: external,
		isOnline: true}
}

// A file or directory has been created
func (self *GitBackend) Changed(filename string) {
	if self.isGit(filename) || self.isSyncActive {
		return
	}
	self.lastEvent = time.Now()
	go self.syncLater()
}

// Run: git pull; git add --all ; git commit --all; git push
func (self *GitBackend) Sync() error {

	log.Println("* Sync start")
	self.isSyncActive = true

	var err *GitError

	self.checkGitConnection()

	if self.isOnline {
		// Pull first to ensure a fast-forward when we push
		err = self.pull()
		if err != nil {
			self.isSyncActive = false
			return err
		}
	}

	err = self.git("add", "--all")
	if err != nil {
		self.isSyncActive = false
		return err
	}

	commitMsg := self.displayStatus("status", "--porcelain")

	err = self.git("commit", "--all", "--message="+commitMsg)
	// An err with status==1 means nothing to commit, it's not an error
	if err != nil && err.status != 1 {
		self.isSyncActive = false
		return err
	}

	if self.isOnline && self.isBehindRemote() {
		err = self.push()
		if err != nil {
			self.isSyncActive = false
			return err
		}
	}

	self.isSyncActive = false
	log.Println("* Sync end")
	return nil
}

// Check whether we can talk to remote git repo, and inform
func (self *GitBackend) checkGitConnection() {

	isOnline := self.isOnlineCheck()

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
}

// Display summary of changes, and return that summary
func (self *GitBackend) displayStatus(args ...string) string {

	created, modified, deleted := self.status(args...)

	msg := summaryMsg(created, "New") +
		summaryMsg(modified, "Edit") +
		summaryMsg(deleted, "Del")

	if len(msg) != 0 {
		self.info(msg)
	}
	return msg
}

// Short summary of what's in 'changed'.
func summaryMsg(changed []string, action string) string {

	var msg string
	var fs []string

	for pos, filename := range changed {

		if pos >= MAX_SUMMARY_NAMES {
			remain := len(changed) - MAX_SUMMARY_NAMES
			fs = append(fs, "and "+strconv.Itoa(remain)+" more.")
			break
		}

		fs = append(fs, filename)
	}

	if len(fs) != 0 {
		msg = action + ": " + strings.Join(fs, ", ")
	}

	return msg
}

// Register the function to be called after we push to remote
func (self *GitBackend) RegisterPushHook(callback func()) {
	self.pushHook = callback
}

// Status of directory. Returns filenames created, modified or deleted.
func (self *GitBackend) status(args ...string) (created []string, modified []string, deleted []string) {

	cmd := exec.Command(self.gitPath, args...)
	log.Println(strings.Join(cmd.Args, " "))

	cmd.Dir = self.rootDir

	output, err := cmd.CombinedOutput()
	if err != nil {
		log.Println(err)
	}
	if len(output) > 0 {
		log.Println(string(output))
	}

	for _, line := range strings.Split(string(output), "\n") {
		if len(line) == 0 {
			continue
		}

		// Replace double spaces and tabs with single space so that Split is predictable
		line = strings.Replace(line, "  ", " ", -1)
		line = strings.Replace(line, "\t", " ", -1)

		lineParts := strings.Split(line, " ")

		status := lineParts[0]
		filename := lineParts[1]

		switch status[0] {
		case 'A':
			created = append(created, filename)
		case 'M':
			modified = append(modified, filename)
		case 'R': // Renamed, but treat as Modified
			modified = append(modified, filename)
		case 'D':
			deleted = append(deleted, filename)
		case '?':
			log.Println("Unknown. Need git add", filename)
		default:
			log.Println("Other", status)
		}
	}

	return created, modified, deleted
}

// Is filename inside a .git directory?
func (self *GitBackend) isGit(filename string) bool {
	return strings.Contains(filename, ".git")
}

// Schedule a synchronise for in a few seconds. Run it in go routine.
func (self *GitBackend) syncLater() {

	// ensure only once per time - might be able to use sync.Once instead (?)
	self.syncLock.Lock()
	if self.isSyncPending {
		self.syncLock.Unlock()
		return
	}
	self.isSyncPending = true
	self.syncLock.Unlock()

	for time.Now().Sub(self.lastEvent) < (SYNC_IDLE_SECS * time.Second) {
		time.Sleep(time.Second)
	}

	log.Println("syncLater initiated sync")
	self.Sync()

	self.isSyncPending = false
}

// Run: git push
func (self *GitBackend) push() *GitError {
	err := self.git("push")
	if err == nil && self.pushHook != nil {
		go self.pushHook()
	}
	return err
}

// Run: git pull
func (self *GitBackend) pull() *GitError {

	var err *GitError

	err = self.git("fetch")
	if err != nil {
		return err
	}

	//self.displayStatus("diff", "origin/master", "--name-status")
	err = self.git("merge", "origin/master")
	return err
}

// Run: git remote show origin
// We use this to check if we are online
func (self *GitBackend) isOnlineCheck() bool {
	return self.git("remote", "show", "origin") == nil
}

// Is the local repo behind the remote, i.e. is a push needed?
func (self *GitBackend) isBehindRemote() bool {

	created, modified, deleted := self.status("diff", "origin/master", "--name-status")
	return (len(created) != 0 || len(modified) != 0 || len(deleted) != 0)
}

/* Runs a git command, returns nil if success, error if err
   Errors are not always bad. For example a "commit" that
   didn't have to do anything returns an error.
*/
func (self *GitBackend) git(gitCmd string, args ...string) *GitError {

	allArgs := append([]string{gitCmd}, args...)
	output, err := self.external.Exec(self.rootDir, self.gitPath, allArgs...)

	if len(output) > 0 {
		log.Println(string(output))
	}

	if err == nil {
		return nil
	}

	exitStatus := err.(*exec.ExitError).Sys().(syscall.WaitStatus).ExitStatus()
	gitErr := &GitError{
		cmd:           self.gitPath + " " + strings.Join(allArgs, " "),
		internalError: err,
		output:        string(output),
		status:        exitStatus}
	if exitStatus != 1 { // 1 means command had nothing to do
		log.Println(err)
	}
	return gitErr
}

// Utility function to inform user about something - for example file changes
func (self *GitBackend) info(msg string) {
	self.external.Exec("", CMD_INFO, msg)
}

type GitError struct {
	cmd           string
	internalError error
	output        string
	status        int
}

// error implementation which displays git info
func (self *GitError) Error() string {
	msg := "git error running: " + self.cmd + "\n\n"
	msg += self.output + "\n"
	msg += self.internalError.Error()
	return msg
}
