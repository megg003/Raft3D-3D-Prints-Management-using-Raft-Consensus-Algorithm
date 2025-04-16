package httpserver

import (
    "encoding/json"
    "fmt"
    "net/http"
    "sync"
    "bytes"
    "encoding/gob"
    "time"

    "github.com/gorilla/mux"
    "github.com/hashicorp/raft"
    "raft3d/internal/raftnode"
    "raft3d/internal/model"
)

// ====== STRUCTS ======



// ====== IN-MEMORY STORAGE ======

var (
    
    printerMux  sync.Mutex
    filamentMux sync.Mutex
    printJobMux sync.Mutex
    raftNode    *raft.Raft   // <<< ADD this global raft node
    kvStore     *raftnode.KVStore // <<< ADD this global variable
)

// ====== SERVER START ======

func Start(port int) {
    r := mux.NewRouter()

    r.HandleFunc("/health", healthHandler).Methods("GET")
    r.HandleFunc("/leave", leaveHandler).Methods("POST") // <<< New API to leave

    // Printers
    r.HandleFunc("/api/v1/printers", CreatePrinter).Methods("POST")
    r.HandleFunc("/api/v1/printers", ListPrinters).Methods("GET")

    // Filaments
    r.HandleFunc("/api/v1/filaments", CreateFilament).Methods("POST")
    r.HandleFunc("/api/v1/filaments", ListFilaments).Methods("GET")

    // Print Jobs
    r.HandleFunc("/api/v1/print_jobs", CreatePrintJob).Methods("POST")
    r.HandleFunc("/api/v1/print_jobs", ListPrintJobs).Methods("GET")
    r.HandleFunc("/api/v1/print_jobs/{id}/status", UpdatePrintJobStatus).Methods("POST")

    addr := fmt.Sprintf(":%d", port)
    fmt.Println("Starting HTTP server at", addr)
    if err := http.ListenAndServe(addr, r); err != nil {
        fmt.Println("Error starting HTTP server:", err)
    }
}

// ====== SET RAFT NODE ======

// SetRaftNode sets the Raft node for the HTTP server to use.
func SetRaftNode(r *raft.Raft) {
    raftNode = r
}

// SetKVStore sets the KVStore for the HTTP server to use.
func SetKVStore(store *raftnode.KVStore) {
    kvStore = store
}

// ====== HANDLERS ======

func healthHandler(w http.ResponseWriter, r *http.Request) {
    w.WriteHeader(http.StatusOK)
    w.Write([]byte("Server is running 🚀"))
}

// Leave Handler
func leaveHandler(w http.ResponseWriter, r *http.Request) {
    if raftNode == nil {
        w.WriteHeader(http.StatusInternalServerError)
        w.Write([]byte("Raft node not initialized"))
        return
    }
    err := raftNode.RemoveServer(raft.ServerID("01"), 0, 0).Error()
    if err != nil {
        w.WriteHeader(http.StatusInternalServerError)
        w.Write([]byte(fmt.Sprintf("Failed to leave cluster: %v", err)))
    } else {
        w.WriteHeader(http.StatusOK)
        w.Write([]byte("Node 01 left the cluster successfully"))
    }
}

// --- Printers ---

func CreatePrinter(w http.ResponseWriter, r *http.Request) {
    var p model.Printer
    if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
        http.Error(w, "Invalid payload", http.StatusBadRequest)
        return
    }
    // Submit to Raft
    cmd := raftnode.Command{Op: "create_printer", Payload: p}
    var buf bytes.Buffer
    gob.NewEncoder(&buf).Encode(cmd)
    future := raftNode.Apply(buf.Bytes(), 5*time.Second)
    if err := future.Error(); err != nil {
        http.Error(w, "Raft apply failed", http.StatusInternalServerError)
        return
    }
    w.WriteHeader(http.StatusCreated)
    json.NewEncoder(w).Encode(p)
}

func ListPrinters(w http.ResponseWriter, r *http.Request) {
    kvStore.Mu.Lock()
    defer kvStore.Mu.Unlock()
    var list []model.Printer
    for _, p := range kvStore.Printers {
        list = append(list, p)
    }
    w.WriteHeader(http.StatusOK)
    json.NewEncoder(w).Encode(list)
}

// --- Filaments ---

