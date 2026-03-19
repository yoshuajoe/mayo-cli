package cmd

import (
	"context"
	"database/sql"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"mayo-cli/internal/ai"
	"mayo-cli/internal/api"
	"mayo-cli/internal/changelog"
	"mayo-cli/internal/config"
	"mayo-cli/internal/dataframe"
	"mayo-cli/internal/db"
	"mayo-cli/internal/enhancer"
	"mayo-cli/internal/files"
	"mayo-cli/internal/git"
	"mayo-cli/internal/knowledge"
	"mayo-cli/internal/privacy"
	"mayo-cli/internal/session"
	"mayo-cli/internal/teleskop"
	"mayo-cli/internal/ui"
	"mayo-cli/pkg/version"
	"os/exec"

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

var quietChangelog bool

var releaseNotesCmd = &cobra.Command{
	Use:   "release-notes [version]",
	Short: "Generate AI-powered changelog from git commits",
	Args:  cobra.MaximumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		ver := "HEAD"
		if len(args) > 0 {
			ver = args[0]
		}

		cfg, err := config.LoadConfig()
		// It's okay if config loading fails in CI/CD as we check env vars during InitAIClient
		InitAIClient(cfg)

		if GlobalAI == nil {
			fmt.Println("AI not configured. Set MAYO_AI_KEY/PROVIDER env vars or run /setup.")
			os.Exit(1)
		}

		if !quietChangelog {
			fmt.Println("Fetching git commits...")
		}
		commits, err := git.GetCommitsSinceLastTag()
		if err != nil {
			fmt.Printf("Git error: %v\n", err)
			os.Exit(1)
		}

		if !quietChangelog {
			fmt.Println("Generating AI changelog...")
		}
		md, err := ai.GenerateChangelogFromCommits(context.Background(), GlobalAI, commits, ver)
		if err != nil {
			fmt.Printf("AI error: %v\n", err)
			os.Exit(1)
		}

		if quietChangelog {
			fmt.Println(md)
		} else {
			fmt.Println("\n--- GENERATED CHANGELOG ---")
			fmt.Println(md)
			fmt.Println("---------------------------")
		}
	},
}

func init() {
	releaseNotesCmd.Flags().BoolVarP(&quietChangelog, "quiet", "q", false, "Output only the raw markdown")
	rootCmd.AddCommand(releaseNotesCmd)
	rootCmd.AddCommand(enhanceWorkerCmd)
}

var enhanceWorkerCmd = &cobra.Command{
	Use:    "enhance-worker [id]",
	Hidden: true,
	Args:   cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		id := args[0]
		worker, err := enhancer.NewWorker(id)
		if err != nil {
			fmt.Printf("Worker initialization failed: %v\n", err)
			os.Exit(1)
		}
		if err := worker.Run(context.Background()); err != nil {
			fmt.Printf("Worker error: %v\n", err)
			os.Exit(1)
		}
	},
}

