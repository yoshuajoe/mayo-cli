package cmd

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"mayo-cli/internal/ai"
	"mayo-cli/internal/config"
	"mayo-cli/internal/dataframe"
	"mayo-cli/internal/db"
	"mayo-cli/internal/knowledge"
	"mayo-cli/internal/privacy"
	"mayo-cli/internal/session"
	"mayo-cli/internal/ui"

	"github.com/AlecAivazis/survey/v2"
	"github.com/chzyer/readline"
	"github.com/kballard/go-shellquote"
	"github.com/spf13/cobra"
)

var GlobalOrchestrator *ai.Orchestrator
var GlobalSess *session.Session
var GlobalAI ai.AIClient
var lastResponse string

var rootCmd = &cobra.Command{
	Use:   "mayo",
	Short: "Mayo is an autonomous AI research & data analysis partner",
	Long:  `An autonomous AI research & data analysis partner for your terminal, featuring Mayo the Poodle.`,
	Run: func(cmd *cobra.Command, args []string) {
		StartInteractiveShell()
	},
}

func StartInteractiveShell() {
	cfg, _ := config.LoadConfig()
	if cfg != nil {
		privacy.PrivacyMode = cfg.PrivacyMode
	}
	ui.PrintBanner("v1.2.0")

	// 1. Session selection first
	list, _ := session.ListSessions()
	if len(list) > 0 {
		options := []string{"[+ CREATE NEW SESSION]"}
		idMap := make(map[string]string)
		for _, s := range list {
			label := fmt.Sprintf("[%s] %s", s.ID[:8], s.Summary)
			options = append(options, label)
			idMap[label] = s.ID
		}

		var selectedLabel string
		err := survey.AskOne(&survey.Select{
			Message: "Choose Research Session:",
			Options: options,
		}, &selectedLabel)

		if err == nil && selectedLabel != "[+ CREATE NEW SESSION]" {
			id := idMap[selectedLabel]
			GlobalSess, _ = session.LoadSessionMetadata(id)
			session.InitVault(GlobalSess) // Initialize vault from stored session key
			ui.PrintSuccess(fmt.Sprintf("Welcome back! Resuming: %s", GlobalSess.Summary))
		}
	}

	if GlobalSess == nil {
		GlobalSess, _ = session.NewSession()
		ui.RenderStep("✨", "Starting a fresh research session...")
	}

	if cfg == nil || len(cfg.AIProfiles) == 0 {
		ui.PrintInfo("No AI Profiles set. Run /setup to configure your AI provider (Gemini, OpenAI, or Groq).")
	} else {
		InitAIClient(cfg)

		// Auto-connect to active DS profile
		if cfg.ActiveDSProfile != "" && GlobalAI != nil {
			for _, d := range cfg.DSProfiles {
				if d.Name == cfg.ActiveDSProfile {
					ui.RenderStep("🔌", fmt.Sprintf("Auto-connecting to saved source: %s [%s]", d.Name, d.Driver))
					HandleConnect(d.Driver, d.DSN, "", d.Name)
					break
				}
			}
		}
	}

	// Setup Readline with autocomplete
	completer := readline.NewPrefixCompleter(
		readline.PcItem("/connect",
			readline.PcItem("postgres"),
			readline.PcItem("mysql"),
			readline.PcItem("file"),
			readline.PcItem("folder"),
		),
		readline.PcItem("/model"),
		readline.PcItem("/profile",
			readline.PcItem("ai"),
			readline.PcItem("ds"),
		),
		readline.PcItem("/context"),
		readline.PcItem("/setup"),
		readline.PcItem("/sessions",
			readline.PcItem("create"),
			readline.PcItem("clear"),
			readline.PcItem("rename"),
			readline.PcItem("delete"),
			readline.PcItem("list"),
		),
		readline.PcItem("/session",
			readline.PcItem("create"),
			readline.PcItem("clear"),
			readline.PcItem("rename"),
			readline.PcItem("delete"),
			readline.PcItem("list"),
		),
		readline.PcItem("/history"),
		readline.PcItem("/audit"),
		readline.PcItem("/export"),
		readline.PcItem("/sources"),
		readline.PcItem("/disconnect"),
		readline.PcItem("/knowledge"),
		readline.PcItem("/privacy"),
		readline.PcItem("/debug"),
		readline.PcItem("/df",
			readline.PcItem("save"),
			readline.PcItem("commit"),
			readline.PcItem("status"),
			readline.PcItem("list"),
			readline.PcItem("load"),
			readline.PcItem("reset"),
			readline.PcItem("export"),
			readline.PcItem("delete"),
		),
		readline.PcItem("/help"),
		readline.PcItem("/exit"),
		readline.PcItem("/quit"),
	)

	rl, err := readline.NewEx(&readline.Config{
		Prompt:          ui.FormatPrompt("", "", "", false, false),
		AutoComplete:    completer,
		InterruptPrompt: "^C",
		EOFPrompt:       "exit",
	})
	if err != nil {
		fmt.Printf("Error initializing readline: %v\n", err)
		return
	}
	defer rl.Close()

	var lastInterrupt time.Time
	for {
		promptName := ""
		dfName := ""
		if GlobalOrchestrator != nil {
			dfName = GlobalOrchestrator.StagedName
			if len(GlobalOrchestrator.Connections) == 1 {
				for k := range GlobalOrchestrator.Connections {
					promptName = k
					break
				}
			} else if len(GlobalOrchestrator.Connections) > 1 {
				promptName = fmt.Sprintf("multi:%d", len(GlobalOrchestrator.Connections))
			} else if len(GlobalOrchestrator.Files) > 0 {
				promptName = "files"
			}
		}

		summary := ""
		if GlobalSess != nil {
			summary = GlobalSess.Summary
		}
		rl.SetPrompt(ui.FormatPrompt(promptName, dfName, summary, GlobalOrchestrator != nil, GlobalSess != nil))

		line, err := rl.Readline()
		if err != nil {
			if err == readline.ErrInterrupt {
				if time.Since(lastInterrupt) < 2*time.Second {
					fmt.Println(ui.StyleTitle.Render("\n👋 Goodbye!"))
					os.Exit(0)
				}
				lastInterrupt = time.Now()
				fmt.Println(ui.StyleStatus.Render("\n(Press Ctrl+C again to quit)"))
				continue
			}
			break
		}

		input := strings.TrimSpace(line)
		if input == "" {
			continue
		}

		if strings.HasPrefix(input, "/") {
			HandleSlashCommand(input)
			continue
		}

		if GlobalSess == nil {
			ui.PrintError("Session abandoned. Please use '/sessions create'.")
			continue
		}

		if GlobalOrchestrator == nil {
			ui.PrintError("Not connected. Use /connect [driver] [dsn] [alias].")
			continue
		}

		fmt.Println(ui.StyleStatus.Render("🤖 Thinking..."))
		resp, err := GlobalOrchestrator.ProcessQuery(context.Background(), input)
		if err != nil {
			ui.PrintError(err.Error())
			continue
		}

		ui.RenderSeparator()
		fmt.Println(resp)
		ui.RenderSeparator()
		lastResponse = resp
	}
}