func CreateFilament(w http.ResponseWriter, r *http.Request) {
    var f model.Filament
    if err := json.NewDecoder(r.Body).Decode(&f); err != nil {
        http.Error(w, "Invalid payload", http.StatusBadRequest)
        return
    }
    f.RemainingWeightInGrams = f.TotalWeightInGrams
    cmd := raftnode.Command{Op: "create_filament", Payload: f}
    var buf bytes.Buffer
    gob.NewEncoder(&buf).Encode(cmd)
    future := raftNode.Apply(buf.Bytes(), 5*time.Second)
    if err := future.Error(); err != nil {
        http.Error(w, "Raft apply failed", http.StatusInternalServerError)
        return
    }
    w.WriteHeader(http.StatusCreated)
    json.NewEncoder(w).Encode(f)
}

func ListFilaments(w http.ResponseWriter, r *http.Request) {
    kvStore.Mu.Lock()
    defer kvStore.Mu.Unlock()
    var list []model.Filament
    for _, f := range kvStore.Filaments {
        list = append(list, f)
    }
    w.WriteHeader(http.StatusOK)
    json.NewEncoder(w).Encode(list)
}

// --- Print Jobs ---

func CreatePrintJob(w http.ResponseWriter, r *http.Request) {
    if raftNode.State() != raft.Leader {
        http.Error(w, "This node is not the leader. Please send requests to the leader.", http.StatusServiceUnavailable)
        return
    }
    var job model.PrintJob
    if err := json.NewDecoder(r.Body).Decode(&job); err != nil {
        http.Error(w, "Invalid payload", http.StatusBadRequest)
        return
    }
    kvStore.Mu.Lock()
    defer kvStore.Mu.Unlock()
    if _, exists := kvStore.Printers[job.PrinterID]; !exists {
        http.Error(w, "Printer not found", http.StatusBadRequest)
        return
    }
    filament, exists := kvStore.Filaments[job.FilamentID]
    if !exists {
        http.Error(w, "Filament not found", http.StatusBadRequest)
        return
    }
    if job.PrintWeightInGrams > filament.RemainingWeightInGrams {
        http.Error(w, "Not enough filament remaining", http.StatusBadRequest)
        return
    }
    job.Status = "Queued"
    cmd := raftnode.Command{Op: "create_print_job", Payload: job}
    var buf bytes.Buffer
    gob.NewEncoder(&buf).Encode(cmd)
    future := raftNode.Apply(buf.Bytes(), 5*time.Second)
    if err := future.Error(); err != nil {
        http.Error(w, "Raft apply failed", http.StatusInternalServerError)
        return
    }
    w.WriteHeader(http.StatusCreated)
    json.NewEncoder(w).Encode(job)
}

func ListPrintJobs(w http.ResponseWriter, r *http.Request) {
    kvStore.Mu.Lock()
    defer kvStore.Mu.Unlock()
    var list []model.PrintJob
    for _, j := range kvStore.PrintJobs {
        list = append(list, j)
    }
    w.WriteHeader(http.StatusOK)
    json.NewEncoder(w).Encode(list)
}

func UpdatePrintJobStatus(w http.ResponseWriter, r *http.Request) {
    vars := mux.Vars(r)
    id := vars["id"]
    newStatus := r.URL.Query().Get("status")

    kvStore.Mu.Lock()
    job, exists := kvStore.PrintJobs[id]
    if (!exists) {
        kvStore.Mu.Unlock()
        http.Error(w, "Print job not found", http.StatusNotFound)
        return
    }
    valid := false
    switch job.Status {
    case "Queued":
        if newStatus == "running" || newStatus == "cancelled" {
            valid = true
        }
    case "Running":
        if newStatus == "done" || newStatus == "cancelled" {
            valid = true
        }
    }
    if !valid {
        kvStore.Mu.Unlock()
        http.Error(w, "Invalid status transition", http.StatusBadRequest)
        return
    }
    kvStore.Mu.Unlock()

    update := model.StatusUpdate{JobID: id, NewStatus: newStatus}
    cmd := raftnode.Command{Op: "update_print_job_status", Payload: update}
    var buf bytes.Buffer
    gob.NewEncoder(&buf).Encode(cmd)
    future := raftNode.Apply(buf.Bytes(), 5*time.Second)
    if err := future.Error(); err != nil {
        http.Error(w, "Raft apply failed", http.StatusInternalServerError)
        return
    }
    w.WriteHeader(http.StatusOK)
    json.NewEncoder(w).Encode(update)
}
