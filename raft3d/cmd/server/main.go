package main

import (
    "encoding/gob"
    "fmt"
    "os"
    "os/signal"
    "strconv"
    "syscall"

    "raft3d/internal/httpserver"
    "raft3d/internal/raftnode"
    "raft3d/internal/model"
)

func main() {
    gob.Register(model.Printer{})
    fmt.Println("Registered model.Printer with gob")
    gob.Register(model.Filament{})
    fmt.Println("Registered model.Filament with gob")
    gob.Register(model.PrintJob{})
    fmt.Println("Registered model.PrintJob with gob")
    gob.Register(model.StatusUpdate{})
    fmt.Println("Registered model.StatusUpdate with gob")
    gob.Register(raftnode.Command{})

    // --- Parse Command-line arguments ---

    if len(os.Args) != 3 {
        fmt.Println("Usage: go run main.go <node_id> <http_port>")
        os.Exit(1)
    }

    nodeID := os.Args[1]             // Node ID like "01", "02", "03"
    httpPort, err := strconv.Atoi(os.Args[2]) // HTTP port like 8080, 8081, 8082
    if err != nil {
        fmt.Println("Invalid HTTP port:", err)
        os.Exit(1)
    }

    // --- Configuration for this node ---

    raftConfig := raftnode.Config{
        NodeID:      nodeID,
        DataDir:     "raft-data-" + nodeID,    // Each node has its own raft data
        BindAddress: "127.0.0.1:12" + nodeID,  // Transport ports like 12001, 12002, etc.
    }

    // --- Initialize Raft Node ---
    raftNode, kvStore, err := raftnode.NewRaftNode(raftConfig)
    if err != nil {
        fmt.Println("Failed to initialize Raft node:", err)
        return
    }

    // --- ✨ SET RaftNode in HTTP server ---
    httpserver.SetRaftNode(raftNode)
    httpserver.SetKVStore(kvStore)

    // --- Start HTTP Server in a separate goroutine ---
    go httpserver.Start(httpPort)

    // --- Graceful shutdown on Ctrl+C ---
    sigCh := make(chan os.Signal, 1)
    signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
    <-sigCh

    fmt.Println("Shutting down...")
    raftNode.Shutdown()
}

