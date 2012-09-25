package main

import (
	"errors"
	"log"
	"os/exec"
	"strings"
	"syscall"
)

const (
	MAX_SUMMARY_NAMES = 3
)

type GitBackend struct {
	external External
	gitPath  string
	rootDir  string
	isOnline bool // Can we talk to remote git / ssh server?
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

// Display summary of changes, and return that summary
/*
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
*/

/*
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
*/

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

// Run: git push
func (self *GitBackend) Push() error {
	err := self.git("push")
	if err == nil && self.pushHook != nil {
		go self.pushHook()
	}
	return err
}

// Run: git pull
func (self *GitBackend) Pull() error {

	err := self.git("fetch")
	if err != nil {
		return err
	}

	//self.displayStatus("diff", "origin/master", "--name-status")
	return self.git("merge", "origin/master")
}

// Run: git add --all
func (self *GitBackend) AddAll() error {
	return self.git("add", "--all")
}

// Run: git commit --all --message=..
func (self *GitBackend) Commit(msg string) error {
	return self.git("commit", "--all", "--message="+msg)
}

// Run: git remote show origin
// We use this to check if we are online
func (self *GitBackend) IsOnline() bool {
	return self.git("remote", "show", "origin") == nil
}

// Check our directory is actualy a repository
func (self *GitBackend) Check() error {
	err := self.git("status")
	if err != nil {
		return errors.New(self.rootDir + " is not a git repository")
	}
	return nil
}

// Is the local repo behind the remote, i.e. is a push needed?
/*
func (self *GitBackend) isBehindRemote() bool {

	created, modified, deleted := self.status("diff", "origin/master", "--name-status")
	return (len(created) != 0 || len(modified) != 0 || len(deleted) != 0)
}
*/

// Runs a git command, returns nil if success, error if err
func (self *GitBackend) git(gitCmd string, args ...string) error {
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
		return gitErr
	}
	return nil
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
