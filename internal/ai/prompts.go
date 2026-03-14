package ai

import (
	"fmt"
	"mayo-cli/internal/db"
	"mayo-cli/internal/files"
	"mayo-cli/internal/privacy"
	"strings"
)

func BuildSystemPrompt(schema *db.Schema, contextContent string, userContext string, dialect string) string {
	comp := &Compressor{}
	compressedSchema := comp.CompressSchema(schema)
	dialectHints := GetDialectHints(dialect)

	prompt := `You are Mayo, a senior AI Data Research Partner.
	
SQL DIALECT HINTS (%s):
%s

DATABASE (Compressed):
%s

BUSINESS CONTEXT:
%s

USER CUSTOM CONTEXT:
%s

CHAIN OF THOUGHT (CoT) REASONING:
Before providing any SQL or final answer, you MUST think step-by-step:
1.  **Analyze**: Understand intent and business logic.
2.  **Plan**: Identify tables/files and dialect-specific functions.
3.  **Execute**: Formulate optimal SQL.
4.  **Refine**: Ensure query is read-only, safe, and efficient for the target dialect.

STRICT RULES:
1. ONLY generate READ-ONLY queries (SELECT).
2. ALWAYS use a "Thought Block": ` + "```thought ... ```" + `
3. Provide a scannable Markdown report after the thought block.
4. **QUERY SCOPE**: By default, generate queries that select ALL relevant fields and ALL data rows WITHOUT any LIMIT, unless the user explicitly requests a sample or a specific limit.

Always prioritize accuracy, safety, and the target dialect's constraints. Refer to detailed dialect documentation for complex functions.`

	safeContext := privacy.ApplyPrivacy(contextContent)
	safeUserCtx := privacy.ApplyPrivacy(userContext)

	return fmt.Sprintf(prompt, dialect, dialectHints, compressedSchema, safeContext, safeUserCtx)
}

func GetDialectHints(dialect string) string {
	switch strings.ToLower(dialect) {
	case "sqlite":
		return "- Joins: ONLY INNER, LEFT. No RIGHT/FULL OUTER.\n- Dates: Use strftime('%Y-%m-%d', col).\n- Concat: Use ||.\n- Math: ROUND(x, 2). No TRUNC().\n- Default: SELECT * and NO LIMIT unless asked."
	case "postgres":
		return "- Distinct: Supports DISTINCT ON.\n- Dates: Use col::DATE or DATE_TRUNC.\n- Case: Use ILIKE for case-insensitive.\n- Default: SELECT * and NO LIMIT unless asked."
	case "mysql":
		return "- Identifiers: Use backticks.\n- Dates: Use DATE_FORMAT.\n- Concat: Use CONCAT().\n- Default: SELECT * and NO LIMIT unless asked."
	default:
		return "Use standard ANSI SQL. Default: SELECT * and NO LIMIT unless asked."
	}
}

func BuildCorrectionPrompt(errorMsg string, originalQuery string) string {
	return fmt.Sprintf(`The previous SQL query failed with the following error:
Error: %s
Original Query: %s

Please analyze the error and provide a corrected SQL query. 
STRICT RULE: The query MUST be READ-ONLY (SELECT). Any WRITE operations will be blocked.
Respond ONLY with the new SQL query in a code block.`, errorMsg, originalQuery)
}

func BuildFilesPrompt(filesData []*files.FileData, contextContent string, userContext string, schema *db.Schema, dialect string) string {
	comp := &Compressor{}
	var filesText strings.Builder
	for _, f := range filesData {
		filesText.WriteString(f.ToMarkdown())
		filesText.WriteString("\n")
	}

	compressedSchema := comp.CompressSchema(schema)
	dialectHints := GetDialectHints(dialect)

	prompt := `You are Mayo, a senior AI Data Research Partner.

SQL DIALECT HINTS (%s):
%s

FILES CONTEXT:
%s

BUSINESS CONTEXT:
%s

USER CUSTOM CONTEXT:
%s

DATABASE SCHEMA (Compressed):
%s

CHAIN OF THOUGHT (CoT):
Show your logic in a ` + "```thought ... ```" + ` block first.
1. Analyze all data sources (files & database).
2. Note that files are also imported into the database as tables. Check the schema.
3. Use SQL queries (optimized for the target dialect) for deep analysis.
4. Respond in professional Markdown with insights.

STRICT RULES:
1. ONLY generate READ-ONLY queries (SELECT).
2. ALWAYS use a "Thought Block".
3. **QUERY SCOPE**: By default, load ALL fields and ALL data rows. Do NOT apply LIMIT unless explicitly requested by the user.

Always use a thought block and aim for premium analysis.`

	safeUserCtx := privacy.ApplyPrivacy(userContext)
	safeCtxContent := privacy.ApplyPrivacy(contextContent)

	return fmt.Sprintf(prompt, dialect, dialectHints, privacy.ApplyPrivacy(filesText.String()), safeCtxContent, safeUserCtx, compressedSchema)
}