func InitAIClient(cfg *config.Config) {
	var active *config.AIProfile
	for i, p := range cfg.AIProfiles {
		if p.Name == cfg.ActiveAIProfile {
			active = &cfg.AIProfiles[i]
			break
		}
	}

	if active == nil && len(cfg.AIProfiles) > 0 {
		active = &cfg.AIProfiles[0]
	}

	if active == nil || active.Provider == "" || active.APIKey == "" {
		ui.PrintError("AI Profile incomplete. Please run /setup.")
		return
	}

	ui.RenderStep("🧠", fmt.Sprintf("Initializing %s (%s)...", active.Provider, active.DefaultModel))
	switch active.Provider {
	case "openai":
		GlobalAI = ai.NewOpenAIClient(active.APIKey, "https://api.openai.com/v1", active.DefaultModel)
	case "groq":
		GlobalAI = ai.NewOpenAIClient(active.APIKey, "https://api.groq.com/openai/v1", active.DefaultModel)
	case "gemini":
		GlobalAI, _ = ai.NewGeminiClient(active.APIKey, active.DefaultModel)

	case "anthropic":
		GlobalAI = ai.NewAnthropicClient(active.APIKey, active.DefaultModel)
	default:
		ui.PrintError(fmt.Sprintf("Unknown provider: %s", active.Provider))
		return
	}

	if GlobalAI != nil && GlobalOrchestrator != nil {
		GlobalOrchestrator.AI = GlobalAI
	}
}

