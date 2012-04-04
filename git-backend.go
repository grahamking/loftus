package main

import (
    "log"
    "fmt"
    "strings"
    "os/exec"
    "path/filepath"
    "time"
    "sync"
)

const (
    GIT_BIN = "/usr/bin/git"
    COMMIT_PAUSE = 2
    PUSH_PAUSE = 10
)

type GitBackend struct {
    RootDir string
    commitLock sync.Mutex
    isCommitPending bool
    pushLock sync.Mutex
    isPushPending bool
}

func (self *GitBackend) Created(filename string) {
    if self.isGit(filename) {
        return
    }
    fmt.Println("GitBackend created:", filename)
    relPart, _ := filepath.Rel(self.RootDir, filename)
    self.git("add", relPart)
    go self.commitLater()
}

func (self *GitBackend) Modified(filename string) {
    if self.isGit(filename) {
        return
    }
    fmt.Println("GitBackend modified:", filename, "in", self.RootDir)
    go self.commitLater()
}

func (self *GitBackend) Deleted(filename string) {
    if self.isGit(filename) {
        return
    }
    fmt.Println("GitBackend deleted:", filename)

    relPart, _ := filepath.Rel(self.RootDir, filename)
    self.git("rm", relPart)
    go self.commitLater()
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
}

func (self *GitBackend) git(gitCmd string, args ...string) {

    cmd := exec.Command(GIT_BIN, append([]string{gitCmd}, args...)...)
    cmd.Dir = self.RootDir

    output, err := cmd.CombinedOutput()
    if err != nil {
        log.Println(err)
    }
    fmt.Println(string(output))
}
