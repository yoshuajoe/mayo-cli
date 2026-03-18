package ai

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/AlecAivazis/survey/v2"
	"github.com/blastrain/vitess-sqlparser/sqlparser"
	_ "github.com/mattn/go-sqlite3"
	"mayo-cli/internal/config"
	"mayo-cli/internal/dataframe"
	"mayo-cli/internal/db"
	"mayo-cli/internal/files"
	"mayo-cli/internal/knowledge"
	"mayo-cli/internal/privacy"
	"mayo-cli/internal/session"
	"mayo-cli/internal/ui"
	"os"
)

type DBConnection struct {
	Alias    string
	Source   string // Descriptive name (profile name, file path, etc)
	DSN      string // Actual connection string / path
	DB       *sql.DB
	Driver   string
	Schema   *db.Schema
	IsImport bool // True if it's from local files
}

type Orchestrator struct {
	AI             AIClient
	Connections    map[string]*DBConnection
	Files          []*files.FileData
	Session        *session.Session
	Ctx            string
	UserContext    string
	LastResultData string     // tokenized JSON for AI context
	LastCols       []string   // raw column names (working copy)
	LastRows       [][]string // raw (real) values (working copy)
	LastSQL        string     // last executed SQL
	StagedName     string     // name of loaded/staged dataframe
	IsDirty        bool       // true if working copy differs from saved state
	DefaultLimit   int        // Point 1.C
	Interactive    bool       // Point 2.B
	AnalystEnabled bool
}

