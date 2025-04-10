package httpserver

import (
    "encoding/json"
    "fmt"
    "net/http"
    "sync"

    "github.com/gorilla/mux"
    "github.com/hashicorp/raft"
)

// ====== STRUCTS ======

type Printer struct {
    ID      string `json:"id"`
    Company string `json:"company"`
    Model   string `json:"model"`
}

type Filament struct {
    ID                     string `json:"id"`
    Type                   string `json:"type"`
    Color                  string `json:"color"`
    TotalWeightInGrams     int    `json:"total_weight_in_grams"`
    RemainingWeightInGrams int    `json:"remaining_weight_in_grams"`
}

type PrintJob struct {
    ID                 string `json:"id"`
    PrinterID          string `json:"printer_id"`
    FilamentID         string `json:"filament_id"`
    FilePath           string `json:"filepath"`
    PrintWeightInGrams int    `json:"print_weight_in_grams"`
    Status             string `json:"status"` // Queued, Running, Done, Cancelled
}

// ====== IN-MEMORY STORAGE ======

var (
    printers    = make(map[string]Printer)
    filaments   = make(map[string]Filament)
    printJobs   = make(map[string]PrintJob)
    printerMux  sync.Mutex
    filamentMux sync.Mutex
    printJobMux sync.Mutex
    raftNode    *raft.Raft   // <<< ADD this global raft node
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
    var p Printer
    if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
        http.Error(w, "Invalid payload", http.StatusBadRequest)
        return
    }
    printerMux.Lock()
    defer printerMux.Unlock()
    if _, exists := printers[p.ID]; exists {
        http.Error(w, "Printer already exists", http.StatusConflict)
        return
    }
    printers[p.ID] = p
    w.WriteHeader(http.StatusCreated)
    json.NewEncoder(w).Encode(p)
}

func ListPrinters(w http.ResponseWriter, r *http.Request) {
    printerMux.Lock()
    defer printerMux.Unlock()
    var list []Printer
    for _, p := range printers {
        list = append(list, p)
    }
    w.WriteHeader(http.StatusOK)
    json.NewEncoder(w).Encode(list)
}

// --- Filaments ---

func CreateFilament(w http.ResponseWriter, r *http.Request) {
    var f Filament
    if err := json.NewDecoder(r.Body).Decode(&f); err != nil {
        http.Error(w, "Invalid payload", http.StatusBadRequest)
        return
    }
    filamentMux.Lock()
    defer filamentMux.Unlock()
    if _, exists := filaments[f.ID]; exists {
        http.Error(w, "Filament already exists", http.StatusConflict)
        return
    }
    f.RemainingWeightInGrams = f.TotalWeightInGrams
    filaments[f.ID] = f
    w.WriteHeader(http.StatusCreated)
    json.NewEncoder(w).Encode(f)
}

func ListFilaments(w http.ResponseWriter, r *http.Request) {
    filamentMux.Lock()
    defer filamentMux.Unlock()
    var list []Filament
    for _, f := range filaments {
        list = append(list, f)
    }
    w.WriteHeader(http.StatusOK)
    json.NewEncoder(w).Encode(list)
}

// --- Print Jobs ---

func CreatePrintJob(w http.ResponseWriter, r *http.Request) {
    var job PrintJob
    if err := json.NewDecoder(r.Body).Decode(&job); err != nil {
        http.Error(w, "Invalid payload", http.StatusBadRequest)
        return
    }
    printerMux.Lock()
    if _, exists := printers[job.PrinterID]; !exists {
        printerMux.Unlock()
        http.Error(w, "Printer not found", http.StatusBadRequest)
        return
    }
    printerMux.Unlock()

    filamentMux.Lock()
    filament, exists := filaments[job.FilamentID]
    if !exists {
        filamentMux.Unlock()
        http.Error(w, "Filament not found", http.StatusBadRequest)
        return
    }
    if job.PrintWeightInGrams > filament.RemainingWeightInGrams {
        filamentMux.Unlock()
        http.Error(w, "Not enough filament remaining", http.StatusBadRequest)
        return
    }
    filamentMux.Unlock()

    printJobMux.Lock()
    defer printJobMux.Unlock()

    job.Status = "Queued"
    printJobs[job.ID] = job
    w.WriteHeader(http.StatusCreated)
    json.NewEncoder(w).Encode(job)
}

func ListPrintJobs(w http.ResponseWriter, r *http.Request) {
    printJobMux.Lock()
    defer printJobMux.Unlock()
    var list []PrintJob
    for _, j := range printJobs {
        list = append(list, j)
    }
    w.WriteHeader(http.StatusOK)
    json.NewEncoder(w).Encode(list)
}

func UpdatePrintJobStatus(w http.ResponseWriter, r *http.Request) {
    vars := mux.Vars(r)
    id := vars["id"]
    newStatus := r.URL.Query().Get("status")

    printJobMux.Lock()
    job, exists := printJobs[id]
    if !exists {
        printJobMux.Unlock()
        http.Error(w, "Print job not found", http.StatusNotFound)
        return
    }

    // Validate transitions
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
        printJobMux.Unlock()
        http.Error(w, "Invalid status transition", http.StatusBadRequest)
        return
    }

    // If done, deduct filament weight
    if newStatus == "done" {
        filamentMux.Lock()
        filament, exists := filaments[job.FilamentID]
        if exists {
            filament.RemainingWeightInGrams -= job.PrintWeightInGrams
            filaments[job.FilamentID] = filament
        }
        filamentMux.Unlock()
    }

    // Update job status
    job.Status = newStatus
    printJobs[id] = job
    printJobMux.Unlock()

    w.WriteHeader(http.StatusOK)
    json.NewEncoder(w).Encode(job)
}
