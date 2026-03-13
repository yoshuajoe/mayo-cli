package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"mayo-cli/internal/ai"
	"mayo-cli/internal/config"
	"mayo-cli/internal/db"
	_ "modernc.org/sqlite"
	"mayo-cli/internal/files"
	"mayo-cli/internal/session"
	"mayo-cli/internal/ui"

	"github.com/spf13/cobra"
)

func HandleConnect(driver, dsn, alias string, source string) {
	cfg, err := config.LoadConfig()
	if err != nil || cfg == nil || len(cfg.AIProfiles) == 0 {
		ui.PrintInfo("No AI Profiles found. Please run '/setup' first.")
		return
	}

	// Sanitize DSN: Trim spaces and quotes if user pasted them
	dsn = strings.TrimSpace(dsn)
	dsn = strings.Trim(dsn, "\"")
	dsn = strings.Trim(dsn, "'")
	dsn = strings.TrimSpace(dsn)

	ensureOrchestrator(cfg)

	if alias == "" {
		alias = fmt.Sprintf("ds_%d", len(GlobalOrchestrator.Connections)+1)
	}

	// Check if alias already exists
	if _, exists := GlobalOrchestrator.Connections[alias]; exists {
		ui.PrintInfo(fmt.Sprintf("Alias '%s' already exists. Overwriting...", alias))
	}

	// Check if it's a file or directory connection
	if driver == "file" || driver == "csv" || driver == "xlsx" || driver == "folder" {
		var filesList []*files.FileData
		var err error

		info, err := os.Stat(dsn)
		if err == nil && info.IsDir() {
			filesList, err = files.ScanDirectory(dsn)
			ui.PrintSuccess(fmt.Sprintf("[%s] Connected to folder: %s (%d files found)", alias, dsn, len(filesList)))
		} else {
			ui.RenderStep("📄", fmt.Sprintf("[%s] Parsing file data...", alias))
			var fData *files.FileData
			if driver == "xlsx" || strings.HasSuffix(dsn, ".xlsx") {
				fData, err = files.ParseXLSX(dsn)
			} else {
				fData, err = files.ParseCSV(dsn)
			}
			if err == nil {
				filesList = append(filesList, fData)
			}
			ui.PrintSuccess(fmt.Sprintf("[%s] Connected to file: %s", alias, dsn))
		}

		if err != nil {
			ui.PrintError(fmt.Sprintf("Failed to parse data: %v", err))
			return
		}

		// --- Convert to SQLite ---
		ui.RenderStep("🗄️", fmt.Sprintf("[%s] Converting file data to SQLite...", alias))
		sqlitePath := filepath.Join(config.GetConfigDir(), "data", alias+".db")
		
		ctxDB, cancelDB := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancelDB()

		sqliteDB, err := db.ConnectWithContext(ctxDB, "sqlite", sqlitePath)
		if err != nil {
			ui.PrintError(fmt.Sprintf("Failed to initialize SQLite: %v", err))
			return
		}

		for _, f := range filesList {
			if f.Type == "csv" || f.Type == "xlsx" {
				tbl, err := db.ImportFileDataToSQLite(sqliteDB, f)
				if err == nil {
					ui.RenderStep("📦", fmt.Sprintf("[%s] Imported '%s' into table '%s'", alias, filepath.Base(f.Name), tbl))
				}
			}
		}

		schema, _ := db.ScanSchema(ctxDB, sqliteDB, "sqlite")
		
		// Use provided source name or fallback to dsn
		finalSource := source
		if finalSource == "" {
			finalSource = dsn
		}

		GlobalOrchestrator.Connections[alias] = &ai.DBConnection{
			Alias:    alias,
			Source:   finalSource,
			DB:       sqliteDB,
			Driver:   "sqlite",
			Schema:   schema,
			IsImport: true,
		}
		GlobalOrchestrator.Files = append(GlobalOrchestrator.Files, filesList...)

	} else {
		ctxDB, cancelDB := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancelDB()

		database, err := db.ConnectWithContext(ctxDB, driver, dsn)
		if err != nil {
			ui.PrintError(fmt.Sprintf("Connection failed: %v", err))
			return
		}
		ui.PrintSuccess(fmt.Sprintf("[%s] Connected to database (%s).", alias, driver))

		ui.RenderStep("🕵️", fmt.Sprintf("[%s] Scanning schema...", alias))
		schema, err := db.ScanSchema(ctxDB, database, driver)
		if err != nil {
			ui.PrintError(fmt.Sprintf("Scan failed: %v", err))
			return
		}

		// Use provided source name or fallback to dsn
		finalSource := source
		if finalSource == "" {
			finalSource = dsn
		}

		GlobalOrchestrator.Connections[alias] = &ai.DBConnection{
			Alias:  alias,
			Source: finalSource,
			DB:     database,
			Driver: driver,
			Schema: schema,
		}
	}

	// Update profiles list but DON'T auto-set as ActiveDSProfile 
	// unless it's explicitly done via /profile ds
	newProfile := config.DSProfile{
		Name:   fmt.Sprintf("DS_%s_%s", driver, alias),
		Driver: driver,
		DSN:    dsn,
	}
	
	exists := false
	for _, p := range cfg.DSProfiles {
		if p.DSN == dsn && p.Driver == driver {
			exists = true
			break
		}
	}
	if !exists {
		cfg.DSProfiles = append(cfg.DSProfiles, newProfile)
		config.SaveConfig(cfg)
	}
}


func ensureOrchestrator(cfg *config.Config) {
	if GlobalOrchestrator == nil {
		if GlobalSess == nil {
			GlobalSess, _ = session.NewSession()
		}
		GlobalOrchestrator = &ai.Orchestrator{
			AI:          GlobalAI,
			Connections: make(map[string]*ai.DBConnection),
			Session:     GlobalSess,
			UserContext: cfg.UserContext,
		}
	}
}

var connectCmd = &cobra.Command{
	Use:   "connect [driver] [dsn] [alias]",
	Short: "Connect to a data source with an optional alias",
	Args:  cobra.MinimumNArgs(2),
	Run: func(cmd *cobra.Command, args []string) {
		alias := ""
		if len(args) > 2 {
			alias = args[2]
		}
		HandleConnect(args[0], args[1], alias, "")
	},
}

func init() {
	rootCmd.AddCommand(connectCmd)
}