func (o *Orchestrator) ProcessQuery(ctx context.Context, userInput string) (string, error) {
	ui.RenderStep("🔍", "Analyzing intent and preparing context...")

	var systemPrompt string
	history := ""
	comp := &Compressor{}

	if o.Session != nil {
		logs, err := session.ReadSessionLogs(o.Session.ID)
		if err == nil && logs != "" {
			// Compress history to keep tokens low
			history = comp.CompressContext(logs)
		}
	}

	if len(o.Connections) > 0 {
		ui.RenderStep("🗺️", fmt.Sprintf("Building system prompt from %d database connections...", len(o.Connections)))

		var fullSchema db.Schema
		for alias, conn := range o.Connections {
			// Try to load metadata if not present
			if conn.Schema == nil {
				o.LoadMetadata(alias)
			}
			if conn.Schema != nil {
				// We inform AI about the alias. AI should use alias.table if needed,
				// or we'll handle routing. For now, we give prefixed table names for clarity.
				for _, tbl := range conn.Schema.Tables {
					clonedTbl := tbl
					if tbl.SchemaName != "" {
						clonedTbl.Name = fmt.Sprintf("%s.%s.%s", conn.Alias, tbl.SchemaName, tbl.Name)
					} else {
						clonedTbl.Name = fmt.Sprintf("%s.%s", conn.Alias, tbl.Name)
					}
					fullSchema.Tables = append(fullSchema.Tables, clonedTbl)
				}
			}
		}

		// 🆕 DATAFRAME SCHEMA INTEGRATION
		if o.StagedName != "" {
			// Fetch fresh column list from local store
			cols, _, err := dataframe.Load(o.StagedName)
			if err == nil {
				o.LastCols = cols // Sync internal state
				dfTable := db.Table{
					Name: "df_" + o.StagedName,
				}
				for _, c := range cols {
					dfTable.Columns = append(dfTable.Columns, db.Column{Name: c, Type: "TEXT"})
				}
				fullSchema.Tables = append(fullSchema.Tables, dfTable)
			}
		}

		dialect := "ansi"
		if o.StagedName != "" {
			dialect = "sqlite"
		} else if len(o.Connections) > 0 {
			// Typical use case: all connections usually share a target driver or we pick the first
			for _, conn := range o.Connections {
				dialect = conn.Driver
				break
			}
		}

		systemPrompt = BuildSystemPrompt(&fullSchema, history, o.UserContext, dialect)

		if o.DefaultLimit > 0 {
			systemPrompt += fmt.Sprintf("\n\nCRITICAL: By default, you MUST apply a LIMIT %d to your queries to protect against high token usage for large data loads. ONLY omit or increase the LIMIT if the user explicitly asks for 'all' or a larger number of rows.", o.DefaultLimit)
		} else {
			systemPrompt += "\n\nCRITICAL: There is NO default limit for queries. You MUST prioritize selecting ALL data unless the user asks for a sample."
		}

		// 2. Dataframes as Tables (THE NEW WAY)
		if o.StagedName != "" {
			systemPrompt += fmt.Sprintf("\n\nDATAFRAME MODE: The active dataframe '%s' is available as a local table named 'df_%s'.", o.StagedName, o.StagedName)
			systemPrompt += "\nCRITICAL: DO NOT ask for data samples. Write standard SQL queries against 'df_" + o.StagedName + "' to analyze the data. I will execute them against the local SQLite store."
		}

		if len(o.Connections) > 1 {
			systemPrompt += "\n\nCRITICAL: MULTIPLE DATA SOURCES DETECTED. You can join across them by using the format 'alias.table_name' in your SQL query. If they are from the same driver, I will handle the routing. If they are from different drivers, prioritize analyzing them sequentially unless they are imported files (which all live in the same SQLite engine)."
		}
		if len(o.Files) > 0 {
			ui.RenderStep("📦", fmt.Sprintf("Informing AI about %d imported files...", len(o.Files)))
			var filesNote strings.Builder
			filesNote.WriteString("\n\nIMPORTED FILES (Available as Local Database Tables):\n")
			for _, f := range o.Files {
				tableName := db.SanitizeName(filepath.Base(f.Name))
				tableName = regexp.MustCompile(`\.(csv|xlsx|xls|pdf)$`).ReplaceAllString(tableName, "")
				filesNote.WriteString(fmt.Sprintf("- File: '%s' -> Table Name: '%s'\n", f.Name, tableName))
			}
			systemPrompt += filesNote.String()
		}
	} else if len(o.Files) > 0 {
		ui.RenderStep("📦", fmt.Sprintf("Bundling %d files for text-based analysis...", len(o.Files)))
		systemPrompt = BuildFilesPrompt(o.Files, history, o.UserContext, nil, "sqlite")
	}

	// --- 3.C KNOWLEDGE INTEGRATION (RAG) ---
	if o.Session != nil {
		knowledgeTableName := "vector_" + strings.ReplaceAll(o.Session.ID, "-", "_")
		sqlitePath := filepath.Join(config.GetConfigDir(), "data", "vectors.db")
		if _, err := os.Stat(sqlitePath); err == nil {
			if kb, err := sql.Open("sqlite3", sqlitePath); err == nil {
				defer kb.Close()
				// Use user input as search query
				results, _ := knowledge.SearchKnowledge(kb, knowledgeTableName, userInput, 5)
				if len(results) > 0 {
					ui.RenderStep("📚", fmt.Sprintf("Retrieved %d relevant knowledge snippets...", len(results)))
					kbCtx := "\n\nRELEVANT KNOWLEDGE CONTEXT (from indexed documents):\n"
					kbCtx += strings.Join(results, "\n---\n")
					systemPrompt += kbCtx
				}
			}
		}
	}

	ui.RenderStep("🗜️", "Compressing prompt for token efficiency...")

	// 1. Log user input
	if o.Session != nil {
		session.LogToSession(o.Session.ID, fmt.Sprintf("User: %s", userInput))
	}

	trimmedInput := strings.TrimSpace(userInput)
	if isLikelySQL(trimmedInput) {
		ui.RenderStep("⚡", "Direct SQL detected. Skipping AI analysis...")
		return o.executeAndAnalyze(ctx, userInput, trimmedInput, "", "", nil)
	}

	// 2. Generate SQL from AI
	if o.AI == nil {
		return "", fmt.Errorf("AI client not initialized. Please run /setup to configure your AI profile.")
	}
	ui.RenderStep("🤖", "Requesting LLM analysis...")

	// Tokenize user input before sending to AI (PII protection)
	safeUserInput := userInput
	if privacy.ActiveVault != nil && privacy.PrivacyMode {
		ui.RenderStep("🔐", "Tokenizing PII in user input...")
		safeUserInput = privacy.ActiveVault.Tokenize(userInput)
	}

	if ui.DebugEnabled {
		ui.RenderDebug("SYSTEM PROMPT", systemPrompt)
		ui.RenderDebug("USER PROMPT (TOKENIZED)", safeUserInput)
	}

	aiResponse, usage, err := o.AI.GenerateResponse(ctx, systemPrompt, safeUserInput)
	if err != nil {
		return "", fmt.Errorf("AI error: %v", err)
	}

	if ui.DebugEnabled {
		ui.RenderDebugLLM("RAW LLM RESPONSE", aiResponse)
	}

	// Detokenize AI response back to original values for user display
	if privacy.ActiveVault != nil && privacy.PrivacyMode {
		aiResponse = privacy.ActiveVault.Detokenize(aiResponse)
	}

	sqlQuery := extractSQL(aiResponse)
	if sqlQuery == "" {
		// AI might be just talking
		if o.Session != nil {
			session.LogToSession(o.Session.ID, fmt.Sprintf("Mayo: %s", aiResponse))
			o.ensureSessionSummary(ctx, userInput)
		}
		ui.RenderMarkdown(aiResponse)
		if usage != nil {
			ui.RenderUsage(usage.PromptTokens, usage.CompletionTokens, usage.TotalTokens)
		}
		return aiResponse, nil
	}

	// 3. Render AI Response (Thought -> Conversation -> SQL)
	renderAIResponse(aiResponse)

	// 3.1 Check if AI provided a Data Transformation (JSON block)
	jsonData := extractJSON(aiResponse)
	if jsonData != "" {
		var records []map[string]interface{}
		if err := json.Unmarshal([]byte(jsonData), &records); err == nil && len(records) > 0 {
			ui.RenderStep("🔄", "Updating working copy with AI-transformed data...")

			// Extract columns and rows
			newCols := []string{}
			// Use the keys from the first record as columns
			for k := range records[0] {
				newCols = append(newCols, k)
			}
			sort.Strings(newCols)

			newRows := [][]string{}
			for _, rec := range records {
				row := make([]string, len(newCols))
				for i, col := range newCols {
					row[i] = fmt.Sprintf("%v", rec[col])
				}
				newRows = append(newRows, row)
			}

			o.LastCols = newCols
			o.LastRows = newRows
			o.IsDirty = true
			o.LastResultData = fmt.Sprintf("Columns: %s\nRows: %s", strings.Join(newCols, ", "), jsonData)

			ui.PrintSuccess("Working copy updated. Use '/df commit' to save permanently.")
		}
	}

	return o.executeAndAnalyze(ctx, userInput, sqlQuery, aiResponse, systemPrompt, usage)
}

