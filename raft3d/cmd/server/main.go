
package main

import (
    "fmt"
    "os"
    "os/signal"
    "syscall"

    "raft3d/internal/httpserver"
    "raft3d/internal/raftnode"
)

func main() {
    // Configuration for the Raft node
    raftConfig := raftnode.Config{
        NodeID:      "node1",
        DataDir:     "raft-data",
        BindAddress: "127.0.0.1:12000",
    }

    // Initialize the Raft node
    raftNode, err := raftnode.NewRaftNode(raftConfig)
    if err != nil {
        fmt.Println("Failed to initialize Raft node:", err)
        return
    }

    // Start the HTTP server in a separate goroutine
    go httpserver.Start(8080)

    // Handle termination signals to gracefully shut down
    sigCh := make(chan os.Signal, 1)
    signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
    <-sigCh

    // Graceful shutdown
    fmt.Println("Shutting down...")
    raftNode.Shutdown()
}
