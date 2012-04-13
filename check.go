package main

import (
    "fmt"
    "os"
)

// Run a bunch of self checks
func runCheck(config *Config) {

    fmt.Println("Starting client self-test...")

    checkDir(config.syncDir)

    backend := NewGitBackend(config)

    checkIsRepo(config.syncDir, backend)

    checkPull(backend)

    checkPush(backend)

    checkRemoteConfig(config)

    if config.serverAddr != "" {
        checkRemoteConnection(config.serverAddr)
    }

    // Can see udp peer
}

// Check directory is accessible
func checkDir(syncDir string) {

    fmt.Print("Can access sync dir / repository (", syncDir ,")? ")
    info, err := os.Stat(syncDir)

    if err != nil {
        fmt.Println("No. ", err)
        os.Exit(1)
    }

    if ! info.IsDir() {
        fmt.Println("No. Is not a directory")
        os.Exit(1)
    }

    fmt.Println("Yes.")
}

// Is the given directory a git repository?
func checkIsRepo(syncDir string, backend *GitBackend) {
    fmt.Print("Is a git repository? ")
    err := backend.git("status")
    if err != nil {
        fmt.Println("No. ", err)
        os.Exit(1)
    }
    fmt.Println("Yes.")
}

// Can we pull in git repo?
func checkPull(backend *GitBackend) {
    fmt.Print("Can 'git pull' in the repo? ")
    err := backend.pull()
    if err != nil {
        fmt.Println("No. ", err)
        os.Exit(1)
    }
    fmt.Println("Yes.")
}

// Can we call a bare push in git repo?
func checkPush(backend *GitBackend) {
    fmt.Print("Can 'git push' in the repo? ")
    err := backend.push()
    if err != nil {
        fmt.Println("No. ", err)
        os.Exit(1)
    }
    fmt.Println("Yes.")
}

// Check if a remote server is configured
func checkRemoteConfig(config *Config) {
    fmt.Print("Is configured for remote server? ")

    if config.isServer {
        fmt.Println("No. We _are_ the server.")
        return
    }
    if config.serverAddr == "" {
        fmt.Println("No. Server address missing from command line.")
        fmt.Println("Using a remote server is optional. You're fine.")
    } else {
        fmt.Println("Yes.")
    }
}

// Can we see the remote server?
func checkRemoteConnection(serverAddr string) {

    fmt.Print("Can connect to server (", serverAddr, ")? ")
    conn := getRemoteConnection(serverAddr, false)
    if conn == nil {
        fmt.Println("No.")
        os.Exit(1)
    }
    fmt.Println("Yes.")

    fmt.Print("Can write to connection data? ")
    err := tcpSend(conn, "Test\n")
    if err != nil {
        fmt.Println("No. ", err)
        os.Exit(1)
    }
    fmt.Println("Yes")
}
