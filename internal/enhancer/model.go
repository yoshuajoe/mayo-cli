package enhancer

import (
	"time"
)

type EnhancerConfig struct {
	ID           string `json:"id"`
	DBPath       string `json:"db_path"`
	Table        string `json:"table"`
	TargetColumn string `json:"target_column"`
	Prompt       string `json:"prompt"`
	BatchSize    int    `json:"batch_size"`
	Idle         bool   `json:"idle"`
	Polling      int    `json:"polling"` // in seconds
}

type EnhancerStatus struct {
	ID           string    `json:"id"`
	State        string    `json:"state"` // running, stopped, idle, completed, error
	Progress     int       `json:"progress"`
	Total        int       `json:"total"`
	LastRun      time.Time `json:"last_run"`
	StartTime    time.Time `json:"start_time"`
	LastError    string    `json:"last_error,omitempty"`
	ProcessedCount int     `json:"processed_count"`
}
