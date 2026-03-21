package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"mayo-cli/internal/config"
	"mayo-cli/internal/enhancer"
	"mayo-cli/internal/ui"

	"github.com/AlecAivazis/survey/v2"
	"github.com/google/uuid"
	"github.com/spf13/cobra"
)

var enhanceCmd = &cobra.Command{
	Use:   "enhance",
	Short: "Manage AI Data Enhancer tasks",
}

var startEnhanceCmd = &cobra.Command{
	Use:   "start",
	Short: "Start a new AI Data Enhancer task",
	Run: func(cmd *cobra.Command, args []string) {
		HandleEnhanceStart(cmd, args)
	},
}

func HandleEnhanceStart(cmd *cobra.Command, args []string) {
	table, _ := cmd.Flags().GetString("table")
	column, _ := cmd.Flags().GetString("column")
	prompt, _ := cmd.Flags().GetString("prompt")
	batchSize, _ := cmd.Flags().GetInt("batch")
	idle, _ := cmd.Flags().GetBool("idle")
	polling, _ := cmd.Flags().GetInt("polling")
	dbPath, _ := cmd.Flags().GetString("db")

	// --- VALIDATION: DF MODE ONLY ---
	if GlobalOrchestrator == nil || GlobalOrchestrator.StagedName == "" {
		ui.PrintError("AI Data Enhancer is only available in 'Dataframe Mode'.")
		ui.PrintInfo("Please load a dataframe first with '/df load [name]'.")
		return
	}

	// Interactive DB selection
	if dbPath == "" && GlobalOrchestrator != nil && GlobalOrchestrator.StagedName != "" {
		// Use dataframes.db path
		dbPath = filepath.Join(config.GetConfigDir(), "dataframes.db")
	}

	if dbPath == "" && GlobalOrchestrator != nil && len(GlobalOrchestrator.Connections) > 0 {
		var sqliteConns []string
		for alias, conn := range GlobalOrchestrator.Connections {
			if conn.Driver == "sqlite" {
				sqliteConns = append(sqliteConns, alias)
			}
		}

		if len(sqliteConns) == 1 {
			dbPath = GlobalOrchestrator.Connections[sqliteConns[0]].DSN
		} else if len(sqliteConns) > 1 {
			var selectedAlias string
			survey.AskOne(&survey.Select{
				Message: "Select SQLite Connection:",
				Options: sqliteConns,
			}, &selectedAlias)
			if selectedAlias != "" {
				dbPath = GlobalOrchestrator.Connections[selectedAlias].DSN
			}
		}
	}

	if dbPath == "" {
		ui.PrintError("No SQLite database found. Connect first or specify --db.")
		return
	}

	// Interactive Table selection
	if table == "" && GlobalOrchestrator != nil {
		if GlobalOrchestrator.StagedName != "" {
			table = "df_" + GlobalOrchestrator.StagedName // Dataframe tables are prefixed with df_
		} else {
			var tables []string
			// Find connection by DSN to get its schema
			for _, conn := range GlobalOrchestrator.Connections {
				if conn.DSN == dbPath && conn.Schema != nil {
					for _, t := range conn.Schema.Tables {
						tables = append(tables, t.Name)
					}
					break
				}
			}

			if len(tables) > 0 {
				survey.AskOne(&survey.Select{
					Message: "Select Target Table:",
					Options: tables,
				}, &table)
			} else {
				survey.AskOne(&survey.Input{Message: "Target Table Name:"}, &table)
			}
		}
	}

	if table == "" {
		ui.PrintError("Table name is required.")
		return
	}

	// Interactive Column input
	if column == "" {
		survey.AskOne(&survey.Input{
			Message: "New/Target Column Name (e.g., sentiment):",
		}, &column)
	}
	if column == "" {
		ui.PrintError("Column name is required.")
		return
	}

	// Interactive Prompt input
	if prompt == "" {
		survey.AskOne(&survey.Multiline{
			Message: "AI Enhancement Prompt (Instructions):",
			Default: "Analyze the row content and provide a summary.",
		}, &prompt)
	}
	if prompt == "" {
		ui.PrintError("Prompt is required.")
		return
	}

	// Interactive Settings
	if cmd != nil && !cmd.Flags().Changed("batch") {
		var change bool
		survey.AskOne(&survey.Confirm{Message: "Change default batch size (10)?", Default: false}, &change)
		if change {
			var batchStr string
			survey.AskOne(&survey.Input{Message: "Batch Size:", Default: "10"}, &batchStr)
			fmt.Sscanf(batchStr, "%d", &batchSize)
		}
	}

	if cmd != nil && !cmd.Flags().Changed("idle") {
		survey.AskOne(&survey.Confirm{Message: "Enable Idle Mode (Wait for new data)?", Default: false}, &idle)
	}

	if idle && cmd != nil && !cmd.Flags().Changed("polling") {
		var pollStr string
		survey.AskOne(&survey.Input{Message: "Polling Interval (seconds):", Default: "60"}, &pollStr)
		fmt.Sscanf(pollStr, "%d", &polling)
	}

	absDBPath, _ := filepath.Abs(dbPath)

	id := uuid.New().String()[:8]
	cfg := &enhancer.EnhancerConfig{
		ID:           id,
		DBPath:       absDBPath,
		Table:        table,
		TargetColumn: column,
		Prompt:       prompt,
		BatchSize:    batchSize,
		Idle:         idle,
		Polling:      polling,
	}

	status := &enhancer.EnhancerStatus{
		ID:        id,
		State:     "starting",
		StartTime: time.Now(),
	}

	manager := enhancer.NewManager()
	manager.Register(cfg, status)
	manager.Save()

	// --- BACKGROUND SPAWN ---
	exe, _ := os.Executable()
	if strings.Contains(exe, "go-build") || strings.Contains(exe, "/tmp/") {
		ui.PrintWarn("Background worker might die after CLI exit because you are using 'go run'.")
		ui.PrintInfo("Recommendation: Build the binary first (go build -o mayo) and run it manually.")
	}

	workerCmd := exec.Command(exe, "enhance-worker", id)

	// Redirect output to log file
	logFile, _ := os.OpenFile(enhancer.GetLogPath(id), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	workerCmd.Stdout = logFile
	workerCmd.Stderr = logFile

	// Detach from parent process (Unix/macOS)
	if workerCmd.SysProcAttr == nil {
		workerCmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	workerCmd.SysProcAttr.Setsid = true
	workerCmd.Stdin = nil // Ensure no terminal input

	err := workerCmd.Start()
	if err != nil {
		ui.PrintError("Failed to start background worker: " + err.Error())
		logFile.Close()
		return
	}
	logFile.Close() // Parent doesn't need to keep it open

	ui.PrintSuccess(fmt.Sprintf("Enhancer task '%s' started in background.", id))
	ui.PrintInfo(fmt.Sprintf("Monitoring logs: mayo enhance logs %s", id))
}

var listEnhanceCmd = &cobra.Command{
	Use:   "list",
	Short: "List all enhancer tasks",
	Run: func(cmd *cobra.Command, args []string) {
		showAll, _ := cmd.Flags().GetBool("all")
		HandleEnhanceList(showAll)
	},
}

func HandleEnhanceList(showAll bool) {
	m := enhancer.NewManager()
	if len(m.Enhancers) == 0 {
		ui.PrintInfo("No enhancer tasks found.")
		return
	}

	headers := []string{"ID", "Table", "Column", "Status", "Processed"}
	var rows [][]string
	for id, s := range m.Enhancers {
		if s.State == "completed" && !showAll {
			continue
		}
		cfg := m.Configs[id]
		rows = append(rows, []string{
			id,
			cfg.Table,
			cfg.TargetColumn,
			s.State,
			fmt.Sprintf("%d", s.ProcessedCount),
		})
	}
	ui.RenderTable(headers, rows)
}

var stopEnhanceCmd = &cobra.Command{
	Use:   "stop [id]",
	Short: "Stop an enhancer task",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		HandleEnhanceStop(args)
	},
}

func HandleEnhanceStop(args []string) {
	if len(args) == 0 {
		ui.PrintError("Task ID required.")
		return
	}
	id := args[0]
	m := enhancer.NewManager()
	if s, ok := m.Enhancers[id]; ok {
		s.State = "stopping"
		m.Save()
		ui.PrintSuccess(fmt.Sprintf("Task %s marked for stopping. It will exit after the current batch.", id))
	} else {
		ui.PrintError("Task not found.")
	}
}

var logsEnhanceCmd = &cobra.Command{
	Use:   "logs [id]",
	Short: "View enhancer logs",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		HandleEnhanceLogs(args)
	},
}

