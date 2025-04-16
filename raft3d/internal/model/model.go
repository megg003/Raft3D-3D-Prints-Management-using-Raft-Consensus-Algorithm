package model

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
    Status             string `json:"status"`
}

type StatusUpdate struct {
    JobID     string
    NewStatus string
}