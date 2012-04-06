package main

import (
    "os"
    "log"
    "fmt"
    "strings"
    "os/exec"
    "path/filepath"
    "time"
    "sync"
)

const (
    COMMIT_PAUSE = 2
    PUSH_PAUSE = 10
)

type GitBackend struct {
    logger *log.Logger
    gitPath string

    rootDir string

    commitLock sync.Mutex
    isCommitPending bool

    pushLock sync.Mutex
    isPushPending bool

    pushHook func()

    isPullActive bool   // Is a pull currently running - ignore all events
}

func NewGitBackend(rootDir string, logDir string) *GitBackend {

    logger := openLog(logDir)

    gitPath, err := exec.LookPath("git")
    if err != nil {
        log.Fatal("Error looking for 'git' on path. ", err)
    }

    return &GitBackend{
        logger: logger,
        rootDir: rootDir,
        gitPath: gitPath}
}

func openLog(logDir string) *log.Logger {

    writer, err := os.OpenFile(
        logDir + "git.log", os.O_RDWR|os.O_APPEND|os.O_CREATE, 0650)

    if err != nil {
        log.Fatal("Error opening log file git.log in ", logDir, err)
    }

    return log.New(writer, "", log.LstdFlags)
}

// A file or directory has been created
func (self *GitBackend) Created(filename string) {
    if self.isGit(filename) || self.isPullActive {
        return
    }
    fmt.Println("GitBackend created:", filename)
    relPart, _ := filepath.Rel(self.rootDir, filename)
    self.git("add", relPart)
    go self.commitLater()
}

// A file has been modified
func (self *GitBackend) Modified(filename string) {
    if self.isGit(filename) || self.isPullActive{
        return
    }
    fmt.Println("GitBackend modified:", filename, "in", self.rootDir)
    go self.commitLater()
}

// A file has been deleted
func (self *GitBackend) Deleted(filename string) {
    if self.isGit(filename) || self.isPullActive{
        return
    }
    fmt.Println("GitBackend deleted:", filename)

    relPart, _ := filepath.Rel(self.rootDir, filename)
    self.git("rm", relPart)
    go self.commitLater()
}

// Fetch data from remote
func (self *GitBackend) Fetch() {
    self.pull()
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

// Schedule a commit for in a few seconds
func (self *GitBackend) commitLater() {

    // ensure only once per time - might be able to use sync.Once instead (?)
    self.commitLock.Lock()
    if self.isCommitPending {
        self.commitLock.Unlock()
        return
    }
    self.isCommitPending = true
    self.commitLock.Unlock()

    time.Sleep(COMMIT_PAUSE * time.Second)
    self.commit()

    go self.pushLater()

    self.isCommitPending = false
}

// Run: git commit --all
func (self *GitBackend) commit() {
    self.git("commit", "--all", "--message=bup")
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
    self.push()

    self.isPushPending = false
}

// Run: git push
func (self *GitBackend) push() {
    self.git("push")
    self.pushHook()
}

// Run: git pull
func (self *GitBackend) pull() {
    self.isPullActive = true
    self.git("pull")
    self.isPullActive = false
}

func (self *GitBackend) git(gitCmd string, args ...string) {

    cmd := exec.Command(self.gitPath, append([]string{gitCmd}, args...)...)
    cmd.Dir = self.rootDir
    self.logger.Println(cmd)

    output, err := cmd.CombinedOutput()
    if err != nil {
        self.logger.Println(err)
    }
    self.logger.Println(string(output))
}