func HandleEnhanceLogs(args []string) {
	if len(args) == 0 {
		ui.PrintError("Task ID required.")
		return
	}
	id := args[0]
	path := enhancer.GetLogPath(id)
	content, err := os.ReadFile(path)
	if err != nil {
		ui.PrintError("Failed to read logs: " + err.Error())
		return
	}
	fmt.Println(string(content))
}

var statusEnhanceCmd = &cobra.Command{
	Use:   "status [id]",
	Short: "View detailed status of an enhancer task",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		HandleEnhanceStatus(args)
	},
}

func HandleEnhanceStatus(args []string) {
	if len(args) == 0 {
		ui.PrintError("Task ID required.")
		return
	}
	id := args[0]
	m := enhancer.NewManager()
	s, ok := m.Enhancers[id]
	if !ok {
		ui.PrintError("Task not found.")
		return
	}
	cfg := m.Configs[id]

	ui.RenderMarkdown(fmt.Sprintf(`
# 🚀 Enhancer Task: %s
- **Status**: %s
- **Table**: %s
- **Target Column**: %s
- **Processed**: %d rows
- **Prompt**: %s
- **Started**: %s
`, id, s.State, cfg.Table, cfg.TargetColumn, s.ProcessedCount, cfg.Prompt, s.StartTime.Format(time.RFC822)))
}

func init() {
	startEnhanceCmd.Flags().StringP("table", "t", "", "Target table name")
	startEnhanceCmd.Flags().StringP("column", "c", "", "Target column name to populate")
	startEnhanceCmd.Flags().StringP("prompt", "p", "", "AI prompt for data enhancement")
	startEnhanceCmd.Flags().IntP("batch", "b", 10, "Batch size")
	startEnhanceCmd.Flags().BoolP("idle", "i", false, "Keep running and wait for new data")
	startEnhanceCmd.Flags().Int("polling", 60, "Interval to check for new data (seconds) in idle mode")
	startEnhanceCmd.Flags().String("db", "", "Path to SQLite database (optional)")

	listEnhanceCmd.Flags().BoolP("all", "a", false, "Show all tasks including completed ones")
	enhanceCmd.AddCommand(startEnhanceCmd, listEnhanceCmd, statusEnhanceCmd, stopEnhanceCmd, logsEnhanceCmd)
	rootCmd.AddCommand(enhanceCmd)
}
