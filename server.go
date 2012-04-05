package main

import (
    "log"
    "net"
    "bufio"
    "io"
)

/* Create and start a server */
func startServer(addr string) {

    server := Server{addr: addr}
    log.Println("Listening on", addr)
    server.listen()
}

type Server struct {
    connections []net.Conn
    addr string
}

// Listen for new connections
func (self *Server) listen() {

    listener, err := net.Listen("tcp", self.addr)
    if err != nil {
        log.Fatal("Error on listen: " + err.Error())
    }
    defer listener.Close()

    for {
        conn, err := listener.Accept()
        log.Println("New connection")
        if err != nil {
            log.Fatal("Error on accept: " + err.Error())
        }

        self.connections = append(self.connections, conn)

        go self.handle(conn)
    }
}

// Read from this connection, and echo to all others
func (self *Server) handle(conn net.Conn) {

    for {
        bufRead := bufio.NewReader(conn)
        content, err := bufRead.ReadString('\n')

        if err == io.EOF {
            conn.Close()
            // Delete ourself from self.connections and leave go-routine
            newConnections := make([]net.Conn, 0, len(self.connections)-1)
            for _, candidate := range self.connections {
                if candidate != conn {
                    newConnections = append(newConnections, candidate)
                }
            }
            self.connections = newConnections
            return
        }

        log.Println("Echoing: ", content)
        for _, outConn := range self.connections {
            if outConn != conn {
                outConn.Write([]byte(content))
            }
        }
    }

}
