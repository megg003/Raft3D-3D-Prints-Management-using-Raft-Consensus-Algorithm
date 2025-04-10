package raftnode

import (
    "fmt"
    "os"
    "path/filepath"
    "time"

    "github.com/hashicorp/raft"
    raftboltdb "github.com/hashicorp/raft-boltdb"
)

// Config holds the configuration for the Raft node.
type Config struct {
    NodeID      string
    DataDir     string
    BindAddress string
}

// NewRaftNode initializes and returns a Raft node with the given configuration.
func NewRaftNode(config Config) (*raft.Raft, error) {
    // Ensure the data directory exists
    if err := os.MkdirAll(config.DataDir, 0755); err != nil {
        return nil, fmt.Errorf("failed to create data directory: %v", err)
    }

    // Create a new Raft configuration with default settings
    raftConfig := raft.DefaultConfig()
    raftConfig.LocalID = raft.ServerID(config.NodeID)

    // Set up the log store and stable store using BoltDB
    boltDB, err := raftboltdb.NewBoltStore(filepath.Join(config.DataDir, "raft.db"))
    if err != nil {
        return nil, fmt.Errorf("failed to create BoltDB store: %v", err)
    }

    // Set up the snapshot store
    snapshots, err := raft.NewFileSnapshotStore(config.DataDir, 2, os.Stderr)
    if err != nil {
        return nil, fmt.Errorf("failed to create snapshot store: %v", err)
    }

    // Set up the transport layer
    transport, err := raft.NewTCPTransport(config.BindAddress, nil, 3, 10*time.Second, os.Stderr)
    if err != nil {
        return nil, fmt.Errorf("failed to create transport: %v", err)
    }

    // Create FSM (your Key-Value store)
    fsm := NewKVStore()

    // Initialize the Raft system using the FSM
    r, err := raft.NewRaft(raftConfig, fsm, boltDB, boltDB, snapshots, transport)
    if err != nil {
        return nil, fmt.Errorf("failed to create Raft node: %v", err)
    }

    return r, nil
}
