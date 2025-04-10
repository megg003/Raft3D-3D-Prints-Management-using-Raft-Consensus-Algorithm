package raftnode

import (
	"bytes"
	"encoding/gob"
	"io"
	"sync"

	"github.com/hashicorp/raft"
)

// KVStore is a simple distributed key-value store
type KVStore struct {
	mu   sync.Mutex
	data map[string]string
}

// NewKVStore initializes a new KVStore
func NewKVStore() *KVStore {
	return &KVStore{
		data: make(map[string]string),
	}
}

// Apply applies a Raft log entry to the key-value store
func (kv *KVStore) Apply(log *raft.Log) interface{} {
	var cmd Command
	decoder := gob.NewDecoder(bytes.NewReader(log.Data))
	err := decoder.Decode(&cmd)
	if err != nil {
		return err
	}

	kv.mu.Lock()
	defer kv.mu.Unlock()
	kv.data[cmd.Key] = cmd.Value
	return nil
}

// Snapshot returns a snapshot of the key-value store
func (kv *KVStore) Snapshot() (raft.FSMSnapshot, error) {
	kv.mu.Lock()
	defer kv.mu.Unlock()

	clone := make(map[string]string)
	for k, v := range kv.data {
		clone[k] = v
	}

	return &kvSnapshot{store: clone}, nil
}

// Restore restores the key-value store to a previous state
func (kv *KVStore) Restore(snapshot io.ReadCloser) error {
	defer snapshot.Close()
	decoder := gob.NewDecoder(snapshot)
	return decoder.Decode(&kv.data)
}

// Command defines the structure of the data to replicate
type Command struct {
	Key   string
	Value string
}

// kvSnapshot is used for Raft snapshots
type kvSnapshot struct {
	store map[string]string
}

func (s *kvSnapshot) Persist(sink raft.SnapshotSink) error {
	data, err := gobEncode(s.store)
	if err != nil {
		sink.Cancel()
		return err
	}

	if _, err := sink.Write(data); err != nil {
		sink.Cancel()
		return err
	}
	return sink.Close()
}

func (s *kvSnapshot) Release() {}

func gobEncode(data interface{}) ([]byte, error) {
	var buf bytes.Buffer
	enc := gob.NewEncoder(&buf)
	err := enc.Encode(data)
	return buf.Bytes(), err
}
