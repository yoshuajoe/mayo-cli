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

	"mayo-cli/internal/db"
	"mayo-cli/internal/dataframe"
	"mayo-cli/internal/files"
	"mayo-cli/internal/privacy"
	"mayo-cli/internal/session"
	"mayo-cli/internal/ui"
)

type DBConnection struct {
	Alias    string
	Source   string // Descriptive name (profile name, file path, etc)
	DB       *sql.DB
	Driver   string
	Schema   *db.Schema
	IsImport bool // True if it's from local files
}

type Orchestrator struct {
	AI          AIClient
	Connections map[string]*DBConnection
	Files       []*files.FileData
	Session     *session.Session
	Ctx            string
	UserContext    string
	LastResultData string // tokenized JSON for AI context
	LastCols       []string   // raw column names (working copy)
	LastRows       [][]string // raw (real) values (working copy)
	LastSQL        string     // last executed SQL
	StagedName     string     // name of loaded/staged dataframe
	IsDirty        bool       // true if working copy differs from saved state
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
		for _, conn := range o.Connections {
			if conn.Schema != nil {
				// We inform AI about the alias. AI should use alias.table if needed,
				// or we'll handle routing. For now, we give prefixed table names for clarity.
				for _, tbl := range conn.Schema.Tables {
					clonedTbl := tbl
					clonedTbl.Name = fmt.Sprintf("%s.%s", conn.Alias, tbl.Name)
					fullSchema.Tables = append(fullSchema.Tables, clonedTbl)
				}
			}
		}

		systemPrompt = BuildSystemPrompt(&fullSchema, history, o.UserContext)

			// 2. Dataframes as Tables (THE NEW WAY)
			if o.StagedName != "" {
				systemPrompt += fmt.Sprintf("\n\nDATAFRAME MODE: The active dataframe '%s' is available as a local table named 'df_%s'.", o.StagedName, o.StagedName)
				systemPrompt += "\nCRITICAL: DO NOT ask for data samples. Write standard SQL queries against 'df_" + o.StagedName + "' to analyze the data. I will execute them against the local SQLite store."
			}

		if len(o.Files) > 0 {
			ui.RenderStep("📦", fmt.Sprintf("Informing AI about %d imported files...", len(o.Files)))
			var filesNote strings.Builder
			filesNote.WriteString("\n\nIMPORTED FILES (Available as Local Database Tables):\n")
			for _, f := range o.Files {
				tableName := db.SanitizeName(filepath.Base(f.Name))
				tableName = regexp.MustCompile(`\.(csv|xlsx|xls|pdf)$`).ReplaceAllString(tableName, "")
				filesNote.WriteString(fmt.Sprintf("- Original file: '%s'\n", f.Name))
			}
			systemPrompt += filesNote.String()
		}
	} else if len(o.Files) > 0 {
		ui.RenderStep("📦", fmt.Sprintf("Bundling %d files for text-based analysis...", len(o.Files)))
		systemPrompt = BuildFilesPrompt(o.Files, history, o.UserContext, nil)
	}

	ui.RenderStep("🗜️", "Compressing prompt for token efficiency...")

	// 1. Log user input
	if o.Session != nil {
		session.LogToSession(o.Session.ID, fmt.Sprintf("User: %s", userInput))
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

	// 4. Validate and Execute SQL
	if sqlQuery != "" {
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

			ui.RenderSQLStatus("Executing generated SQL query...")
			ui.RenderSQLQuery(sqlQuery)
			o.LastSQL = sqlQuery
			o.IsDirty = true // Mark that we have uncommitted data in memory
			err = o.ExecuteAndRender(sqlQuery)
			if err != nil {
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
			}
		}
	}

	// 5. Detect and render charts
	chartData := extractChartData(aiResponse)
	if chartData != nil {
		fmt.Println(chartData())
	}

	// 6. --- NEW: ANALYST STAGE ---
	finalResponse := aiResponse
	var analysis string
	if len(o.Connections) > 0 {
		ui.RenderStep("🧠", "Analyzing data results for insights...")
		var err error
		analysis, err = o.AnalyzeResults(ctx, userInput, sqlQuery, aiResponse)
		if err == nil && analysis != "" {
			ui.RenderSeparator()
			ui.PrintInfo("Analyst Insight:")
			ui.RenderMarkdown(analysis)
			finalResponse += "\n\n### Analyst Insight\n" + analysis
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
	var lastErr error
	for alias, conn := range o.Connections {
		// Intelligent routing: if query has alias.table, strip it for the specific DB
		cleanQuery := query
		// Find all occurrences of "alias." and remove them
		pattern := fmt.Sprintf(`(?i)%s\.`, regexp.QuoteMeta(alias))
		cleanQuery = regexp.MustCompile(pattern).ReplaceAllString(query, "")

		// Detokenize any PII tokens in the query so the DB can find real values
		if privacy.ActiveVault != nil {
			cleanQuery = privacy.ActiveVault.Detokenize(cleanQuery)
		}

		rows, err := conn.DB.Query(cleanQuery)
		if err == nil {
			defer rows.Close()
			cols, err := rows.Columns()
			if err != nil {
				return err
			}

			var data [][]string
			for rows.Next() {
				vals := make([]interface{}, len(cols))
				valPtrs := make([]interface{}, len(cols))
				for i := range vals {
					valPtrs[i] = &vals[i]
				}

				if err := rows.Scan(valPtrs...); err != nil {
					return err
				}

				row := make([]string, len(cols))
				for i, v := range vals {
					if b, ok := v.([]byte); ok {
						row[i] = string(b)
					} else {
						row[i] = fmt.Sprintf("%v", v)
					}
				}
				data = append(data, row)
			}
			
			// Save raw results for dataframe feature
			o.LastCols = cols
			o.LastRows = data

			// Build CSV-like format instead of JSON (Much more token efficient)
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
		lastErr = err
	}
	return lastErr
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

func validateReadOnly(query string) error {
	forbidden := []string{"INSERT", "UPDATE", "DELETE", "DROP", "TRUNCATE", "ALTER", "CREATE", "REPLACE", "GRANT", "REVOKE"}
	for _, word := range forbidden {
		re := regexp.MustCompile(`(?i)\b` + word + `\b`)
		if re.MatchString(query) {
			return fmt.Errorf("security violation: non-read-only operation '%s' detected. InsightCLI is restricted to READ-ONLY mode", word)
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
