package ai

import (
	"context"
	"fmt"
	"mayo-cli/internal/ui"
	"strings"
	"github.com/AlecAivazis/survey/v2"
	"mayo-cli/internal/db"
)

func (o *Orchestrator) Reconcile(ctx context.Context, alias1, alias2 string) (string, error) {
	// 1. Validate aliases
	conn1, ok1 := o.Connections[alias1]
	conn2, ok2 := o.Connections[alias2]
	if !ok1 || !ok2 {
		return "", fmt.Errorf("both aliases '%s' and '%s' must be active", alias1, alias2)
	}

	// 2. Load schemas if not present
	if conn1.Schema == nil { o.LoadMetadata(alias1) }
	if conn2.Schema == nil { o.LoadMetadata(alias2) }

	ui.RenderStep("⚖️", fmt.Sprintf("Assessing relationship between '%s' and '%s'...", alias1, alias2))

	// Phase 1: AI assessment of relationship
	schema1 := db.ExportSchemaToMarkdown(conn1.Schema)
	schema2 := db.ExportSchemaToMarkdown(conn2.Schema)

	prompt := fmt.Sprintf(`Analyze the schemas of two data sources:
Source 1 (Alias: %s):
%s

Source 2 (Alias: %s):
%s

Additional Heuristics/Context:
%s

TASK:
1. Determine if these data sources are related (e.g., share entities like 'transactions', 'accounts', 'orders').
2. Identify the most likely join keys or matching columns.
3. Suggest which table from %s should be compared against which table from %s.

Format your response as:
REASONING: [Your analysis]
RELATED: [YES/NO]
TABLE1: [Table from %s]
TABLE2: [Table from %s]
JOIN_KEYS: [Comma separated keys, e.g. date, amount, description]`, alias1, schema1, alias2, schema2, o.UserContext, alias1, alias2, alias1, alias2)

	assessment, _, err := o.AI.GenerateResponse(ctx, "You are a senior data architect and reconciliation specialist.", prompt)
	if err != nil {
		return "", err
	}

	// Parse assessment
	related := strings.Contains(strings.ToUpper(assessment), "RELATED: YES")
	if !related {
		ui.RenderMarkdown(assessment)
		return "Data sources do not appear to be related for reconciliation.", nil
	}

	// Extract table names and keys (simple parsing)
	table1 := extractValue(assessment, "TABLE1:")
	table2 := extractValue(assessment, "TABLE2:")
	keys := extractValue(assessment, "JOIN_KEYS:")

	ui.RenderStep("🤝", fmt.Sprintf("Identified related tables: %s and %s", table1, table2))
	ui.PrintInfo(fmt.Sprintf("Suggested match keys: %s", keys))

	// Phase 2: Discrepancy detection
	ui.RenderStep("🔍", "Searching for discrepancies...")
	
	reconPrompt := fmt.Sprintf(`Create a SQL query (SQLite dialect) that reconciles Table A (%s.%s) and Table B (%s.%s).
Match them using these keys: %s.
The query should return:
1. All columns from both sides.
2. A status column 'recon_status' which is one of: 'MATCH', 'MISSING_IN_A', 'MISSING_IN_B', 'MISMATCH_VALUES'.
3. Use COALESCE where appropriate.

Note: Both databases are ATTACHED with their aliases (%s and %s) as the schema names.
TABLE A (%s) schema: %s
TABLE B (%s) schema: %s

ONLY return the SQL block.`, alias1, table1, alias2, table2, keys, alias1, alias2, alias1, schema1, alias2, schema2)

	sqlResp, _, err := o.AI.GenerateResponse(ctx, "You are a SQL expert specializing in reconciliation queries.", reconPrompt)
	if err != nil {
		return "", err
	}

	reconSQL := extractSQL(sqlResp)
	if reconSQL == "" {
		return "", fmt.Errorf("AI failed to generate reconciliation SQL")
	}

	ui.RenderSQLQuery(reconSQL)

	cols, rows, err := o.RunReconQuery(reconSQL)
	if err != nil {
		return "", fmt.Errorf("reconciliation query failed: %v", err)
	}

	ui.RenderTable(cols, rows)

	// Phase 3: Suggest Resolutions (Git-like)
	ui.RenderSeparator()
	ui.PrintInfo("--- RECONCILIATION SUMMARY ---")
	
	// Count statuses
	statusIdx := -1
	for i, c := range cols {
		if strings.ToLower(c) == "recon_status" {
			statusIdx = i
			break
		}
	}

	if statusIdx == -1 {
		return "Reconciliation query executed but 'recon_status' column not found.", nil
	}

	stats := make(map[string]int)
	mismatches := [][]string{}
	for _, row := range rows {
		s := row[statusIdx]
		stats[s]++
		if s != "MATCH" {
			mismatches = append(mismatches, row)
		}
	}

	for s, count := range stats {
		fmt.Printf("- %s: %d rows\n", s, count)
	}

	if len(mismatches) == 0 {
		ui.PrintSuccess("Perfect Tally! No discrepancies found.")
		return "Reconciliation complete: Perfect Match.", nil
	}

	// Decision Phase
	var proceed bool
	survey.AskOne(&survey.Confirm{Message: "Discrepancies found. Do you want to resolve them to create a 'Golden Record'?", Default: true}, &proceed)
	if !proceed {
		return "Reconciliation finished without resolution.", nil
	}

	// For each discrepancy type, ask for a rule
	ui.RenderStep("🤖", "Analyzing discrepancies for resolution rules...")
	
	sampleLimit := 10
	if len(mismatches) < sampleLimit {
		sampleLimit = len(mismatches)
	}
	sampleData := mismatches[:sampleLimit]

	resolutionPrompt := fmt.Sprintf(`I have the following discrepancies from reconciling %s and %s:
Samples: %v

TASK:
For each recon_status ('MISSING_IN_A', 'MISSING_IN_B', 'MISMATCH_VALUES'), suggest a "Resolution Rule".
Example: For 'MISSING_IN_B', rule could be "Accept A as truth" or "Discard row".

Format:
STATUS: [Status]
SUGGESTION: [Your suggestion]
SQL_FRAG: [SQL CASE WHEN fragment or logic to reach Golden Record]`, table1, table2, sampleData)

	resResp, _, err := o.AI.GenerateResponse(ctx, "You are a data integrity officer.", resolutionPrompt)
	if err != nil {
		return "", err
	}

	ui.RenderMarkdown(resResp)

	var confirmFinal bool
	survey.AskOne(&survey.Confirm{Message: "Do you want to apply these suggestions and create a 'Golden' dataframe?", Default: true}, &confirmFinal)
	
	if confirmFinal {
		ui.RenderStep("🏗️", "Building Golden Record dataframe...")
		goldenPrompt := fmt.Sprintf(`Using the reconciliation query results, write a final SELECT statement that produces a "Golden Record".
Apply the resolution rules suggested:
%s

Source Tables: %s, %s
Keys: %s

Ensure all columns are cleaned and resolved.
Return ONLY the SQL.`, resResp, table1, table2, keys)

		goldenResp, _, err := o.AI.GenerateResponse(ctx, "You are a SQL expert.", goldenPrompt)
		if err != nil {
			return "", err
		}
		goldenSQL := extractSQL(goldenResp)
		
		gCols, gRows, err := o.RunReconQuery(goldenSQL)
		if err != nil {
			return "", err
		}

		o.LastCols = gCols
		o.LastRows = gRows
		o.StagedName = "golden_record"
		o.IsDirty = true
		ui.RenderTable(gCols, gRows)
		ui.PrintSuccess("Golden Record created as a staged dataframe. Use '/df commit golden' to save.")
	}

	return "Reconciliation process completed.", nil
}

func extractValue(text, prefix string) string {
	lines := strings.Split(text, "\n")
	for _, l := range lines {
		trimmed := strings.TrimSpace(l)
		if strings.HasPrefix(strings.ToUpper(trimmed), prefix) {
			return strings.TrimSpace(trimmed[len(prefix):])
		}
	}
	return ""
}

func (o *Orchestrator) RunReconQuery(query string) ([]string, [][]string, error) {
	return o.ExecuteCrossQuery(query)
}
