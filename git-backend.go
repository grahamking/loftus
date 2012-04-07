package main

import (
    "os"
    "log"
    "strings"
    "os/exec"
    "time"
    "sync"
)

const (
    SYNC_PAUSE = 2
    PUSH_PAUSE = 10
)

type GitBackend struct {
    logger *log.Logger
    gitPath string

    rootDir string

    syncLock sync.Mutex
    isSyncPending bool

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
    self.action(filename)
}

// A file has been modified
func (self *GitBackend) Modified(filename string) {
    self.action(filename)
}

// A file has been deleted
func (self *GitBackend) Deleted(filename string) {
    self.action(filename)
}

// An inotify event
func (self *GitBackend) action(filename string) {
    if self.isGit(filename) || self.isPullActive {
        return
    }
    go self.syncLater()
}

// Fetch data from remote
func (self *GitBackend) Fetch() {
    self.pull()
}

// Run: git add --all ; git commit --all
func (self *GitBackend) Sync() {
    self.git("add", "--all")
    self.git("commit", "--all", "--message=bup")
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

    go self.pushLater()

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