func HandleSlashCommand(input string) {
	parts, err := shellquote.Split(input)
	if err != nil {
		ui.PrintError(fmt.Sprintf("Invalid format: %v", err))
		return
	}
	if len(parts) == 0 {
		return
	}
	cmd := parts[0]

	switch cmd {
	case "/export":
		if len(parts) < 2 {
			ui.PrintInfo("Usage: /export [filename.md]")
			return
		}
		if lastResponse == "" {
			ui.PrintError("No output to export.")
			return
		}
		fname := parts[1]
		if !strings.HasSuffix(fname, ".md") {
			fname += ".md"
		}
		err := os.WriteFile(fname, []byte(lastResponse), 0644)
		if err != nil {
			ui.PrintError(err.Error())
		} else {
			ui.PrintSuccess("Exported.")
		}
	case "/this":
		cfg, _ := config.LoadConfig()
		var sb strings.Builder
		sb.WriteString("# 🛸 Mayo Session Inspection (Verbose Mode)\n\n")

		// 1. Session Information
		sb.WriteString("## 📦 CURRENT SESSION\n")
		if GlobalSess != nil {
			sb.WriteString(fmt.Sprintf("- **ID**: `%s`\n", GlobalSess.ID))
			sb.WriteString(fmt.Sprintf("- **Summary**: %s\n", GlobalSess.Summary))
			sb.WriteString(fmt.Sprintf("- **Started**: %s\n", GlobalSess.CreatedAt.Format(time.RFC1123)))
			sb.WriteString(fmt.Sprintf("- **Last Active**: %s\n", GlobalSess.UpdatedAt.Format(time.RFC1123)))

			sessionPath := filepath.Join(config.GetConfigDir(), "sessions", GlobalSess.ID)
			sb.WriteString(fmt.Sprintf("- **Root Directory**: `%s`\n", sessionPath))

			files, _ := os.ReadDir(sessionPath)
			sb.WriteString("- **Session Artifacts**:\n")
			for _, f := range files {
				if !f.IsDir() {
					sb.WriteString(fmt.Sprintf("  - [`%s`](file://%s)\n", f.Name(), filepath.Join(sessionPath, f.Name())))
				}
			}
		} else {
			sb.WriteString("- *No active session.*\n")
		}

		// 2. AI Intelligence Profile
		sb.WriteString("\n## 🧠 AI INTELLIGENCE\n")
		if GlobalAI != nil {
			sb.WriteString(fmt.Sprintf("- **Active Profile**: `%s`\n", cfg.ActiveAIProfile))
			sb.WriteString(fmt.Sprintf("- **Provider**: %s\n", strings.ToUpper(GlobalAI.GetProvider())))
			sb.WriteString(fmt.Sprintf("- **Model**: `%s`\n", GlobalAI.GetModel()))
		} else {
			sb.WriteString("- *AI Client not initialized.*\n")
		}

		// 3. Data Context & Dataframes
		sb.WriteString("\n## 📊 DATA CONTEXT\n")
		if GlobalOrchestrator != nil {
			if GlobalOrchestrator.StagedName != "" {
				dirtySuffix := ""
				if GlobalOrchestrator.IsDirty {
					dirtySuffix = " (⚠️ Uncommitted Changes)"
				}
				sb.WriteString(fmt.Sprintf("- **Active Dataframe**: `%s` %s\n", GlobalOrchestrator.StagedName, dirtySuffix))
				sb.WriteString(fmt.Sprintf("- **In-Memory Rows**: %d rows\n", len(GlobalOrchestrator.LastRows)))
			} else if GlobalOrchestrator.LastResultData != "" {
				sb.WriteString("- **Active Context**: *Latest Query Result* (Temporary)\n")
				sb.WriteString(fmt.Sprintf("- **In-Memory Rows**: %d rows\n", len(GlobalOrchestrator.LastRows)))
			} else {
				sb.WriteString("- **Active Context**: *Pure Database / Files Mode*\n")
			}
		}

		// 4. Data Sources (Connected)
		sb.WriteString("\n## 🔌 DATA SOURCES\n")
		if GlobalOrchestrator != nil && len(GlobalOrchestrator.Connections) > 0 {
			for alias, conn := range GlobalOrchestrator.Connections {
				typeStr := "External DB"
				if conn.IsImport {
					typeStr = "Imported File/Folder"
				}
				sb.WriteString(fmt.Sprintf("- **[%s]** (%s)\n", alias, typeStr))
				sb.WriteString(fmt.Sprintf("  - Source: `%s`\n", privacy.MaskCredentials(conn.Source)))
				sb.WriteString(fmt.Sprintf("  - Driver: `%s`\n", conn.Driver))
				if conn.Schema != nil {
					sb.WriteString(fmt.Sprintf("  - Schema Depth: %d Tables\n", len(conn.Schema.Tables)))
				}
			}
		} else {
			sb.WriteString("- *No active connections.*\n")
		}

		// 4. Knowledge & Vector DB
		sb.WriteString("\n## 📚 KNOWLEDGE CONTEXT\n")
		vectorDBPath := filepath.Join(config.GetConfigDir(), "data", "vectors.db")
		if _, err := os.Stat(vectorDBPath); err == nil {
			sb.WriteString(fmt.Sprintf("- **Vector DB (SQLite)**: `%s`\n", vectorDBPath))
			if GlobalSess != nil {
				localTableName := "vector_" + strings.ReplaceAll(GlobalSess.ID, "-", "_")
				sb.WriteString(fmt.Sprintf("- **Session Knowledge Table**: `%s`\n", localTableName))
			}
		} else {
			sb.WriteString("- *Knowledge base not yet initialized.*\n")
		}

		// 5. Custom Context
		sb.WriteString("\n## 📝 CUSTOM CONTEXT\n")
		if cfg.UserContext != "" {
			ctxSnippet := cfg.UserContext
			if len(ctxSnippet) > 100 {
				ctxSnippet = ctxSnippet[:97] + "..."
			}
			sb.WriteString(fmt.Sprintf("- **Raw Context**: \"%s\"\n", ctxSnippet))
		} else {
			sb.WriteString("- *No custom context provided.*\n")
		}

		// 6. Security & Privacy
		sb.WriteString("\n## 🛡️ SECURITY & PRIVACY\n")
		privacyStatus := "ON (Strict Masking)"
		if !cfg.PrivacyMode {
			privacyStatus = "OFF (Raw Data Visible)"
		}
		sb.WriteString(fmt.Sprintf("- **Privacy Mode**: %s\n", privacyStatus))
		if privacy.ActiveVault != nil {
			sb.WriteString("- **PII Vault**: Active (AES-256-GCM)\n")
			stats := privacy.ActiveVault.GetStats()
			if len(stats) > 0 {
				sb.WriteString("- **Tokenized Entities**:\n")
				for eType, count := range stats {
					sb.WriteString(fmt.Sprintf("  - %s: %d\n", eType, count))
				}
			} else {
				sb.WriteString("- **Tokenized Entities**: None yet\n")
			}
		} else {
			sb.WriteString("- **PII Vault**: Not initialized\n")
		}

		// 7. Debug Mode
		sb.WriteString("\n## 🛠️ DEBUGGING\n")
		debugStatus := "OFF"
		if ui.DebugEnabled {
			debugStatus = "ON (Verbose Prompt Logging)"
		}
		sb.WriteString(fmt.Sprintf("- **Debug Mode**: %s\n", debugStatus))

		ui.RenderMarkdown(sb.String())

	case "/sources":
		if GlobalOrchestrator == nil || len(GlobalOrchestrator.Connections) == 0 {
			ui.PrintInfo("No sources.")
			return
		}
		ui.PrintInfo("Sources:")
		for alias, conn := range GlobalOrchestrator.Connections {
			fmt.Printf(" - %s: %s\n", ui.StyleHighlight.Render(alias), conn.Driver)
		}

	case "/df":
		subCmd := ""
		if len(parts) >= 2 {
			subCmd = parts[1]
		}
	switchAgain:
		switch subCmd {
		case "commit":
			if GlobalOrchestrator == nil || len(GlobalOrchestrator.LastCols) == 0 {
				ui.PrintInfo("Nothing to commit. Run a query or load a dataframe first.")
				return
			}
			name := GlobalOrchestrator.StagedName
			if len(parts) >= 3 {
				name = strings.Join(parts[2:], "_")
			} else if name == "" {
				survey.AskOne(&survey.Input{Message: "Save as dataframe name:"}, &name)
			}

			if name == "" {
				return
			}
			if err := dataframe.Save(name, GlobalOrchestrator.LastSQL, GlobalOrchestrator.LastCols, GlobalOrchestrator.LastRows); err != nil {
				ui.PrintError(fmt.Sprintf("Failed to commit: %v", err))
				return
			}
			GlobalOrchestrator.StagedName = name
			GlobalOrchestrator.IsDirty = false
			ui.PrintSuccess(fmt.Sprintf("Committed to dataframe '%s' (%d rows)", name, len(GlobalOrchestrator.LastRows)))

		case "status":
			if GlobalOrchestrator == nil || len(GlobalOrchestrator.LastCols) == 0 {
				ui.PrintInfo("Empty working copy.")
				return
			}
			status := "Clean"
			if GlobalOrchestrator.IsDirty {
				status = "Uncommitted Changes"
			}
			name := GlobalOrchestrator.StagedName
			if name == "" {
				name = "New/Unnamed"
			}
			fmt.Printf("Dataframe: %s\nStatus: %s\nRows: %d\nColumns: %s\n",
				ui.StyleHighlight.Render(name), status, len(GlobalOrchestrator.LastRows), strings.Join(GlobalOrchestrator.LastCols, ", "))

		case "save": // Keep as alias for commit
			parts[1] = "commit"
			goto switchAgain // Bad practice but fast for REPL logic, let's just repeat the case or use a function.
			// Actually, I'll just fallthrough or repeat logic. Let's just repeat logic for clarity in this one-shot edit.
		case "list":
			frames, err := dataframe.List()
			if err != nil || len(frames) == 0 {
				ui.PrintInfo("No dataframes saved yet.")
				return
			}
			ui.PrintInfo("Saved Dataframes:")
			for _, f := range frames {
				dirtyMarker := ""
				if GlobalOrchestrator != nil && GlobalOrchestrator.StagedName == f.Name && GlobalOrchestrator.IsDirty {
					dirtyMarker = " *"
				}
				fmt.Printf("  %s%s  %s  (%d rows, cols: %s)\n",
					ui.StyleHighlight.Render(f.Name),
					dirtyMarker,
					ui.StyleMuted.Render(f.CreatedAt.Format("2006-01-02 15:04")),
					f.RowCount,
					strings.Join(f.Columns, ", "),
				)
			}

		case "load":
			name := ""
			if len(parts) >= 3 {
				name = parts[2]
			} else {
				frames, _ := dataframe.List()
				if len(frames) == 0 {
					ui.PrintInfo("No dataframes saved.")
					return
				}
				options := []string{}
				for _, f := range frames {
					options = append(options, f.Name)
				}
				survey.AskOne(&survey.Select{Message: "Select Dataframe to Load:", Options: options}, &name)
			}
			if name == "" {
				return
			}
			cols, rows, err := dataframe.Load(name)
			if err != nil {
				ui.PrintError(fmt.Sprintf("Failed to load: %v", err))
				return
			}
			if GlobalOrchestrator == nil {
				ui.PrintInfo("No orchestrator active.")
				return
			}
			// Inject into orchestrator as working copy
			GlobalOrchestrator.LastCols = cols
			GlobalOrchestrator.LastRows = rows
			GlobalOrchestrator.LastSQL = fmt.Sprintf("-- loaded from dataframe '%s'", name)
			GlobalOrchestrator.StagedName = name
			GlobalOrchestrator.IsDirty = false

			// Rebuild masked LastResultData for AI context (Sample only to save tokens)
			var sb strings.Builder
			sb.WriteString(strings.Join(cols, "|") + "\n")
			sampleSize := len(rows)
			if sampleSize > 20 {
				sampleSize = 20
			}

			for i := 0; i < sampleSize; i++ {
				row := rows[i]
				for j, val := range row {
					v := val
					if privacy.ActiveVault != nil && privacy.PrivacyMode {
						v = privacy.ActiveVault.Tokenize(v)
					}
					sb.WriteString(v)
					if j < len(row)-1 {
						sb.WriteString("|")
					}
				}
				sb.WriteString("\n")
			}
			GlobalOrchestrator.LastResultData = fmt.Sprintf("Format: CSV (Pipe Separated)\nData (showing 20 of %d total): %s",
				len(rows), sb.String())

			ui.PrintSuccess(fmt.Sprintf("Loaded dataframe '%s' into memory.", name))
			ui.RenderTable(cols, rows)

		case "export":
			name := ""
			if len(parts) >= 3 {
				name = parts[2]
			} else {
				frames, _ := dataframe.List()
				if len(frames) == 0 {
					ui.PrintInfo("No dataframes saved.")
					return
				}
				options := []string{}
				for _, f := range frames {
					options = append(options, f.Name)
				}
				survey.AskOne(&survey.Select{Message: "Select Dataframe to Export:", Options: options}, &name)
			}
			if name == "" {
				return
			}
			md, err := dataframe.ExportMarkdown(name)
			if err != nil {
				ui.PrintError(fmt.Sprintf("Export failed: %v", err))
				return
			}
			filename := name + ".md"
			if err := os.WriteFile(filename, []byte(md), 0644); err != nil {
				ui.PrintError(fmt.Sprintf("Could not write file: %v", err))
				return
			}
			ui.PrintSuccess(fmt.Sprintf("Exported to: %s", filename))

		case "reset":
			if GlobalOrchestrator != nil {
				GlobalOrchestrator.LastCols = nil
				GlobalOrchestrator.LastRows = nil
				GlobalOrchestrator.LastResultData = ""
				GlobalOrchestrator.StagedName = ""
				GlobalOrchestrator.IsDirty = false
				ui.PrintSuccess("Working copy reset. AI is now back to Pure Database mode.")
			}

		default:
			ui.PrintInfo("Usage:\n  /df commit [name] — persist memory to SQLite\n  /df list           — list saved dataframes\n  /df load <name>    — load into memory\n  /df reset          — clear memory (back to DB mode)\n  /df status         — show working copy status\n  /df export <name>  — export as Markdown table\n  /df delete <name>  — remove dataframe")
		}

	case "/connect":
		var driver, dsn, alias string
		if len(parts) >= 3 {
			driver = parts[1]
			dsn = parts[2]
			if len(parts) > 3 {
				alias = parts[3]
			}
		} else {
			// Interactive Prompting
			drivers := []string{"postgres", "mysql", "sqlite", "file"}
			err := survey.AskOne(&survey.Select{
				Message: "Select Driver:",
				Options: drivers,
			}, &driver)
			if err != nil {
				return
			}

			err = survey.AskOne(&survey.Input{
				Message: "Enter DSN (Connection String / Path):",
			}, &dsn)
			if err != nil {
				return
			}

			err = survey.AskOne(&survey.Input{
				Message: "Enter Alias (optional):",
			}, &alias)
			if err != nil {
				return
			}
		}

		if driver != "" && dsn != "" {
			HandleConnect(driver, dsn, alias, "")
		}
	case "/disconnect":
		if GlobalOrchestrator == nil || len(GlobalOrchestrator.Connections) == 0 {
			ui.PrintInfo("No active connections to disconnect.")
			return
		}

		var alias string
		if len(parts) > 1 {
			alias = parts[1]
		} else {
			options := []string{}
			for a := range GlobalOrchestrator.Connections {
				options = append(options, a)
			}
			sort.Strings(options)
			if len(options) == 1 {
				alias = options[0]
			} else {
				err := survey.AskOne(&survey.Select{
					Message: "Select Data Source to disconnect:",
					Options: options,
				}, &alias)
				if err != nil {
					return
				}
			}
		}

		if conn, ok := GlobalOrchestrator.Connections[alias]; ok {
			conn.DB.Close()
			delete(GlobalOrchestrator.Connections, alias)
			ui.PrintSuccess(fmt.Sprintf("Disconnected from '%s'.", alias))
		} else {
			ui.PrintError(fmt.Sprintf("Source '%s' not found.", alias))
		}
	case "/knowledge":
		if len(parts) < 2 {
			ui.PrintInfo("Usage: /knowledge [file]")
			return
		}
		HandleKnowledge(parts[1])
	case "/context":
		cfg, _ := config.LoadConfig()
		if len(parts) < 2 {
			ui.PrintInfo(fmt.Sprintf("Current Context: %s", cfg.UserContext))
			return
		}
		newCtx := strings.Join(parts[1:], " ")
		if newCtx == "clear" {
			var confirm bool
			survey.AskOne(&survey.Confirm{
				Message: "Are you sure you want to clear the user context?",
				Default: false,
			}, &confirm)
			if confirm {
				cfg.UserContext = ""
				ui.PrintSuccess("Cleared.")
			} else {
				ui.PrintInfo("Action cancelled.")
				return
			}
		} else {
			if _, err := os.Stat(newCtx); err == nil {
				data, err := os.ReadFile(newCtx)
				if err == nil {
					newCtx = string(data)
				}
			}
			cfg.UserContext = newCtx
			ui.PrintSuccess("Updated.")
		}
		config.SaveConfig(cfg)
		if GlobalOrchestrator != nil {
			GlobalOrchestrator.UserContext = cfg.UserContext
		}
	case "/debug":
		ui.DebugEnabled = !ui.DebugEnabled
		status := "DISABLED"
		if ui.DebugEnabled {
			status = "ENABLED (Prompts will be shown)"
		}
		ui.PrintSuccess(fmt.Sprintf("Debug Mode is now %s", status))
	case "/setup":
		RunSetup()
	case "/exit", "/quit":
		os.Exit(0)
	case "/model":
		if GlobalAI == nil {
			return
		}
		cfg, _ := config.LoadConfig()
		models := config.GetModelList()
		var selected string
		survey.AskOne(&survey.Select{Message: "Select Model:", Options: models}, &selected)
		if selected != "" {
			GlobalAI.SetModel(selected)

			// Persist to config
			if cfg != nil {
				for i, p := range cfg.AIProfiles {
					if p.Name == cfg.ActiveAIProfile {
						cfg.AIProfiles[i].DefaultModel = selected
						config.SaveConfig(cfg)
						break
					}
				}
			}
			ui.PrintSuccess("Model updated and saved.")
		}

	case "/profile":
		cfg, _ := config.LoadConfig()
		if len(parts) < 2 {
			ui.PrintInfo("Usage: /profile [ai|ds]")
			return
		}
		if parts[1] == "ai" {
			options := []string{}
			for _, p := range cfg.AIProfiles {
				options = append(options, p.Name)
			}
			var selected string
			survey.AskOne(&survey.Select{Message: "Select AI Profile:", Options: options}, &selected)
			if selected != "" {
				cfg.ActiveAIProfile = selected
				config.SaveConfig(cfg)
				InitAIClient(cfg)
			}
		} else if parts[1] == "ds" {
			options := []string{"[NONE - Disable Auto Connect]"}
			for _, p := range cfg.DSProfiles {
				options = append(options, p.Name)
			}
			var selected string
			survey.AskOne(&survey.Select{Message: "Select DS Profile:", Options: options}, &selected)
			if selected == "[NONE - Disable Auto Connect]" {
				cfg.ActiveDSProfile = ""
				config.SaveConfig(cfg)
				ui.PrintSuccess("Auto-connect disabled.")
			} else if selected != "" {
				cfg.ActiveDSProfile = selected
				config.SaveConfig(cfg)
				for _, d := range cfg.DSProfiles {
					if d.Name == selected {
						HandleConnect(d.Driver, d.DSN, "", d.Name)
					}
				}
			}
		}

	case "/sessions", "/session":
		if len(parts) < 2 {
			list, _ := session.ListSessions()
			if len(list) == 0 {
				ui.PrintInfo("No sessions found. Create one with '/sessions create'")
				return
			}

			options := []string{}
			idMap := make(map[string]string)
			for _, s := range list {
				// Indicate which one is active
				prefix := "  "
				if GlobalSess != nil && s.ID == GlobalSess.ID {
					prefix = "🟢 "
				}
				label := fmt.Sprintf("%s[%s] %s", prefix, s.ID[:8], s.Summary)
				options = append(options, label)
				idMap[label] = s.ID
			}

			var selectedLabel string
			err := survey.AskOne(&survey.Select{
				Message:  "Select Session to Switch:",
				Options:  options,
				PageSize: 10,
			}, &selectedLabel)

			if err == nil && selectedLabel != "" {
				id := idMap[selectedLabel]
				s, err := session.LoadSessionMetadata(id)
				if err == nil {
					GlobalSess = s
					session.InitVault(s) // Initialize vault with this session's key
					if GlobalOrchestrator != nil {
						GlobalOrchestrator.Session = s
					}
					ui.PrintSuccess(fmt.Sprintf("Switched to session: %s", s.ID[:8]))
				}
			}
			return
		}
		switch parts[1] {
		case "create":
			GlobalSess, _ = session.NewSession()
			if GlobalOrchestrator != nil {
				GlobalOrchestrator.Session = GlobalSess
			}
			ui.PrintSuccess("Created.")
		case "clear":
			var confirm bool
			survey.AskOne(&survey.Confirm{
				Message: "Are you sure you want to clear ALL sessions? This cannot be undone.",
				Default: false,
			}, &confirm)
			if confirm {
				session.ClearSessions()
				GlobalSess, _ = session.NewSession() // Create a fresh one after clearing
				if GlobalOrchestrator != nil {
					GlobalOrchestrator.Session = GlobalSess
				}
				ui.PrintSuccess("Cleared all sessions and started a fresh one.")
			} else {
				ui.PrintInfo("Action cancelled.")
			}
		case "delete":
			list, _ := session.ListSessions()
			if len(list) == 0 {
				ui.PrintInfo("No sessions to delete.")
				return
			}
			options := []string{}
			idMap := make(map[string]string)
			for _, s := range list {
				label := fmt.Sprintf("[%s] %s", s.ID[:8], s.Summary)
				options = append(options, label)
				idMap[label] = s.ID
			}
			var selectedLabel string
			err := survey.AskOne(&survey.Select{
				Message: "Select Session to Delete:",
				Options: options,
			}, &selectedLabel)
			if err == nil {
				id := idMap[selectedLabel]
				var confirm bool
				survey.AskOne(&survey.Confirm{
					Message: fmt.Sprintf("Are you sure you want to delete session %s?", id[:8]),
					Default: false,
				}, &confirm)
				if confirm {
					session.DeleteSession(id)
					if GlobalSess != nil && GlobalSess.ID == id {
						GlobalSess, _ = session.NewSession()
						if GlobalOrchestrator != nil {
							GlobalOrchestrator.Session = GlobalSess
						}
					}
					ui.PrintSuccess("Deleted.")
				}
			}
		case "rename":
			if GlobalSess == nil {
				return
			}
			var name string
			survey.AskOne(&survey.Input{Message: "New name:", Default: GlobalSess.Summary}, &name)
			if name != "" {
				session.UpdateSessionSummary(GlobalSess.ID, name)
				GlobalSess.Summary = name
				ui.PrintSuccess("Renamed.")
			}
		}
	case "/privacy":
		cfg, _ := config.LoadConfig()
		privacy.PrivacyMode = !privacy.PrivacyMode
		cfg.PrivacyMode = privacy.PrivacyMode
		config.SaveConfig(cfg)
		status := "ON (PII Masking Active)"
		if !privacy.PrivacyMode {
			status = "OFF (PII Masking Disabled)"
		}
		ui.PrintSuccess(fmt.Sprintf("Privacy Mode is now %s", status))
	case "/help":
		helpManual := `
# 🐶 Mayo Manual - Senior AI Data Partner (v1.2.0)

Mayo is your autonomous partner for deep data research and analysis. It combines LLM reasoning with direct database and file access. Use it to explore, query, and visualize complex datasets using natural language.

---

## 🛠️ CORE COMMANDS (Manual Pages)

### 🔌 Connectivity & Data Sources
- **/connect [driver] [dsn] [alias]**
  Creates a new connection to a data source.
  - **Drivers**: 
    - ` + "`postgres`" + `: Use for PostgreSQL databases.
    - ` + "`mysql`" + `: Use for MySQL/MariaDB.
    - ` + "`sqlite`" + `: Use for local SQLite files.
    - ` + "`file`" + `: Use for CSV or Excel (.xlsx) files.
  - **Sample (Postgres)**: 
    ` + "`/connect postgres \"host=localhost user=admin dbname=prod\" main_db`" + `
  - **Sample (Excel)**: 
    ` + "`/connect file \"./reports/sales_2024.xlsx\" sales`" + `

- **/sources**
  Displays verbose info about all active connections, their drivers, and aliases.

- **/disconnect [alias]**
  Unwires and closes a specific data source connection.
  - **Sample**: ` + "`/disconnect main_db`" + `
  - **Note**: If alias is omitted, Mayo will prompt for selection.

### 🧠 AI Intelligence & Configuration
- **/setup**
  Interactive wizard to configure AI Providers and Models.
  - Supports: **Gemini**, **OpenAI**, **Groq (Llama/Qwen)**, **Anthropic (Claude)**.

- **/model**
  Selectively change the LLM model for the current session.
  - **Sample**: Switch to ` + "`gemini-1.5-pro`" + ` for complex multi-table joins.
  - **Customization**: Add your own models in ` + "`~/.mayo-cli/models.txt`" + ` to avoid rebuilding.

- **/profile [ai|ds]**
  Instantly switch between saved AI or Data Source profiles.
  - **Sample**: ` + "`/profile ai`" + ` (to switch from OpenAI to Groq).

- **/context [text|file|clear]**
  Injects persistent business rules or domain knowledge into the AI's prompts.
  - **Sample**: ` + "`/context \"Always use Indonesian for final reports\"`" + `
  - **Sample**: ` + "`/context \"./docs/business_rules.txt\"`" + `

### 📚 Knowledge Base (RAG)
- **/knowledge [path]**
  Parses and indexes external documents into the local vector database.
  - **Formats**: ` + "`.pdf, .md, .txt`" + `.
  - **Sample**: ` + "`/knowledge \"./manuals/api_spec.pdf\"`" + `

- **/privacy**
  Toggle Privacy Mode ON/OFF. When ON, Mayo automatically masks PII (Emails, Phones) and credentials from UI, Logs, and AI prompts.
  - **Status**: Default is ON. Use this if you need to see raw PII data.

- **/debug**
  Toggle Debug Mode ON/OFF. When ON, Mayo will display the raw system and user prompts sent to the AI before processing.
  - **Sample**: Use this to troubleshoot why the AI is generating incorrect SQL.

### 💾 Sessions & Research Management
- **/sessions [create|rename|clear]**
  Manages the research lifecycle.
  - **Sample**: ` + "`/sessions rename`" + ` (allows naming your current research for later retrieval).

- **/export [filename.md]**
  Saves the current research state, AI answers, and SQL queries to a clean Markdown file.
  - **Sample**: ` + "`/export research_summary.md`" + `

### 🛸 Inspection & Debugging
- **/this**
  Provides an overwhelming, verbose dump of the current session state, file paths, AI client details, and active data source schemas.

### 🚪 Exit & Quit
- **/exit** or **/quit** - Safely terminates the CLI session.

---

## 💡 EXPERT USAGE TIPS
1. **Multi-Source Joins**: Connect a Postgres DB and an Excel file at the same time. You can ask: "How many users in ` + "`production_db`" + ` have orders recorded in the ` + "`sales`" + ` Excel sheet?"
2. **Read-Only Safety**: Every SQL query is validated against a blacklist of WRITE operations (` + "`INSERT`" + `, ` + "`UPDATE`" + `, ` + "`DELETE`" + `, etc.).
3. **Thought Blocks**: Look at the ` + "`thought`" + ` blocks to understand *how* the AI interpreted your data schema.

*Developed by Popolo Research Labs.*
*Status: READY | READ-ONLY: ON*
---`
		ui.RenderMarkdown(helpManual)
	}
}

