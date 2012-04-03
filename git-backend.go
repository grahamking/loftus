package main

import (
    "log"
    "fmt"
    "strings"
    "os/exec"
    "path/filepath"
)

const (
    GIT_BIN = "/usr/bin/git"
)

type GitBackend struct {
    RootDir string
}

func (self *GitBackend) Created(filename string) {
    if self.isGit(filename) {
        return
    }
    fmt.Println("GitBackend created:", filename)
    self.git("add", filename)
    self.commit()
}

func (self *GitBackend) Modified(filename string) {
    if self.isGit(filename) {
        return
    }
    fmt.Println("GitBackend modified:", filename, "in", self.RootDir)
    self.commit()
}

func (self *GitBackend) Deleted(filename string) {
    if self.isGit(filename) {
        return
    }
    fmt.Println("GitBackend deleted:", filename)

    self.git("rm", filename)
    self.commit()
}

// Should the inotify watch watch the given path
func (self *GitBackend) ShouldWatch(filename string) bool {
    return !self.isGit(filename)
}

// Is filename inside a .git directory?
func (self *GitBackend) isGit(filename string) bool {
    return strings.Contains(filename, ".git")
}

func (self *GitBackend) commit() {

    cmd := exec.Command(GIT_BIN, "commit", "--all", "--message=bup")
    cmd.Dir = self.RootDir

    output, err := cmd.CombinedOutput()
    if err != nil {
        log.Println(err)
    }
    fmt.Println(string(output))
}

func (self *GitBackend) push() {
    git("push", nil)
}

func (self *GitBackend) git(gitCmd string, filename string) {

    if filename != nil {
        relPart, _ := filepath.Rel(self.RootDir, filename)
        cmd := exec.Command(GIT_BIN, gitCmd, relPart)
    } else {
        cmd := exec.Command(GIT_BIN, gitCmd)
    }
    cmd.Dir = self.RootDir

    output, err := cmd.CombinedOutput()
    if err != nil {
        log.Println(err)
    }
    fmt.Println(string(output))
}