func (o *Orchestrator) executeAndAnalyze(ctx context.Context, userInput, sqlQuery, aiResponse, systemPrompt string, usage *TokenUsage) (string, error) {
	// 4. Validate and Execute SQL
	if sqlQuery != "" {
		// --- INTERACTIVE REVIEW (Point 2.B) ---
		if o.Interactive {
			fmt.Println(ui.StyleTitle.Render("\n[QUERY REVIEW]"))
			var finalSQL string
			err := survey.AskOne(&survey.Input{
				Message: "Review/Edit SQL:",
				Default: sqlQuery,
			}, &finalSQL)
			if err != nil || strings.TrimSpace(finalSQL) == "" {
				ui.PrintInfo("Execution cancelled.")
				return aiResponse, nil
			}
			sqlQuery = strings.TrimSpace(finalSQL)
		}

		// --- SMART ROUTING ---
		// If query targets a dataframe (prefixed with df_ or matches staged name), execute against local storage
		isDfQuery := o.StagedName != "" && (strings.Contains(strings.ToLower(sqlQuery), "df_") || strings.Contains(strings.ToLower(sqlQuery), strings.ToLower(o.StagedName)))

		if isDfQuery {
			ui.RenderSQLStatus("Executing query against local Dataframe engine...")
			ui.RenderSQLQuery(sqlQuery)

			cols, rows, err := dataframe.Query(sqlQuery)
			if err != nil {
				return "", fmt.Errorf("local engine error: %v", err)
			}
			o.LastCols = cols
			o.LastRows = rows
			o.IsDirty = true
			ui.RenderTable(cols, rows)
			// Proceed to analysis
		} else {
			ui.RenderSQLStatus("Validating query safety...")
			if err := validateReadOnly(sqlQuery); err != nil {
				return "", err
			}

			ui.RenderSQLStatus("Executing SQL query...")
			ui.RenderSQLQuery(sqlQuery)
			o.LastSQL = sqlQuery
			o.IsDirty = true // Mark that we have uncommitted data in memory
			err := o.ExecuteAndRender(sqlQuery)
			if err != nil && systemPrompt != "" {
				fmt.Printf("⚠️ Query failed, attempting self-correction...\n")
				correctionPrompt := BuildCorrectionPrompt(err.Error(), sqlQuery)
				correctedResponse, _, errCorr := o.AI.GenerateResponse(ctx, systemPrompt, correctionPrompt)
				if errCorr != nil {
					return "", fmt.Errorf("correction failed: %v", errCorr)
				}
				sqlQuery = extractSQL(correctedResponse)
				if sqlQuery != "" {
					ui.RenderSQLStatus("Executing corrected query...")
					ui.RenderSQLQuery(sqlQuery)
					err = o.ExecuteAndRender(sqlQuery)
					if err != nil {
						return "", fmt.Errorf("SQL error after correction: %v", err)
					}
				}
			} else if err != nil {
				return "", fmt.Errorf("SQL error: %v", err)
			}
		}
	}

	// 5. Detect and render charts
	if aiResponse != "" {
		chartData := extractChartData(aiResponse)
		if chartData != nil {
			fmt.Println(chartData())
		}
	}

	// 6. --- NEW: ANALYST STAGE ---
	finalResponse := aiResponse
	var analysis string
	if len(o.Connections) > 0 && o.AnalystEnabled {
		ui.RenderStep("🧠", "Analyzing data results for insights...")
		var err error
		analysis, err = o.AnalyzeResults(ctx, userInput, sqlQuery, aiResponse)
		if err == nil && analysis != "" {
			ui.RenderSeparator()
			ui.PrintInfo("Analyst Insight:")
			ui.RenderMarkdown(analysis)
			if finalResponse != "" {
				finalResponse += "\n\n### Analyst Insight\n" + analysis
			} else {
				finalResponse = "### Analyst Insight\n" + analysis
			}
		}
	}

	if o.Session != nil {
		// Log the SQL and a compact result summary so future context has the full picture
		resultSummary := ""
		if o.LastResultData != "" {
			// Truncate JSON to a concise summary for logs to save tokens
			s := o.LastResultData
			if len(s) > 800 {
				s = s[:800] + "...]"
			}
			resultSummary = fmt.Sprintf(" Result: %s", s)
		}
		session.LogToSession(o.Session.ID, fmt.Sprintf("Mayo SQL: %s%s", sqlQuery, resultSummary))
		if analysis != "" {
			session.LogToSession(o.Session.ID, fmt.Sprintf("Analyst Insight: %s", analysis))
		}
		o.ensureSessionSummary(ctx, userInput)
	}

	if usage != nil {
		ui.RenderUsage(usage.PromptTokens, usage.CompletionTokens, usage.TotalTokens)
	}

	return finalResponse, nil
}

