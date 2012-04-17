package main

import (
	"log"
	"os/exec"
	"strings"
	"sync"
	"syscall"
	"time"
)

const (
	SYNC_PAUSE = 2
	PUSH_PAUSE = 10
)

type GitBackend struct {
	logger  *log.Logger
	gitPath string

	rootDir string

	syncLock      sync.Mutex
	isSyncPending bool

	pushLock      sync.Mutex
	isPushPending bool

	pushHook func()

	isPullActive bool // Is a pull currently running - ignore all events
}

func NewGitBackend(config *Config) *GitBackend {

	rootDir := config.syncDir
	logger := openLog(config, "git.log")

	gitPath, err := exec.LookPath("git")
	if err != nil {
		logger.Fatal("Error looking for 'git' on path. ", err)
	}

	return &GitBackend{
		logger:  logger,
		rootDir: rootDir,
		gitPath: gitPath,
	}
}

// A file or directory has been created
func (self *GitBackend) Changed(filename string) {
	if self.isGit(filename) || self.isPullActive {
		return
	}
	go self.syncLater()
}

// Run: git add --all ; git commit --all
func (self *GitBackend) Sync() error {

	var err error

	// Pull first to ensure a fast-forward when we push
	err = self.pull()
	if err != nil {
		return err
	}

	err = self.git("add", "--all")
	if err != nil {
		return err
	}

	err = self.git("commit", "--all", "--message=bup")
	if err != nil {
		return err
	}

	go self.pushLater()

	return nil
}

// Register the function to be called after we push to remote
func (self *GitBackend) RegisterPushHook(callback func()) {
	self.pushHook = callback
}

// Should the inotify watch watch the given path
func (self *GitBackend) ShouldWatch(filename string) bool {
	return !self.isGit(filename)
}

// Is filename inside a .git directory?
func (self *GitBackend) isGit(filename string) bool {
	return strings.Contains(filename, ".git")
}

// Schedule a synchronise for in a few seconds
func (self *GitBackend) syncLater() {

	// ensure only once per time - might be able to use sync.Once instead (?)
	self.syncLock.Lock()
	if self.isSyncPending {
		self.syncLock.Unlock()
		return
	}
	self.isSyncPending = true
	self.syncLock.Unlock()

	time.Sleep(SYNC_PAUSE * time.Second)
	self.Sync()

	self.isSyncPending = false
}

// Schedule a push for later
func (self *GitBackend) pushLater() {

	// ensure only once per time - might be able to use sync.Once instead (?)
	self.pushLock.Lock()
	if self.isPushPending {
		self.pushLock.Unlock()
		return
	}
	self.isPushPending = true
	self.pushLock.Unlock()

	time.Sleep(PUSH_PAUSE * time.Second)
	err := self.push()
	if err != nil {
		Warn(err.Error())
	}

	self.isPushPending = false
}

// Run: git push
func (self *GitBackend) push() error {
	err := self.git("push")
	if err == nil && self.pushHook != nil {
		go self.pushHook()
	}
	return err
}

// Run: git pull
func (self *GitBackend) pull() error {
	self.isPullActive = true
	err := self.git("pull")
	self.isPullActive = false
	return err
}

/* Runs a git command, returns nil if success, error if err
   Errors are not always bad. For example a "commit" that
   didn't have to do anything returns an error.
*/
func (self *GitBackend) git(gitCmd string, args ...string) error {

	cmd := exec.Command(self.gitPath, append([]string{gitCmd}, args...)...)
	cmd.Dir = self.rootDir
	self.logger.Println(cmd)

	output, err := cmd.CombinedOutput()
	self.logger.Println(string(output))

	if err != nil {

		exitStatus := err.(*exec.ExitError).Sys().(syscall.WaitStatus).ExitStatus()
		if exitStatus != 1 {
			self.logger.Println(err)
			gitErr := &GitError{
                cmd: strings.Join(cmd.Args, " "),
                internalError: err,
                output: string(output)}
			return gitErr
		}
	}

	return nil
}

type GitError struct {
	cmd           string
	internalError error
	output        string
}

// error implementation which displays git info
func (self *GitError) Error() string {
    msg := "git error running: " + self.cmd + "\n\n"
    msg += self.output + "\n"
	msg += self.internalError.Error()
    return msg
}
