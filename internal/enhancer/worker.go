package enhancer

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"mayo-cli/internal/ai"
	"mayo-cli/internal/config"
	"mayo-cli/internal/db"
)

type Worker struct {
	ID      string
	Config  *EnhancerConfig
	Status  *EnhancerStatus
	Manager *Manager
	AI      ai.AIClient
}

func NewWorker(id string) (*Worker, error) {
	m := NewManager()
	cfg, ok := m.Configs[id]
	if !ok {
		return nil, fmt.Errorf("config for %s not found", id)
	}
	status, ok := m.Enhancers[id]
	if !ok {
		return nil, fmt.Errorf("status for %s not found", id)
	}

	// Init AI
	appCfg, _ := config.LoadConfig()
	var aiClient ai.AIClient
	for _, p := range appCfg.AIProfiles {
		if p.Name == appCfg.ActiveAIProfile {
			key, _ := p.GetAPIKey(appCfg.UseKeyring)
			aiClient = ai.NewClient(p.Provider, key, p.DefaultModel)
			break
		}
	}
	if aiClient == nil && len(appCfg.AIProfiles) > 0 {
		p := appCfg.AIProfiles[0]
		key, _ := p.GetAPIKey(appCfg.UseKeyring)
		aiClient = ai.NewClient(p.Provider, key, p.DefaultModel)
	}

	if aiClient == nil {
		return nil, fmt.Errorf("AI client not configured")
	}

	return &Worker{
		ID:      id,
		Config:  cfg,
		Status:  status,
		Manager: m,
		AI:      aiClient,
	}, nil
}

