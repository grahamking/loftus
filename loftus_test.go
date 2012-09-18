package main

import (
	"fmt"
	"strings"
	"testing"
)

func TestMainLoop(t *testing.T) {

	config := &Config{
		isServer:   false,
		serverAddr: "test.local",
		syncDir:    "/tmp/fake"}

	external := &MockExternal{}
	backend := NewGitBackend(config, external)

	watchChannel := make(chan string)
	incomingChannel := make(chan string)

	client := Client{
		backend: backend,
		watch: watchChannel,
		external: external,
		incoming: incomingChannel,
	}

	go client.run()

	// Something changed.
	watchChannel <- "/tmp/fake/one.txt"

	expected := []string{
		"/usr/bin/git remote show origin",
		"/usr/bin/git fetch",
		"/usr/bin/git merge origin/master",
		"/usr/bin/git add --all",
		"/usr/bin/git commit --all --message=",
	}
	if fmt.Sprintf("%v", external.cmds) != fmt.Sprintf("%v", expected) {
		t.Error("Unexpected exec: ", external.cmds)
	}
}

type MockExternal struct {
	cmds []string
}

func (self *MockExternal) Exec(rootDir string, cmd string, args ...string) ([]byte, error) {


	self.cmds = append(self.cmds, cmd + " " + strings.Join(args, " "))
	return []byte(""), nil
}
