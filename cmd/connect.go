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
	"mayo-cli/internal/files"
	"mayo-cli/internal/session"
	"mayo-cli/internal/ui"
	_ "modernc.org/sqlite"

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

	// 1. Check if profile already exists for this DSN/Driver to avoid duplicates
	exists := false
	finalProfileName := source

	for _, p := range cfg.DSProfiles {
		if p.DSN == dsn && p.Driver == driver {
			exists = true
			if finalProfileName == "" {
				finalProfileName = p.Name
			}
			// Recover alias from existing profile name if possible
			// Pattern: DS_driver_alias
			parts := strings.Split(p.Name, "_")
			if len(parts) >= 3 && alias == "" {
				alias = parts[2]
			}
			break
		}
	}

	if alias == "" {
		alias = fmt.Sprintf("ds_%d", len(GlobalOrchestrator.Connections)+1)
	}

	// 2. Fallback profile naming
	if finalProfileName == "" {
		finalProfileName = fmt.Sprintf("DS_%s_%s", driver, alias)
	}

	// Check if alias already exists in active connections
	if _, active := GlobalOrchestrator.Connections[alias]; active {
		ui.PrintInfo(fmt.Sprintf("Alias '%s' already exists. Overwriting...", alias))
	}

	// 3. Connection and Data Processing logic
	if driver == "file" || driver == "csv" || driver == "xlsx" || driver == "folder" {
		// ... (file connection logic)
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

		GlobalOrchestrator.Connections[alias] = &ai.DBConnection{
			Alias:    alias,
			Source:   finalProfileName,
			DSN:      sqlitePath,
			DB:       sqliteDB,
			Driver:   "sqlite",
			IsImport: true,
		}
		GlobalOrchestrator.SyncSchema(ctxDB)
		GlobalOrchestrator.Files = append(GlobalOrchestrator.Files, filesList...)

	} else {
		// ... (database connection logic)
		ctxDB, cancelDB := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancelDB()

		database, err := db.ConnectWithContext(ctxDB, driver, dsn)
		if err != nil {
			ui.PrintError(fmt.Sprintf("Connection failed: %v", err))
			return
		}
		ui.PrintSuccess(fmt.Sprintf("[%s] Connected to database (%s).", alias, driver))

		GlobalOrchestrator.Connections[alias] = &ai.DBConnection{
			Alias:  alias,
			Source: finalProfileName,
			DSN:    dsn,
			DB:     database,
			Driver: driver,
		}
		ui.RenderStep("✨", fmt.Sprintf("[%s] Enriching metadata...", alias))
		GlobalOrchestrator.SyncSchema(ctxDB)
	}

	// 4. Update profile in config if new
	if !exists {
		newProfile := config.DSProfile{
			Name:   finalProfileName,
			Driver: driver,
			DSN:    dsn,
		}
		cfg.DSProfiles = append(cfg.DSProfiles, newProfile)
		config.SaveConfig(cfg)
	}

	// 5. Persist to session
	if GlobalSess != nil {
		found := false
		for _, p := range GlobalSess.ConnectedProfiles {
			if p == finalProfileName {
				found = true
				break
			}
		}
		if !found {
			GlobalSess.ConnectedProfiles = append(GlobalSess.ConnectedProfiles, finalProfileName)
			session.SaveSessionMetadata(GlobalSess)
		}
	}
}

func ensureOrchestrator(cfg *config.Config) {
	if GlobalOrchestrator == nil {
		if GlobalSess == nil {
			GlobalSess, _ = session.NewSession()
		}
		GlobalOrchestrator = &ai.Orchestrator{
			AI:             GlobalAI,
			Connections:    make(map[string]*ai.DBConnection),
			Session:        GlobalSess,
			UserContext:    cfg.UserContext,
			DefaultLimit:   cfg.DefaultLimit,
			Interactive:    cfg.Interactive,
			AnalystEnabled: cfg.AnalystEnabled,
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