func (o *Orchestrator) AnalyzeResults(ctx context.Context, originalQuery, sql, aiPrompt string) (string, error) {
	// 1. Get the actual data from the last execution (we'll need a way to capture it)
	// For now, let's assume we capture it during ExecuteAndRender
	// Let's modify ExecuteAndRender to return the data as a string for analysis
	dataString := o.LastResultData

	if dataString == "" || dataString == "[]" {
		return "No data found to analyze.", nil
	}

	prompt := fmt.Sprintf(`You are the Analyst Stage of Mayo. 
User original request: "%s"
SQL Executed: %s
Data Results (JSON-ish):
%s

Your task:
1. Briefly interpret these results.
2. Note any interesting patterns, anomalies, or summaries.
3. Be professional and concise.
Respond in Markdown.`, originalQuery, sql, dataString)

	analysis, _, err := o.AI.GenerateResponse(ctx, "You are a senior data analyst. Provide insights based on query results.", prompt)
	if err != nil {
		return "", err
	}

	// Clean up thought blocks from analysis
	analysis = regexp.MustCompile("(?is)<think>.*?</think>").ReplaceAllString(analysis, "")
	analysis = regexp.MustCompile("(?is)<thought>.*?</thought>").ReplaceAllString(analysis, "")

	return strings.TrimSpace(analysis), nil
}

func (o *Orchestrator) ensureSessionSummary(ctx context.Context, firstInput string) {
	if o.Session == nil || o.Session.Summary != "New Research Session" {
		return
	}

	summaryPrompt := fmt.Sprintf("Briefly summarize this user request in exactly 3 to 4 words. Respond ONLY with the 3-4 words. Request: \"%s\"", firstInput)
	summary, _, err := o.AI.GenerateResponse(ctx, "You are a concise summarizer. Respond ONLY with the summary words, no explanations, no thought blocks.", summaryPrompt)
	if err == nil {
		// Clean the summary
		// 1. Remove all common thought/reasoning tags (case-insensitive)
		summary = regexp.MustCompile("(?is)<think>.*?</think>").ReplaceAllString(summary, "")
		summary = regexp.MustCompile("(?is)<thought>.*?</thought>").ReplaceAllString(summary, "")
		summary = regexp.MustCompile("(?is)```thought.*?```").ReplaceAllString(summary, "")

		// 2. Remove markdown code blocks if any
		summary = regexp.MustCompile("(?s)```.*?```").ReplaceAllString(summary, "")

		// 3. If there are still tags like <think> or <thought> without closures, strip them and everything после
		summary = regexp.MustCompile("(?is)<(think|thought)>.*").ReplaceAllString(summary, "")

		// 4. Clean whitespace and newlines
		summary = strings.TrimSpace(summary)
		summary = strings.ReplaceAll(summary, "\n", " ")
		summary = strings.ReplaceAll(summary, "\r", "")
		summary = strings.ReplaceAll(summary, "\"", "")

		// 5. Final check: some models might output "Summary: word1 word2 word3"
		summary = regexp.MustCompile("(?i)^Summary:").ReplaceAllString(summary, "")
		summary = strings.TrimSpace(summary)

		// Final safety check: if it's still too long, truncate it
		if len(summary) > 40 {
			summary = summary[:37] + "..."
		}

		if summary != "" {
			session.UpdateSessionSummary(o.Session.ID, summary)
			o.Session.Summary = summary // Update local copy
		}
	}
}

