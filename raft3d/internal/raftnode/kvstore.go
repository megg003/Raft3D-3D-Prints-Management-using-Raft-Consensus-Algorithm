package raftnode

import (
	"bytes"
	"encoding/gob"
	"fmt"
	"io"
	"sync"

	"github.com/hashicorp/raft"
	"raft3d/internal/model"
)

// KVStore is a simple distributed key-value store
type KVStore struct {
	Mu        sync.Mutex
	Printers  map[string]model.Printer
	Filaments map[string]model.Filament
	PrintJobs map[string]model.PrintJob
}

// NewKVStore initializes a new KVStore
func NewKVStore() *KVStore {
	return &KVStore{
		Printers:  make(map[string]model.Printer),
		Filaments: make(map[string]model.Filament),
		PrintJobs: make(map[string]model.PrintJob),
	}
}

// Apply applies a Raft log entry to the key-value store
func (kv *KVStore) Apply(log *raft.Log) interface{} {
	fmt.Println("FSM Apply called")
	var cmd Command
	decoder := gob.NewDecoder(bytes.NewReader(log.Data))
	err := decoder.Decode(&cmd)
	if (err != nil) {
		return err
	}

	kv.Mu.Lock()
	defer kv.Mu.Unlock()

	switch cmd.Op {
	case "create_printer":
		p := cmd.Payload.(model.Printer)
		kv.Printers[p.ID] = p
	case "create_filament":
		f := cmd.Payload.(model.Filament)
		kv.Filaments[f.ID] = f
	case "create_print_job":
		j := cmd.Payload.(model.PrintJob)
		kv.PrintJobs[j.ID] = j
	case "update_print_job_status":
		update := cmd.Payload.(model.StatusUpdate)
		job, exists := kv.PrintJobs[update.JobID]
		if exists {
			job.Status = update.NewStatus
			if update.NewStatus == "done" {
				filament := kv.Filaments[job.FilamentID]
				filament.RemainingWeightInGrams -= job.PrintWeightInGrams
				kv.Filaments[job.FilamentID] = filament
			}
			kv.PrintJobs[job.ID] = job
		}
	}
	return nil
}

// Snapshot returns a snapshot of the key-value store
func (kv *KVStore) Snapshot() (raft.FSMSnapshot, error) {
	kv.Mu.Lock()
	defer kv.Mu.Unlock()
	clone := &KVStore{
		Printers:  make(map[string]model.Printer),
		Filaments: make(map[string]model.Filament),
		PrintJobs: make(map[string]model.PrintJob),
	}
	for k, v := range kv.Printers {
		clone.Printers[k] = v
	}
	for k, v := range kv.Filaments {
		clone.Filaments[k] = v
	}
	for k, v := range kv.PrintJobs {
		clone.PrintJobs[k] = v
	}
	return &kvSnapshot{store: clone}, nil
}

// Restore restores the key-value store to a previous state
func (kv *KVStore) Restore(snapshot io.ReadCloser) error {
	defer snapshot.Close()
	decoder := gob.NewDecoder(snapshot)
	return decoder.Decode(kv)
}

// Command defines the structure of the data to replicate
type Command struct {
	Op      string      // "create_printer", "create_filament", "create_print_job", "update_print_job_status"
	Payload interface{} // Will be one of Printer, Filament, PrintJob, or status update
}

// kvSnapshot is used for Raft snapshots
type kvSnapshot struct {
	store *KVStore
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