func HandleKnowledge(filePath string) {
	ui.RenderStep("📚", fmt.Sprintf("Loading: %s", filePath))
	doc, err := knowledge.LoadKnowledge(filePath)
	if err != nil {
		ui.PrintError(err.Error())
		return
	}

	cfg, _ := config.LoadConfig()
	const localOption = "✨ [+] Create Local SQLite Vector (Session Based)"
	options := []string{localOption}
	for _, p := range cfg.DSProfiles {
		options = append(options, p.Name)
	}

	var selected string
	err = survey.AskOne(&survey.Select{Message: "Select Storage Profile:", Options: options}, &selected)
	if err != nil || selected == "" {
		return
	}

	var dbConn *sql.DB
	var tableName string

	if selected == localOption {
		if GlobalSess == nil {
			ui.PrintError("No active session to create local storage.")
			return
		}
		// Sanitize session ID for table name
		tableName = "vector_" + strings.ReplaceAll(GlobalSess.ID, "-", "_")
		sqlitePath := filepath.Join(config.GetConfigDir(), "data", "vectors.db")

		// Create data dir if not exists
		os.MkdirAll(filepath.Dir(sqlitePath), 0755)

		dbConn, err = db.Connect("sqlite", sqlitePath)
		if err != nil {
			ui.PrintError(fmt.Sprintf("Local DB error: %v", err))
			return
		}
	} else {
		var ds config.DSProfile
		for _, p := range cfg.DSProfiles {
			if p.Name == selected {
				ds = p
				break
			}
		}
		survey.AskOne(&survey.Input{Message: "Table name:", Default: "knowledge_base"}, &tableName)
		dbConn, err = db.Connect(ds.Driver, ds.DSN)
		if err != nil {
			ui.PrintError(err.Error())
			return
		}
	}
	defer dbConn.Close()

	ui.RenderStep("⚙️", "Indexing knowledge...")
	if err := knowledge.IndexDocument(dbConn, doc, tableName); err != nil {
		ui.PrintError(err.Error())
	} else {
		ui.PrintSuccess(fmt.Sprintf("Knowledge Indexed into table: %s", tableName))
	}
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func init() {
	cobra.OnInitialize(func() { config.InitConfig() })
}