func renderAIResponse(response string) {
	// Extract thought block if exists
	thoughtRegex := regexp.MustCompile("(?s)```thought(.*?)```")
	thoughtMatch := thoughtRegex.FindStringSubmatch(response)
	if len(thoughtMatch) > 1 {
		ui.RenderThought(strings.TrimSpace(thoughtMatch[1]))
		// Remove thought block from main response to avoid double rendering
		response = thoughtRegex.ReplaceAllString(response, "")
	}

	// Extract SQL block if exists
	sqlRegex := regexp.MustCompile("(?s)```sql(.*?)```")
	sqlMatch := sqlRegex.FindStringSubmatch(response)

	mainText := sqlRegex.ReplaceAllString(response, "")
	if strings.TrimSpace(mainText) != "" {
		ui.RenderMarkdown(mainText)
	}

	if len(sqlMatch) > 1 {
		// We don't render SQL here, it's rendered by SQL Status
	}
}

func extractChartData(text string) func() string {
	// Look for [CHART: title, 1.2, 3.4, ...]
	re := regexp.MustCompile(`\[CHART: (.*?), ([\d\., ]+)\]`)
	match := re.FindStringSubmatch(text)
	if len(match) > 2 {
		title := match[1]
		numsStr := strings.Split(match[2], ",")
		var data []float64
		for _, s := range numsStr {
			var val float64
			fmt.Sscanf(strings.TrimSpace(s), "%f", &val)
			data = append(data, val)
		}
		return func() string {
			return ui.RenderChart(data, title)
		}
	}
	return nil
}

func (o *Orchestrator) ExecuteAndRender(query string) error {
	// Detokenize any PII tokens in the query so the DB can find real values
	cleanQuery := query
	if privacy.ActiveVault != nil {
		cleanQuery = privacy.ActiveVault.Detokenize(query)
	}

	cols, data, err := o.ExecuteCrossQuery(cleanQuery)
	if err != nil {
		return err
	}

	// Save raw results for dataframe feature
	o.LastCols = cols
	o.LastRows = data

	// Build CSV-like format for AI context (Much more token efficient)
	var sb strings.Builder
	sb.WriteString(strings.Join(cols, "|") + "\n")

	sampleSize := len(data)
	if sampleSize > 20 {
		sampleSize = 20
	}
	for i := 0; i < sampleSize; i++ {
		row := data[i]
		for j, val := range row {
			cellVal := val
			if privacy.ActiveVault != nil && privacy.PrivacyMode {
				cellVal = privacy.ActiveVault.Tokenize(cellVal)
			}
			sb.WriteString(cellVal)
			if j < len(row)-1 {
				sb.WriteString("|")
			}
		}
		sb.WriteString("\n")
	}

	o.LastResultData = fmt.Sprintf("Format: CSV (Pipe Separated)\nData (showing 20 of %d rows):\n%s",
		len(data), sb.String())

	ui.RenderTable(cols, data)
	return nil
}

func extractSQL(text string) string {
	re := regexp.MustCompile("(?s)```sql\n?(.*?)\n?```")
	match := re.FindStringSubmatch(text)
	if len(match) > 1 {
		return strings.TrimSpace(match[1])
	}
	return ""
}

func extractJSON(text string) string {
	re := regexp.MustCompile("(?s)```json\n?(.*?)\n?```")
	match := re.FindStringSubmatch(text)
	if len(match) > 1 {
		return strings.TrimSpace(match[1])
	}
	return ""
}

