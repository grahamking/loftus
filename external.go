// Interface to outside world (Exec and Notify), to facilitate unit testing
package main

import (
	"log"
	"os/exec"
	"strings"
)

type External interface {
	// Run a third party app, return it's output and any error
	Exec(rootDir string, cmd string, args ...string) ([]byte, error)
}

type RealExternal struct{}

func (self *RealExternal) Exec(rootDir string, cmd string, args ...string) ([]byte, error) {

	cmdObj := exec.Command(cmd, args...)
	if len(rootDir) > 0 {
		cmdObj.Dir = rootDir
	}

	log.Println(cmd, strings.Join(args, " "))

	return cmdObj.CombinedOutput()
}