func StartInteractiveShell() {
	cfg, _ := config.LoadConfig()
	if cfg != nil {
		privacy.PrivacyMode = cfg.PrivacyMode
	}
	ui.PrintBanner(version.Version)

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

	// Track if this is a fresh session
	isNewSession := false
	if GlobalSess == nil {
		GlobalSess, _ = session.NewSession()
		isNewSession = true
		ui.RenderStep("✨", "Starting a fresh research session...")
	}

	if cfg == nil || len(cfg.AIProfiles) == 0 {
		ui.PrintInfo("Welcome! No configuration found. Run '/wizard' for a guided onboarding or '/setup' for direct setup.")
	} else {
		InitAIClient(cfg)

		// Auto-connect for resumed sessions only
		var targets []string
		if !isNewSession {
			if len(GlobalSess.ConnectedProfiles) > 0 {
				targets = GlobalSess.ConnectedProfiles
			} else if cfg.ActiveDSProfile != "" {
				targets = []string{cfg.ActiveDSProfile}
			}
		}

		if len(targets) > 0 && GlobalAI != nil {
			for _, targetDS := range targets {
				for _, d := range cfg.DSProfiles {
					if d.Name == targetDS {
						ui.RenderStep("🔌", fmt.Sprintf("Auto-connecting to saved source: %s [%s]", d.Name, d.Driver))
						HandleConnect(d.Driver, d.DSN, "", d.Name)
						break
					}
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
		readline.PcItem("/describe"),
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
		readline.PcItem("/serve",
			readline.PcItem("spawn"),
			readline.PcItem("status"),
			readline.PcItem("logs"),
			readline.PcItem("stop"),
		),
		readline.PcItem("/scraper",
			readline.PcItem("spawn"),
			readline.PcItem("list"),
			readline.PcItem("status"),
			readline.PcItem("logs"),
			readline.PcItem("stop"),
			readline.PcItem("delete"),
			readline.PcItem("usage"),
			readline.PcItem("summary"),
			readline.PcItem("head"),
			readline.PcItem("tail"),
		),
		readline.PcItem("/enhance",
			readline.PcItem("start"),
			readline.PcItem("list"),
			readline.PcItem("status"),
			readline.PcItem("stop"),
			readline.PcItem("logs"),
		),
		readline.PcItem("/share"),
		readline.PcItem("/this"),
		readline.PcItem("/scan"),
		readline.PcItem("/reconcile"),
		readline.PcItem("/plot"),
		readline.PcItem("/analyst"),
		readline.PcItem("/changelog"),
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
			ui.PrintError("Not connected. Use /connect to link a database OR /knowledge to index documents.")
			continue
		}

		fmt.Println(ui.StyleStatus.Render("🤖 Thinking..."))
		resp, err := GlobalOrchestrator.ProcessQuery(context.Background(), input)
		if err != nil {
			ui.PrintError(err.Error())
			continue
		}

		ui.RenderSeparator()
		lastResponse = resp
	}
}

func InitAIClient(cfg *config.Config) {
	// 1. Check for Environment Variables (Point 1.A improvement for CI/CD)
	envProvider := os.Getenv("MAYO_AI_PROVIDER")
	envKey := os.Getenv("MAYO_AI_KEY")
	envModel := os.Getenv("MAYO_AI_MODEL")

	if envProvider != "" && envKey != "" {
		GlobalAI = ai.NewClient(envProvider, envKey, envModel)
		ui.RenderStep("🧠", fmt.Sprintf("Initializing %s (%s) from environment variables...", envProvider, envModel))
		if GlobalAI != nil && GlobalOrchestrator != nil {
			GlobalOrchestrator.AI = GlobalAI
		}
		return
	}

	// 2. Fallback to Config Profile
	if cfg == nil || len(cfg.AIProfiles) == 0 {
		return
	}

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

	if active == nil {
		return
	}

	apiKey := active.GetAPIKey(cfg.UseKeyring)
	if apiKey == "" {
		ui.PrintError("AI Profile incomplete (API Key missing). Please run /setup.")
		return
	}

	ui.RenderStep("🧠", fmt.Sprintf("Initializing %s (%s)...", active.Provider, active.DefaultModel))
	GlobalAI = ai.NewClient(active.Provider, apiKey, active.DefaultModel)

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
	case "/enhance":
		if GlobalOrchestrator == nil || GlobalOrchestrator.StagedName == "" {
			ui.PrintError("AI Data Enhancer is only available in 'Dataframe Mode'.")
			ui.PrintInfo("Please load a dataframe first with '/df load [name]'.")
			return
		}
		if len(parts) > 1 {
			sub := parts[1]
			args := parts[2:]
			switch sub {
			case "start":
				startEnhanceCmd.ParseFlags(args)
				HandleEnhanceStart(startEnhanceCmd, args)
			case "list":
				listEnhanceCmd.ParseFlags(args)
				showAll, _ := listEnhanceCmd.Flags().GetBool("all")
				HandleEnhanceList(showAll)
			case "status":
				HandleEnhanceStatus(args)
			case "stop":
				HandleEnhanceStop(args)
			case "logs":
				HandleEnhanceLogs(args)
			default:
				ui.PrintInfo("Usage: /enhance [start|list|status|stop|logs]")
			}
		} else {
			ui.PrintInfo("Usage: /enhance [start|list|status|stop|logs]")
		}
	case "/export":
		if len(parts) < 2 {
			ui.PrintInfo("Usage: /export [filename.md]")
			return
		}
		if GlobalSess == nil {
			ui.PrintError("No active research session to export.")
			return
		}
		fname := parts[1]
		if !strings.HasSuffix(fname, ".md") {
			fname += ".md"
		}

		fullLog, err := session.ReadSessionLogs(GlobalSess.ID)
		if err != nil || fullLog == "" {
			// Fallback to last response if no logs found
			if lastResponse == "" {
				ui.PrintError("No research results found to export.")
				return
			}
			fullLog = lastResponse
		}

		err = os.WriteFile(fname, []byte("# Mayo Research Export: "+GlobalSess.Summary+"\n*Session ID: "+GlobalSess.ID+"*\n\n"+fullLog), 0644)
		if err != nil {
			ui.PrintError(err.Error())
		} else {
			ui.PrintSuccess(fmt.Sprintf("Export complete! Research history saved to %s", fname))
		}
	case "/share":
		if GlobalOrchestrator == nil || GlobalSess == nil {
			ui.PrintError("AI Client or Session not initialized. Run /setup first.")
			return
		}

		report, err := GlobalOrchestrator.GenerateReport(context.Background())
		if err != nil {
			ui.PrintError(err.Error())
			return
		}

		ui.RenderSeparator()
		ui.PrintInfo("✨ GENERATED RESEARCH REPORT ✨")
		ui.RenderMarkdown(report)
		ui.RenderSeparator()

		// Offer to save it
		var save bool
		survey.AskOne(&survey.Confirm{
			Message: "Do you want to save this report to a file?",
			Default: true,
		}, &save)

		if save {
			cleanSummary := strings.ReplaceAll(GlobalSess.Summary, " ", "_")
			cleanSummary = regexp.MustCompile(`[^a-zA-Z0-9_]`).ReplaceAllString(cleanSummary, "")
			fname := fmt.Sprintf("report_%s.md", cleanSummary)
			if len(parts) > 1 {
				fname = parts[1]
				if !strings.HasSuffix(fname, ".md") {
					fname += ".md"
				}
			}
			err := os.WriteFile(fname, []byte(report), 0644)
			if err != nil {
				ui.PrintError(err.Error())
			} else {
				ui.PrintSuccess(fmt.Sprintf("Report saved and ready to share: %s", fname))
			}
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

		keyringStatus := "DISABLED (Plaintext Config)"
		if cfg.UseKeyring {
			keyringStatus = "ENABLED (System Keyring)"
		}
		sb.WriteString(fmt.Sprintf("- **Credential Storage**: %s\n", keyringStatus))

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

		// 7. User Preferences
		sb.WriteString("\n## ⚙️ USER PREFERENCES\n")
		limitStr := "None"
		if cfg.DefaultLimit > 0 {
			limitStr = fmt.Sprintf("%d rows", cfg.DefaultLimit)
		}
		sb.WriteString(fmt.Sprintf("- **Default SQL Limit**: %s\n", limitStr))

		interactiveStr := "OFF"
		if cfg.Interactive {
			interactiveStr = "ON (Confirmation Required)"
		}
		sb.WriteString(fmt.Sprintf("- **Interactive Review**: %s\n", interactiveStr))

		analystStr := "OFF (AI analysis disabled)"
		if cfg.AnalystEnabled {
			analystStr = "ON (AI analysis after queries)"
		}
		sb.WriteString(fmt.Sprintf("- **Analyst Insight**: %s\n", analystStr))

		// 8. Debug Mode
		sb.WriteString("\n## 🛠️ DEBUGGING\n")
		debugStatus := "OFF"
		if ui.DebugEnabled {
			debugStatus = "ON (Verbose Prompt Logging)"
		}
		sb.WriteString(fmt.Sprintf("- **Debug Mode**: %s\n", debugStatus))

		ui.RenderMarkdown(sb.String())

	case "/scan":
		if GlobalOrchestrator == nil {
			ui.PrintError("Not initialized.")
			return
		}
		ui.PrintInfo("Starting schema scan and metadata enrichment...")
		if err := GlobalOrchestrator.SyncSchema(context.Background()); err != nil {
			ui.PrintError(err.Error())
		} else {
			ui.PrintSuccess("Scan complete.")
		}
	case "/sources":
		if GlobalOrchestrator == nil {
			ui.PrintInfo("No sources.")
			return
		}
		ui.PrintInfo("Sources:")

		// 1. Teleskop.id Scraper (Always at top if configured)
		cfg, _ := config.LoadConfig()
		if cfg.GetTeleskopAPIKey() != "" {
			fmt.Printf(" - %s: %s\n", ui.StyleHighlight.Render("teleskop"), "Teleskop.id Scraper")
		}

		// 2. Other connections
		if len(GlobalOrchestrator.Connections) > 0 {
			for alias, conn := range GlobalOrchestrator.Connections {
				fmt.Printf(" - %s: %s\n", ui.StyleHighlight.Render(alias), conn.Driver)
			}
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
			if err != nil {
				frames = []dataframe.Frame{}
			}

			ui.PrintInfo("Saved Dataframes:")
			hasContent := false
			for _, f := range frames {
				hasContent = true
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

			// Also list Knowledge Vector Stores
			vectorDBPath := filepath.Join(config.GetConfigDir(), "data", "vectors.db")
			if _, err := os.Stat(vectorDBPath); err == nil {
				if kb, err := db.Connect("sqlite", vectorDBPath); err == nil {
					defer kb.Close()
					rows, _ := kb.Query("SELECT name FROM sqlite_master WHERE type='table' AND name LIKE 'vector_%' AND name NOT LIKE '%_fts%'")
					if rows != nil {
						firstK := true
						for rows.Next() {
							var tName string
							if err := rows.Scan(&tName); err == nil {
								if firstK {
									fmt.Printf("\n%s\n", ui.StyleMuted.Render("Knowledge Data Assets (Queryable via /df load knowledge:ID):"))
									firstK = false
									hasContent = true
								}
								id := strings.TrimPrefix(tName, "vector_")
								// Get row count
								var count int
								kb.QueryRow(fmt.Sprintf("SELECT COUNT(*) FROM %s", tName)).Scan(&count)
								// Get source filenames
								sourceRows, _ := kb.Query(fmt.Sprintf("SELECT DISTINCT source FROM %s", tName))
								var sources []string
								if sourceRows != nil {
									for sourceRows.Next() {
										var src string
										if sourceRows.Scan(&src) == nil {
											sources = append(sources, src)
										}
									}
									sourceRows.Close()
								}
								sourceLabel := ""
								if len(sources) > 0 {
									sourceLabel = " (" + strings.Join(sources, ", ") + ")"
								}
								fmt.Printf("  %s  (%d chunks)%s\n", ui.StyleHighlight.Render("knowledge:"+id), count, sourceLabel)
							}
						}
						rows.Close()
					}
				}
			}

			if !hasContent {
				ui.PrintInfo("No dataframes or knowledge assets found.")
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

			var cols []string
			var rows [][]string
			var err error

			if strings.HasPrefix(name, "knowledge:") {
				// Load from knowledge vector store as a queryable dataset
				sessionTag := strings.TrimPrefix(name, "knowledge:")
				knowledgeTableName := "vector_" + strings.ReplaceAll(sessionTag, "-", "_")
				sqlitePath := filepath.Join(config.GetConfigDir(), "data", "vectors.db")
				kb, kbErr := db.Connect("sqlite", sqlitePath)
				if kbErr != nil {
					ui.PrintError(fmt.Sprintf("Failed to open knowledge store: %v", kbErr))
					return
				}
				defer kb.Close()
				cols, rows, err = knowledge.LoadAsDataframe(kb, knowledgeTableName)
			} else if strings.HasPrefix(name, "scraper:") {
				scraperID := strings.TrimPrefix(name, "scraper:")
				cols, rows, err = teleskop.GetHead(scraperID, 1000) // Default load 1000 rows
			} else {
				cols, rows, err = dataframe.Load(name)
			}

			if err != nil {
				ui.PrintError(fmt.Sprintf("Failed to load: %v", err))
				return
			}
			if GlobalOrchestrator == nil {
				cfg, _ := config.LoadConfig()
				if cfg != nil {
					GlobalOrchestrator = &ai.Orchestrator{
						AI:             GlobalAI,
						Connections:    make(map[string]*ai.DBConnection),
						Session:        GlobalSess,
						DefaultLimit:   cfg.DefaultLimit,
						Interactive:    cfg.Interactive,
						AnalystEnabled: cfg.AnalystEnabled,
						Files:          []*files.FileData{},
					}
				} else {
					ui.PrintInfo("No configuration found. Please run /setup first.")
					return
				}
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

			// Show Summary instead of all data
			fmt.Printf("\n%s\n", ui.StyleHighlight.Render("--- DATAFRAME SUMMARY ---"))
			fmt.Printf("Records : %d rows\n", len(rows))
			fmt.Printf("Fields  : %d columns\n", len(cols))
			fmt.Printf("Columns : %s\n", ui.StyleHighlight.Render(strings.Join(cols, ", ")))

			// Show a small sample
			sampleSize = 5
			if len(rows) < sampleSize {
				sampleSize = len(rows)
			}
			if sampleSize > 0 {
				fmt.Printf("\n%s (Top %d):\n", ui.StyleMuted.Render("Preview"), sampleSize)
				ui.RenderTable(cols, rows[:sampleSize])
			} else {
				ui.PrintInfo("Dataframe is empty.")
			}

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
			format := "markdown"
			if len(parts) >= 4 {
				format = strings.ToLower(parts[3])
			} else {
				survey.AskOne(&survey.Select{
					Message: "Select Export Format:",
					Options: []string{"markdown", "csv", "excel", "json"},
					Default: "markdown",
				}, &format)
			}

			var filename string
			var err error
			switch format {
			case "csv":
				filename = name + ".csv"
				err = dataframe.ExportCSV(name, filename)
			case "excel", "xlsx":
				filename = name + ".xlsx"
				err = dataframe.ExportExcel(name, filename)
			case "json":
				filename = name + ".json"
				err = dataframe.ExportJSON(name, filename)
			default:
				md, err2 := dataframe.ExportMarkdown(name)
				if err2 == nil {
					filename = name + ".md"
					err = os.WriteFile(filename, []byte(md), 0644)
				} else {
					err = err2
				}
			}

			if err != nil {
				ui.PrintError(fmt.Sprintf("Export failed: %v", err))
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

		case "delete":
			name := ""
			if len(parts) >= 3 {
				name = parts[2]
			} else {
				ui.PrintError("Usage: /df delete <name>")
				return
			}

			var confirm bool
			survey.AskOne(&survey.Confirm{Message: fmt.Sprintf("Are you sure you want to delete dataframe/asset '%s'?", name), Default: false}, &confirm)
			if !confirm {
				return
			}

			if strings.HasPrefix(name, "knowledge:") {
				id := strings.TrimPrefix(name, "knowledge:")
				knowledgeTableName := "vector_" + strings.ReplaceAll(id, "-", "_")
				sqlitePath := filepath.Join(config.GetConfigDir(), "data", "vectors.db")
				kb, err := db.Connect("sqlite", sqlitePath)
				if err != nil {
					ui.PrintError(err.Error())
					return
				}
				defer kb.Close()
				_, err = kb.Exec(fmt.Sprintf("DROP TABLE IF EXISTS %s", knowledgeTableName))
				_, _ = kb.Exec(fmt.Sprintf("DROP TABLE IF EXISTS %s_fts", knowledgeTableName))
				if err != nil {
					ui.PrintError(err.Error())
					return
				}
				ui.PrintSuccess(fmt.Sprintf("Deleted knowledge asset '%s'", name))
			} else {
				if err := dataframe.Delete(name); err != nil {
					ui.PrintError(err.Error())
					return
				}
				ui.PrintSuccess(fmt.Sprintf("Deleted dataframe '%s'", name))
			}
			if GlobalOrchestrator != nil && GlobalOrchestrator.StagedName == name {
				GlobalOrchestrator.StagedName = ""
				GlobalOrchestrator.IsDirty = false
			}

		default:
			ui.PrintInfo("Usage:\n  /df commit [name] — persist memory to SQLite\n  /df list           — list saved dataframes\n  /df load <name>    — load into memory\n  /df reset          — clear memory (back to DB mode)\n  /df status         — show working copy status\n  /df export [name] [format] — export as markdown/csv/excel/json\n  /df delete <name>  — remove dataframe")
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
	case "/reconcile":
		if GlobalOrchestrator == nil {
			ui.PrintError("Not connected.")
			return
		}
		if len(parts) < 3 {
			ui.PrintInfo("Usage: /reconcile <alias1> <alias2>")
			return
		}
		session.LogToSession(GlobalSess.ID, fmt.Sprintf("User requested reconciliation: /reconcile %s %s", parts[1], parts[2]))
		resp, err := GlobalOrchestrator.Reconcile(context.Background(), parts[1], parts[2])
		if err != nil {
			ui.PrintError(err.Error())
		} else {
			lastResponse = resp
			session.LogToSession(GlobalSess.ID, fmt.Sprintf("Mayo Recon Result: %s", resp))
			ui.RenderMarkdown(resp)
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
	case "/plot":
		if GlobalOrchestrator == nil || len(GlobalOrchestrator.LastRows) == 0 {
			ui.PrintInfo("No data in memory to plot. Run a query first.")
			return
		}

		cols := GlobalOrchestrator.LastCols
		rows := GlobalOrchestrator.LastRows

		// Map numerical columns
		numOptions := []string{}
		for i, c := range cols {
			numOptions = append(numOptions, fmt.Sprintf("%d: %s", i, c))
		}

		var selectedNum string
		err := survey.AskOne(&survey.Select{
			Message: "Select Numeric Column (Y-Axis):",
			Options: numOptions,
		}, &selectedNum)
		if err != nil {
			return
		}

		var numCol int
		fmt.Sscanf(selectedNum, "%d:", &numCol)

		var selectedLabel string
		survey.AskOne(&survey.Select{
			Message: "Select Label Column (X-Axis, e.g. Date/ID):",
			Options: append([]string{"[None/Sequence]"}, numOptions...),
		}, &selectedLabel)

		labelCol := -1
		if selectedLabel != "[None/Sequence]" {
			fmt.Sscanf(selectedLabel, "%d:", &labelCol)
		}

		var dataPoints []float64
		var min, max, sum float64
		min = math.MaxFloat64

		for _, row := range rows {
			var val float64
			// Clean currency/percent signs if any
			cleanVal := regexp.MustCompile(`[^0-9\.-]`).ReplaceAllString(row[numCol], "")
			fmt.Sscanf(cleanVal, "%f", &val)
			dataPoints = append(dataPoints, val)

			if val < min {
				min = val
			}
			if val > max {
				max = val
			}
			sum += val
		}

		if len(dataPoints) == 0 {
			ui.PrintError("No numeric data points found.")
			return
		}

		avg := sum / float64(len(dataPoints))
		title := fmt.Sprintf("Plot: %s", cols[numCol])
		if labelCol != -1 {
			title += fmt.Sprintf(" grouped by %s", cols[labelCol])
		}

		fmt.Println(ui.RenderChart(dataPoints, title))

		// Show Stats Summary
		fmt.Printf("\n%s\n", ui.StyleHighlight.Render("📈 STATS SUMMARY"))
		fmt.Printf("Records : %d\n", len(dataPoints))
		fmt.Printf("Min     : %s\n", ui.StyleError.Render(fmt.Sprintf("%.2f", min)))
		fmt.Printf("Max     : %s\n", ui.StyleSuccess.Render(fmt.Sprintf("%.2f", max)))
		fmt.Printf("Average : %s\n", ui.StyleTitle.Render(fmt.Sprintf("%.2f", avg)))

		if labelCol != -1 && len(rows) > 0 {
			fmt.Printf("Range   : %s to %s\n", rows[0][labelCol], rows[len(rows)-1][labelCol])
		}
	case "/changelog":
		logs, err := changelog.GetChangelogs()
		if err != nil {
			ui.PrintInfo("No changelogs found. Check internal/changelog/data.")
			return
		}

		ui.PrintInfo("--- CHANGELOGS ---")
		for _, content := range logs {
			ui.RenderMarkdown(content)
			ui.RenderSeparator()
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
			ui.PrintError("AI Client not initialized. Run /setup first.")
			return
		}
		cfg, _ := config.LoadConfig()
		provider := ""
		for _, p := range cfg.AIProfiles {
			if p.Name == cfg.ActiveAIProfile {
				provider = p.Provider
				break
			}
		}
		models := config.GetModelList(provider)
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
	case "/describe":
		if GlobalOrchestrator == nil {
			ui.PrintError("Not connected.")
			return
		}
		target := ""
		if len(parts) > 1 {
			target = parts[1]
		} else {
			// Prompt for target if not specified
			options := []string{"df (Active Dataframe)"}
			for k := range GlobalOrchestrator.Connections {
				options = append(options, k)
			}
			if len(options) == 1 && options[0] == "df (Active Dataframe)" && GlobalOrchestrator.StagedName == "" && len(GlobalOrchestrator.LastRows) == 0 {
				ui.PrintInfo("No active dataframe or connections to describe.")
				return
			}

			err := survey.AskOne(&survey.Select{
				Message: "Select target to describe:",
				Options: options,
			}, &target)
			if err != nil {
				return
			}
			if strings.HasPrefix(target, "df") {
				target = "df"
			}
		}

		session.LogToSession(GlobalSess.ID, fmt.Sprintf("User requested description: /describe %s", target))
		resp, err := GlobalOrchestrator.Describe(context.Background(), target)
		if err != nil {
			ui.PrintError(err.Error())
		} else {
			lastResponse = resp
			session.LogToSession(GlobalSess.ID, fmt.Sprintf("Mayo Description: %s", resp))
			ui.RenderMarkdown(resp)
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
	case "/analyst":
		cfg, _ := config.LoadConfig()
		cfg.AnalystEnabled = !cfg.AnalystEnabled
		config.SaveConfig(cfg)
		if GlobalOrchestrator != nil {
			GlobalOrchestrator.AnalystEnabled = cfg.AnalystEnabled
		}
		status := "ON (AI analysis will run after queries)"
		if !cfg.AnalystEnabled {
			status = "OFF (AI analysis disabled)"
		}
		ui.PrintSuccess(fmt.Sprintf("Analyst Insight is now %s", status))
	case "/serve":
		mgr := api.NewManager()
		if len(parts) < 2 {
			ui.PrintInfo("Usage: /serve [spawn|status|logs|stop]")
			return
		}

		switch parts[1] {
		case "spawn":
			var port int = 8080
			cfg, _ := config.LoadConfig()
			if len(parts) >= 3 {
				if p, err := strconv.Atoi(parts[2]); err == nil {
					port = p
				}
			}

			token := ""
			if cfg != nil {
				token = cfg.ServeToken
			}

			ui.RenderStep("🚀", fmt.Sprintf("Spawning Mayo Master API on port %d...", port))
			// Pass empty sessionID to indicate Master Mode
			pid, err := mgr.Spawn("", port, token)
			if err != nil {
				ui.PrintError(err.Error())
			} else {
				ui.PrintSuccess(fmt.Sprintf("Master API Server started in background (PID: %d)", pid))
				
				// CURL Example
				sessionID := "SESSION_ID"
				if GlobalSess != nil {
					sessionID = GlobalSess.ID
				}
				fmt.Printf("\n%s\n", ui.StyleMuted.Render("Example CURL for current session:"))
				authHeader := ""
				if token != "" {
					authHeader = fmt.Sprintf("-H \"Authorization: Bearer %s\" ", token)
				}
				fmt.Printf("curl -X POST http://localhost:%d/v1/%s/query %s-H \"Content-Type: application/json\" -d '{\"query\": \"Hello Mayo!\"}'\n\n", port, sessionID, authHeader)
				ui.PrintInfo("You can query ANY session via http://localhost:" + strconv.Itoa(port) + "/v1/:session_id/query")
			}

		case "status":
			servers := mgr.List()
			if len(servers) == 0 {
				ui.PrintInfo("No background API servers running.")
				return
			}

			fmt.Printf("\n%s\n", ui.StyleHighlight.Render("📡 Active Mayo API Servers:"))
			for _, s := range servers {
				status := ui.StyleSuccess.Render("RUNNING")
				fmt.Printf("  • Port %d | Session: %s | PID: %d | Status: %s\n", s.Port, s.SessionID[:8], s.PID, status)
			}
			fmt.Println()

		case "logs":
			port := 8080
			if len(parts) >= 3 {
				port, _ = strconv.Atoi(parts[2])
			}
			path := mgr.GetLogPath(port)
			if _, err := os.Stat(path); err != nil {
				ui.PrintError(fmt.Sprintf("No logs found for port %d", port))
				return
			}
			ui.PrintInfo(fmt.Sprintf("Showing last 20 lines of logs for port %d:", port))
			cmd := exec.Command("tail", "-n", "20", path)
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr
			cmd.Run()

		case "stop":
			if len(parts) < 3 {
				ui.PrintInfo("Usage: /serve stop [port|session_id]")
				return
			}
			if err := mgr.Stop(parts[2]); err != nil {
				ui.PrintError(err.Error())
			} else {
				ui.PrintSuccess(fmt.Sprintf("Stopped server %s", parts[2]))
			}
		}

	case "/clear":
		ui.ClearScreen()
		return
	case "/help":
		helpManual := getHelpManual()

		if len(parts) > 1 {
			query := strings.ToLower(strings.Join(parts[1:], " "))

			// Simple search logic: find sections containing the query
			// We split by "###" to get command groups
			sections := strings.Split(helpManual, "###")
			var filtered []string

			// Always include the header if it matches or just to keep context
			header := sections[0]
			if strings.Contains(strings.ToLower(header), query) {
				filtered = append(filtered, header)
			}

			found := false
			for i := 1; i < len(sections); i++ {
				if strings.Contains(strings.ToLower(sections[i]), query) {
					filtered = append(filtered, "###"+sections[i])
					found = true
				}
			}

			if !found && len(filtered) <= 1 {
				ui.PrintInfo(fmt.Sprintf("No specific command sections found for '%s'. Showing any matching paragraphs...", query))
				// Fallback to paragraph search if no ### section matches
				paragraphs := strings.Split(helpManual, "\n\n")
				var paraFiltered []string
				for _, p := range paragraphs {
					if strings.Contains(strings.ToLower(p), query) {
						paraFiltered = append(paraFiltered, p)
					}
				}
				if len(paraFiltered) > 0 {
					ui.RenderMarkdown("# 🐶 Mayo Help Search: " + query + "\n\n" + strings.Join(paraFiltered, "\n\n---\n\n"))
					return
				}
				ui.PrintError(fmt.Sprintf("No help results found for '%s'.", query))
				return
			}

			ui.RenderMarkdown("# 🐶 Mayo Help Search: " + query + "\n\n" + strings.Join(filtered, "\n\n"))
			return
		}

		ui.RenderMarkdown(helpManual)
		return
	default:
		allCmds := []string{
			"/export", "/share", "/this", "/scan", "/sources", "/df", "/connect",
			"/reconcile", "/disconnect", "/knowledge", "/context", "/changelog",
			"/debug", "/setup", "/exit", "/quit", "/model", "/profile",
			"/sessions", "/session", "/describe", "/privacy", "/clear", "/help", "/serve",
		}

		bestMatch := ""
		minDist := 3 // Max distance to be considered a suggestion

		for _, ac := range allCmds {
			dist := levenshtein(cmd, ac)
			if dist < minDist {
				minDist = dist
				bestMatch = ac
			}
		}

		if bestMatch != "" {
			ui.PrintError(fmt.Sprintf("Unknown command: '%s'. Did you mean '%s'?", cmd, bestMatch))
		} else {
			ui.PrintError(fmt.Sprintf("Unknown command: '%s'. Type /help for usage.", cmd))
		}
	}
}

// Simple Levenshtein distance for command suggestions
func levenshtein(s, t string) int {
	d := make([][]int, len(s)+1)
	for i := range d {
		d[i] = make([]int, len(t)+1)
		d[i][0] = i
	}
	for j := range d[0] {
		d[0][j] = j
	}
	for i := 1; i <= len(s); i++ {
		for j := 1; j <= len(t); j++ {
			if s[i-1] == t[j-1] {
				d[i][j] = d[i-1][j-1]
			} else {
				min := d[i-1][j]
				if d[i][j-1] < min {
					min = d[i][j-1]
				}
				if d[i-1][j-1] < min {
					min = d[i-1][j-1]
				}
				d[i][j] = min + 1
			}
		}
	}
	return d[len(s)][len(t)]
}

func getHelpManual() string {
	return `
# 🐶 Mayo Manual - Senior AI Data Partner (v1.4.0)

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

- **/scan**
  Forces a manual schema re-scan and metadata enrichment for all connected sources. Use this if the database schema has changed while Mayo is running.

### 📊 Dataframes (In-Memory Processing)
Mayo can "stage" query results into memory for further AI analysis or cross-source joins.
- **/df list**
  List all persisted dataframes saved in the local SQLite storage.
- **/df load [name|knowledge:session_id]**
  Load a saved dataframe or an indexed knowledge document into the active working memory.
  - **Sample**: ` + "`/df load knowledge:abc-123`" + ` to query indexed documents as a dataset.
- **/df commit [name]**
  Persist the current working memory (result of your last query) into the local SQLite storage for future sessions.
- **/df status**
  Show the status of the current working copy (rows, columns, and if there are uncommitted changes).
- **/df reset**
  Clear the active memory and return Mayo to "Pure Database Mode".
- **/df export [name]**
  Export a saved dataframe directly to a Markdown file.
- **/enhance start**
  AI-powered data enrichment for SQLite. Add new columns based on AI analysis of existing row data. (Requires Dataframe Mode)

- **/describe [alias|df]**
  Generates a high-level statistical summary (Pandas-style) for a data source or the active dataframe.
  - **Sample**: ` + "`/describe main_db`" + ` or ` + "`/describe df`" + `

- **/reconcile <alias1> <alias2>**
  AI-powered reconciliation between two data sources. Mayo will look for mapping patterns and identify discrepancies.

### 🧠 AI Intelligence & Configuration
- **/setup**
  Interactive wizard to configure AI Providers (Gemini, OpenAI, Groq, Anthropic) and Models.

- **/model**
  Selectively change the LLM model for the current session without changing the permanent profile.
  - **Sample**: ` + "`/model`" + ` (then select from the list).

- **/profile [ai|ds]**
  Instantly switch between saved AI or Data Source profiles.

- **/context [text|file|clear]**
  Injects persistent business rules or domain knowledge.
  - **Sample**: ` + "`/context \"Always use Indonesian for final reports\"`" + `
  - **Sample**: ` + "`/context clear`" + ` to remove current context.

### 📚 Knowledge Base (RAG)
- **/knowledge [path]**
  Parses and indexes external documents (` + "`.pdf, .md, .txt`" + `) into the local vector database for semantic search.
  - **Note**: If Privacy Mode is **ON**, all PII (Emails, Phones, etc.) is masked *per-chunk* before being sent to external AI APIs for embedding generation or stored in the database.

- **/privacy**
  Toggle PII Masking. When ON, Mayo masks Emails, Phones, and Credentials in both inputs and outputs. Default is **ON**.

- **/debug**
  Toggle Debug Mode. When ON, raw LLM prompts and responses (including embeddings metadata) are displayed.

### 💾 Sessions & Research Management
- **/sessions [create|rename|clear|delete]**
  Manage your research lifecycle. Each session has its own vault, logs, and metadata.
  - **create**: Start a fresh research thread.
  - **rename**: Give a meaningful name to the current session.
  - **clear**: WIP - Delete all historical sessions.
  - **delete**: Remove a specific historical session.

- **/export [filename.md]**
  Saves the raw session history (all queries and answers) to a Markdown file.

- **/share [filename.md]**
  Uses AI to synthesize your entire session into a professional Executive Report. Useful for sending findings to stakeholders.

### 🛸 API & Utilities
### 📡 Mayo API & Serving (v1.4.0)
You can expose Mayo's brain as a REST API to be used by other applications (Dashboards, Slack bots, etc.).

- **/serve spawn [session_id] [port]** — Starts a background API server for a specific session on a specific port.
- **/serve status** — Lists all active background API servers and their ports.
- **/serve logs [port]** — Shows recent activity logs for a specific API server.
- **/serve stop [port|session_id]** — Stops a running API server.

- **mayo serve [--port 8080] [--token <api-key>] [--session <id>]** (Terminal Command)
  Starts the API server in the foreground.
  - **Endpoints**: ` + "`POST /v1/query`" + `, ` + "`GET /v1/status`" + `.
  - **Auth**: Bearer token is required if configured.

- **/this**
  A "God Mode" view of the current state: session IDs, file paths, active schemas, and PII vault stats.
- **/clear**
  Clears the terminal screen for a fresh workspace.
- **/changelog**
  View the latest updates and feature additions in Mayo.

### 🚪 Exit & Quit
- **/exit** or **/quit** - Safely terminates the CLI session.

---

## 💡 EXPERT USAGE TIPS
1. **Natural Language Queries**: Don't just ask for data. Ask for *analysis*.
   - *Example*: "Why did sales drop in Q3 based on the ` + "`orders`" + ` and ` + "`returns`" + ` tables?"
2. **Cross-Source Joins**: Connect multiple databases or files and ask questions that require joining them.
3. **Thought Blocks**: If Mayo is struggling, read the ` + "`thought`" + ` blocks to see its reasoning and correct it in your next prompt.

*Mayo by Teleskop.id*
*Status: READY | READ-ONLY: ON*
---`
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
	var pf knowledge.PrivacyFilter
	if privacy.PrivacyMode && privacy.ActiveVault != nil {
		pf = privacy.ActiveVault
	}
	if err := knowledge.IndexDocument(context.Background(), dbConn, GlobalAI, pf, doc, tableName); err != nil {
		ui.PrintError(err.Error())
	} else {
		ui.PrintSuccess(fmt.Sprintf("Knowledge Indexed into table: %s", tableName))

		// Ensure Orchestrator is ready for Pure RAG mode even if no DB is connected
		if GlobalOrchestrator == nil && GlobalAI != nil {
			GlobalOrchestrator = &ai.Orchestrator{
				AI:             GlobalAI,
				Connections:    make(map[string]*ai.DBConnection),
				Session:        GlobalSess,
				DefaultLimit:   cfg.DefaultLimit,
				Interactive:    cfg.Interactive,
				AnalystEnabled: cfg.AnalystEnabled,
				Files:          []*files.FileData{},
			}
			ui.PrintInfo("Mayo is now in Pure Knowledge (RAG) Mode. You can start asking questions!")
		}
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