func (o *Orchestrator) ExecuteCrossQuery(query string) ([]string, [][]string, error) {
	// 1. Identify involved aliases and their tables
	aliasTables := make(map[string][]string)

	// Regex matches: alias.table, "alias"."table", [alias].[table]
	re := regexp.MustCompile(`([a-zA-Z0-9_]+)\.([a-zA-Z0-9_]+)`)
	matches := re.FindAllStringSubmatch(query, -1)

	involvedAliases := make(map[string]bool)
	for _, m := range matches {
		alias := m[1]
		table := m[2]
		if _, ok := o.Connections[alias]; ok {
			involvedAliases[alias] = true
			found := false
			for _, t := range aliasTables[alias] {
				if t == table {
					found = true
					break
				}
			}
			if !found {
				aliasTables[alias] = append(aliasTables[alias], table)
			}
		}
	}

	// 2. If no aliases or only one source is referenced, try direct execution
	if len(involvedAliases) <= 1 {
		for alias := range involvedAliases {
			conn := o.Connections[alias]
			cleanQuery := query
			// Strip alias if it was used in the query
			pattern := fmt.Sprintf(`(?i)%s\.`, regexp.QuoteMeta(alias))
			cleanQuery = regexp.MustCompile(pattern).ReplaceAllString(query, "")
			return o.executeOnConnection(conn, cleanQuery)
		}

		// Fallback for queries without explicit aliases: try active connections
		for _, conn := range o.Connections {
			cols, data, err := o.executeOnConnection(conn, query)
			if err == nil {
				return cols, data, nil
			}
		}

		// Try dataframe engine as last resort
		return dataframe.Query(query)
	}

	// 3. MULTI-SOURCE JOIN (The "Bridge" Feature - ROADMAP Point 3.A)
	ui.RenderStep("🔗", fmt.Sprintf("Activating Bridge: Joining %d data sources...", len(involvedAliases)))

	// Create an in-memory master DB for the join
	masterDB, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		return nil, nil, err
	}
	defer masterDB.Close()

	updatedQuery := query

	for alias := range involvedAliases {
		conn := o.Connections[alias]

		if conn.Driver == "sqlite" {
			ui.RenderStep("📎", fmt.Sprintf("Attaching local source '%s'...", alias))
			// Absolute path is needed for ATTACH
			absPath, _ := filepath.Abs(conn.DSN)
			attachSQL := fmt.Sprintf("ATTACH DATABASE '%s' AS %s", absPath, alias)
			if _, err := masterDB.Exec(attachSQL); err != nil {
				return nil, nil, fmt.Errorf("failed to attach %s: %v", alias, err)
			}
		} else {
			// External source (Postgres, MySQL, etc)
			for _, table := range aliasTables[alias] {
				ui.RenderStep("📥", fmt.Sprintf("Bridging '%s.%s' from %s...", alias, table, conn.Driver))

				// Fetch data with safety limit
				fetchSQL := fmt.Sprintf("SELECT * FROM %s", table)
				if o.DefaultLimit > 0 {
					fetchSQL += fmt.Sprintf(" LIMIT %d", o.DefaultLimit)
				}

				rows, err := conn.DB.Query(fetchSQL)
				if err != nil {
					return nil, nil, fmt.Errorf("failed to fetch from %s.%s: %v", alias, table, err)
				}
				defer rows.Close()

				// Import into master SQLite as "alias_table"
				tempTableName := fmt.Sprintf("%s_%s", alias, table)
				if err := db.ImportRowsToSQLite(masterDB, tempTableName, rows); err != nil {
					return nil, nil, fmt.Errorf("bridge import failed for %s.%s: %v", alias, table, err)
				}

				// Update query to reference the new local table name
				pattern := fmt.Sprintf(`(?i)%s\.%s`, regexp.QuoteMeta(alias), regexp.QuoteMeta(table))
				updatedQuery = regexp.MustCompile(pattern).ReplaceAllString(updatedQuery, tempTableName)
			}
		}
	}

	if ui.DebugEnabled {
		ui.RenderDebug("BRIDGE QUERY", updatedQuery)
	}

	return o.executeOnDB(masterDB, updatedQuery)
}

func (o *Orchestrator) executeOnConnection(conn *DBConnection, query string) ([]string, [][]string, error) {
	return o.executeOnDB(conn.DB, query)
}

func (o *Orchestrator) executeOnDB(db *sql.DB, query string) ([]string, [][]string, error) {
	rows, err := db.Query(query)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()

	cols, _ := rows.Columns()
	var data [][]string
	for rows.Next() {
		vals := make([]interface{}, len(cols))
		ptr := make([]interface{}, len(cols))
		for i := range vals {
			ptr[i] = &vals[i]
		}
		if err := rows.Scan(ptr...); err != nil {
			return nil, nil, err
		}
		row := make([]string, len(cols))
		for i, v := range vals {
			if v == nil {
				row[i] = ""
			} else {
				row[i] = fmt.Sprintf("%v", v)
			}
		}
		data = append(data, row)
	}
	return cols, data, nil
}