func (w *Worker) Run(ctx context.Context) error {
	logFile, _ := os.OpenFile(GetLogPath(w.ID), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	defer logFile.Close()
	logger := log.New(logFile, fmt.Sprintf("[%s] ", w.ID), log.LstdFlags)

	logger.Printf("Worker started for task %s", w.ID)
	w.updateStatus(func(s *EnhancerStatus) {
		s.State = "running"
		s.StartTime = time.Now()
	})

	conn, err := sql.Open("sqlite3", w.Config.DBPath)
	if err != nil {
		w.handleError(logger, "Failed to open DB: "+err.Error())
		return err
	}
	defer conn.Close()

	// Ensure Target Column exists
	if err := db.EnsureColumn(ctx, conn, w.Config.Table, w.Config.TargetColumn, "TEXT"); err != nil {
		w.handleError(logger, "Failed to ensure column: "+err.Error())
		return err
	}

	// IF DATAFRAME: Update metadata in _frames table
	if strings.HasPrefix(w.Config.Table, "df_") {
		dfName := strings.TrimPrefix(w.Config.Table, "df_")
		logger.Printf("Updating metadata for dataframe: %s", dfName)

		var colsJSON string
		row := conn.QueryRowContext(ctx, "SELECT columns FROM _frames WHERE name = ?", dfName)
		if err := row.Scan(&colsJSON); err == nil {
			var columns []string
			json.Unmarshal([]byte(colsJSON), &columns)

			// Check if column already in metadata
			exists := false
			for _, c := range columns {
				if strings.EqualFold(c, w.Config.TargetColumn) {
					exists = true
					break
				}
			}

			if !exists {
				columns = append(columns, w.Config.TargetColumn)
				newJSON, _ := json.Marshal(columns)
				_, err := conn.ExecContext(ctx, "UPDATE _frames SET columns = ? WHERE name = ?", string(newJSON), dfName)
				if err != nil {
					logger.Printf("Warning: Failed to update metadata: %v", err)
				} else {
					logger.Printf("Metadata updated for %s. New column recorded.", dfName)
				}
			}
		}
	}

	for {
		select {
		case <-ctx.Done():
			logger.Println("Context cancelled, stopping worker.")
			w.updateStatus(func(s *EnhancerStatus) { s.State = "stopped" })
			return nil
		default:
			processed, err := w.processBatch(ctx, conn, logger)
			if err != nil {
				w.handleError(logger, "Batch error: "+err.Error())
				// Sleep for a bit before retrying
				time.Sleep(30 * time.Second)
				continue
			}

			if processed == 0 {
				if w.Config.Idle {
					w.updateStatus(func(s *EnhancerStatus) { s.State = "idle" })
					logger.Printf("No more data. Sleeping for %d seconds (Idle mode)...", w.Config.Polling)
					time.Sleep(time.Duration(w.Config.Polling) * time.Second)
					continue
				} else {
					w.updateStatus(func(s *EnhancerStatus) { s.State = "completed" })
					logger.Println("All data processed. Exiting.")
					return nil
				}
			}
		}
	}
}

func (w *Worker) processBatch(ctx context.Context, conn *sql.DB, logger *log.Logger) (int, error) {
	// 1. Fetch batch
	// We use rowid if no specific ID is found, but for better compatibility, let's try to find an ID or use rowid
	query := fmt.Sprintf("SELECT rowid, * FROM %s WHERE %s IS NULL LIMIT %d", w.Config.Table, w.Config.TargetColumn, w.Config.BatchSize)
	rows, err := conn.QueryContext(ctx, query)
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	cols, _ := rows.Columns()
	var batchData []map[string]interface{}
	var ids []int64

	for rows.Next() {
		vals := make([]interface{}, len(cols))
		ptr := make([]interface{}, len(cols))
		for i := range vals {
			ptr[i] = &vals[i]
		}
		if err := rows.Scan(ptr...); err != nil {
			return 0, err
		}

		record := make(map[string]interface{})
		var rowID int64
		for i, col := range cols {
			if col == "rowid" {
				rowID, _ = vals[i].(int64)
				continue
			}
			if col == w.Config.TargetColumn {
				continue // Skip the target column itself
			}
			record[col] = vals[i]
		}
		batchData = append(batchData, record)
		ids = append(ids, rowID)
	}

	if len(batchData) == 0 {
		return 0, nil
	}

	logger.Printf("Processing batch of %d rows...", len(batchData))

	// 2. AI Enrichment
	prompt := fmt.Sprintf(`You are an AI Data Enhancer. 
Instructions: %s
Input Data (JSON): 
%s

You must output a JSON array of strings corresponding to each row in the input data. 
Each string should be the result for the '%s' column.
IMPORTANT: Return ONLY the JSON array, no explanation.

Expected format: ["result1", "result2", ...]`, w.Config.Prompt, mustMarshal(batchData), w.Config.TargetColumn)

	aiResponse, _, err := w.AI.GenerateResponse(ctx, "You are a data processing assistant. Respond only with JSON.", prompt)
	if err != nil {
		return 0, err
	}

	// 3. Parse AI Response
	var results []string
	// Clean AI response from markdown blocks
	aiResponse = strings.TrimPrefix(aiResponse, "```json")
	aiResponse = strings.TrimPrefix(aiResponse, "```")
	aiResponse = strings.TrimSuffix(aiResponse, "```")
	aiResponse = strings.TrimSpace(aiResponse)

	if err := json.Unmarshal([]byte(aiResponse), &results); err != nil {
		logger.Printf("Failed to parse AI response: %v\nRaw Response: %s", err, aiResponse)
		return 0, fmt.Errorf("AI response parsing error: %v", err)
	}

	if len(results) != len(ids) {
		logger.Printf("Response count mismatch: got %d, want %d", len(results), len(ids))
		return 0, fmt.Errorf("AI response count mismatch")
	}

	// 4. Update Database
	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	stmt, err := tx.PrepareContext(ctx, fmt.Sprintf("UPDATE %s SET %s = ? WHERE rowid = ?", w.Config.Table, w.Config.TargetColumn))
	if err != nil {
		return 0, err
	}
	defer stmt.Close()

	for i, id := range ids {
		if _, err := stmt.ExecContext(ctx, results[i], id); err != nil {
			return 0, err
		}
	}

	if err := tx.Commit(); err != nil {
		return 0, err
	}

	w.updateStatus(func(s *EnhancerStatus) {
		s.ProcessedCount += len(ids)
		s.LastRun = time.Now()
	})

	logger.Printf("Batch of %d rows completed.", len(ids))
	return len(ids), nil
}

func (w *Worker) updateStatus(update func(*EnhancerStatus)) {
	w.Manager.UpdateStatus(w.ID, update)
	w.Manager.Save()
}

func (w *Worker) handleError(logger *log.Logger, errMsg string) {
	logger.Println("ERROR: " + errMsg)
	w.updateStatus(func(s *EnhancerStatus) {
		s.LastError = errMsg
		s.State = "error"
	})
}

func mustMarshal(v interface{}) string {
	b, _ := json.Marshal(v)
	return string(b)
}
