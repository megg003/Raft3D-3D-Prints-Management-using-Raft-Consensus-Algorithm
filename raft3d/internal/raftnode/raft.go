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

    // Create the Raft node
    r, err := raft.NewRaft(raftConfig, fsm, boltDB, boltDB, snapshots, transport)
    if err != nil {
        return nil, fmt.Errorf("failed to create Raft node: %v", err)
    }

    // Bootstrap cluster ONLY for Node 01
    if config.NodeID == "01" {
        configuration := raft.Configuration{
            Servers: []raft.Server{
                {
                    ID:      raft.ServerID(config.NodeID),
                    Address: raft.ServerAddress(config.BindAddress),
                },
            },
        }

        // Bootstrap the first cluster with Node 01
        r.BootstrapCluster(configuration)

        // Start a goroutine to add peers
        go func() {
            for {
                if r.State() == raft.Leader {
                    fmt.Println("Node 01 is now Leader! Adding peers...")

                    peers := []raft.Server{
                        {ID: "02", Address: "127.0.0.1:1202"},
                        {ID: "03", Address: "127.0.0.1:1203"},
                    }

                    for _, peer := range peers {
                        fmt.Printf("Trying to add peer %s...\n", peer.ID)
                        future := r.AddVoter(peer.ID, peer.Address, 0, 0)
                        if err := future.Error(); err != nil {
                            fmt.Printf("Error adding peer %s: %v\n", peer.ID, err)
                        } else {
                            fmt.Printf("Successfully added peer %s\n", peer.ID)
                        }
                    }

                    // After adding 02 and 03, also cross-connect them
if r.State() == raft.Leader {
    fmt.Println("Node 01 is connecting Node 02 and Node 03...")

    // Connect 02 → 03
    future := r.AddVoter("03", "127.0.0.1:1203", 0, 0)
    if err := future.Error(); err != nil {
        fmt.Println("Error adding 03 to 02:", err)
    } else {
        fmt.Println("Connected 02 -> 03")
    }

    // Connect 03 → 02
    future = r.AddVoter("02", "127.0.0.1:1202", 0, 0)
    if err := future.Error(); err != nil {
        fmt.Println("Error adding 02 to 03:", err)
    } else {
        fmt.Println("Connected 03 -> 02")
    }
}

                    return
                }
                time.Sleep(1 * time.Second)
            }
        }()
    }

    return r, nil
}