func validateReadOnly(query string) error {
	// Attempt formal parsing (Point 1.A)
	stmt, err := sqlparser.Parse(query)
	if err != nil {
		// Fallback for dialect-specific queries or complex joins that the parser doesn't understand
		return basicSecurityCheck(query)
	}

	switch stmt.(type) {
	case *sqlparser.Select, *sqlparser.Show, *sqlparser.OtherRead:
		return nil
	default:
		return fmt.Errorf("security violation: non-read-only operation detected. Mayo is restricted to READ-ONLY mode")
	}
}

func basicSecurityCheck(query string) error {
	forbidden := []string{"INSERT", "UPDATE", "DELETE", "DROP", "TRUNCATE", "ALTER", "CREATE", "REPLACE", "GRANT", "REVOKE"}
	for _, word := range forbidden {
		re := regexp.MustCompile(`(?i)\b` + word + `\b`)
		if re.MatchString(query) {
			return fmt.Errorf("security violation: potentially destructive operation '%s' detected. Mayo is restricted to READ-ONLY mode", word)
		}
	}
	return nil
}

func (o *Orchestrator) GenerateReport(ctx context.Context) (string, error) {
	if o.Session == nil {
		return "", fmt.Errorf("no active session")
	}
	logs, err := session.ReadSessionLogs(o.Session.ID)
	if err != nil || logs == "" {
		return "", fmt.Errorf("no research logs found for this session")
	}

	ui.RenderStep("📝", "Synthesizing session logs into a comprehensive report...")

	prompt := fmt.Sprintf(`You are Mayo's Senior Report Generator. 
Based on the following research session logs, generate a comprehensive executive summary report.
The report should include:
1. Executive Summary: What was searched for and what was found.
2. Key Insights: Bulleted list of the most important takeaways.
3. Data Evidence: Mention specific SQL queries or results that back up the insights.
4. Recommendations: Next steps based on the data.

FORMAT: Use professional Markdown. Be concise but impactful.

SESSION LOGS:
%s`, logs)

	report, _, err := o.AI.GenerateResponse(ctx, "You are a senior professional researcher and report writer. Generate a high-quality summary report.", prompt)
	if err != nil {
		return "", err
	}

	// Clean up thought blocks
	report = regexp.MustCompile("(?is)<think>.*?</think>").ReplaceAllString(report, "")
	report = regexp.MustCompile("(?is)<thought>.*?</thought>").ReplaceAllString(report, "")

	return strings.TrimSpace(report), nil
}

func (o *Orchestrator) SyncSchema(ctx context.Context) error {
	if len(o.Connections) == 0 {
		return fmt.Errorf("no active connections to scan")
	}

	for alias, conn := range o.Connections {
		ui.RenderStep("🔍", fmt.Sprintf("Scanning and enriching schema for '%s'...", alias))
		schema, err := db.ScanAndEnrichSchema(ctx, conn.DB, conn.Driver)
		if err != nil {
			return err
		}

		// If we already have descriptions (from LoadMetadata), preserve them
		if conn.Schema != nil {
			for _, oldTbl := range conn.Schema.Tables {
				for i := range schema.Tables {
					if schema.Tables[i].Name == oldTbl.Name {
						schema.Tables[i].Description = oldTbl.Description
						// Preserve column descriptions too
						for _, oldCol := range oldTbl.Columns {
							for j := range schema.Tables[i].Columns {
								if schema.Tables[i].Columns[j].Name == oldCol.Name {
									schema.Tables[i].Columns[j].Description = oldCol.Description
								}
							}
						}
					}
				}
			}
		}

		conn.Schema = schema
		o.SaveMetadata(alias)
	}
	return nil
}

func (o *Orchestrator) SaveMetadata(alias string) error {
	if o.Session == nil {
		return nil
	}
	conn, ok := o.Connections[alias]
	if !ok || conn.Schema == nil {
		return nil
	}

	sessionDir := filepath.Join(config.GetConfigDir(), "sessions", o.Session.ID)
	os.MkdirAll(sessionDir, 0755)
	metadataPath := filepath.Join(sessionDir, fmt.Sprintf("metadata_%s.md", alias))

	md := db.ExportSchemaToMarkdown(conn.Schema)
	err := os.WriteFile(metadataPath, []byte(md), 0644)
	if err == nil {
		ui.PrintSuccess(fmt.Sprintf("Metadata for '%s' synchronized to: %s", alias, metadataPath))
	}
	return err
}

func (o *Orchestrator) LoadMetadata(alias string) {
	if o.Session == nil {
		return
	}
	sessionDir := filepath.Join(config.GetConfigDir(), "sessions", o.Session.ID)
	metadataPath := filepath.Join(sessionDir, fmt.Sprintf("metadata_%s.md", alias))

	if data, err := os.ReadFile(metadataPath); err == nil {
		schema, err := db.ParseSchemaFromMarkdown(string(data))
		if err == nil && schema != nil {
			if conn, ok := o.Connections[alias]; ok {
				if conn.Schema == nil {
					conn.Schema = schema
				} else {
					// Merge descriptions into existing schema
					for _, parsedTbl := range schema.Tables {
						for i := range conn.Schema.Tables {
							if conn.Schema.Tables[i].Name == parsedTbl.Name {
								conn.Schema.Tables[i].Description = parsedTbl.Description
								// Merge column descriptions
								for _, parsedCol := range parsedTbl.Columns {
									for j := range conn.Schema.Tables[i].Columns {
										if conn.Schema.Tables[i].Columns[j].Name == parsedCol.Name {
											conn.Schema.Tables[i].Columns[j].Description = parsedCol.Description
										}
									}
								}
							}
						}
					}
				}
			}
		}
	}
}
func (o *Orchestrator) Describe(ctx context.Context, target string) (string, error) {
	// 1. Identify Target (DS alias or "df")
	if target == "df" || target == "" {
		if o.StagedName == "" && len(o.LastRows) == 0 {
			return "", fmt.Errorf("no active dataframe to describe")
		}
		ui.RenderStep("📊", fmt.Sprintf("Generating descriptive statistics for active dataframe '%s'...", o.StagedName))

		// Get basic stats from LastRows
		rowCount := len(o.LastRows)

		sampleData := o.LastResultData
		if len(sampleData) > 2000 {
			sampleData = sampleData[:2000] + "..."
		}

		describePrompt := fmt.Sprintf(`You are Mayo. Provide a Pandas-style "describe()" summary for the following dataframe:
Name: %s
Total Rows: %d
Columns: %s
Sample Data:
%s

TASK: 
Provide a markdown table showing statistics (Count, Mean, Std, Min, Max, etc.) for numeric columns, and unique/top/freq for categorical columns. 
If numeric data isn't obvious, use your best judgment. Be professional and helpful.`, o.StagedName, rowCount, strings.Join(o.LastCols, ", "), sampleData)

		resp, _, err := o.AI.GenerateResponse(ctx, "You are a senior data scientist specializing in statistical summaries.", describePrompt)
		if err != nil {
			return "", err
		}
		return resp, nil
	}

	// 2. Identify if target is a connection alias
	if conn, ok := o.Connections[target]; ok {
		ui.RenderStep("📡", fmt.Sprintf("Describing data source '%s'...", target))
		if conn.Schema == nil {
			o.SyncSchema(ctx)
		}

		schemaMD := db.ExportSchemaToMarkdown(conn.Schema)
		describePrompt := fmt.Sprintf(`You are Mayo. Provide a summary of this data source:
Alias: %s
Driver: %s
Schema Metadata:
%s

TASK:
1. Summarize the content of this data source concisely.
2. List the most important tables and their row counts.
3. Identify potential "key" tables for joining.
4. Format as professional Markdown.`, target, conn.Driver, schemaMD)

		resp, _, err := o.AI.GenerateResponse(ctx, "You are a senior database architect and analyst.", describePrompt)
		if err != nil {
			return "", err
		}
		return resp, nil
	}

	return "", fmt.Errorf("target '%s' not found. Use a connection alias or 'df'", target)
}

func isLikelySQL(input string) bool {
	upper := strings.ToUpper(strings.TrimSpace(input))
	// Basic keywords that start a direct query
	keywords := []string{"SELECT", "WITH", "SHOW", "DESCRIBE", "EXPLAIN", "PRAGMA"}

	isKeywordStart := false
	var matchedKey string
	for _, k := range keywords {
		if strings.HasPrefix(upper, k+" ") || upper == k {
			isKeywordStart = true
			matchedKey = k
			break
		}
	}

	if !isKeywordStart {
		return false
	}

	// Heuristic: If it's a SELECT, it should look structured (have FROM, or end with ;)
	if matchedKey == "SELECT" {
		return strings.Contains(upper, " FROM ") || strings.HasSuffix(upper, ";")
	}

	return true // More lenient for PRAGMA, SHOW, etc.
}
