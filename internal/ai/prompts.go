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

DATABASE SCHEMA (Compressed):
%s

BUSINESS CONTEXT:
%s

USER CUSTOM CONTEXT:
%s

METADATA & SCHEMA INTEGRITY:
1. **STRICT Table & Column Locking**: You MUST ONLY use table names and column names exactly as they appear in the DATABASE SCHEMA (Compressed) section above. 
2. **NO Hallucinations**: Do NOT assume a table exists if it is not listed. Do NOT guess column names based on file names.
3. **Verification Step**: Before writing any SQL, verified that EVERY table and EVERY column in your query is explicitly present in the schema provided.
4. **Alias Usage**: If multiple data sources are provided, use the 'alias.table_name' format.

CHAIN OF THOUGHT (CoT) REASONING:
Before providing any SQL or final answer, you MUST think step-by-step:
1.  **Analyze**: Understand intent and business logic.
2.  **Schema Check**: Identify the EXACT tables and columns needed from the provided metadata. If names differ from your intuition (e.g., underscores vs spaces), use the names from the schema.
3.  **Plan**: Identify dialect-specific functions and join logic.
4.  **Execute**: Formulate optimal SQL.
5.  **Refine**: Ensure query is read-only, safe, and efficient for the target dialect.

STRICT RULES:
1. ONLY generate READ-ONLY queries (SELECT).
2. ALWAYS use a "Thought Block": ` + "```thought ... ```" + `
3. Provide a scannable Markdown report after the thought block.
4. **QUERY SCOPE**: By default, select ALL relevant fields and ALL data rows WITHOUT any LIMIT, unless the user explicitly requests a sample or a specific limit.

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
STRICT RULES:
1. The query MUST be READ-ONLY (SELECT).
2. **METADATA CHECK**: Re-verify that all table names and column names in your corrected query MATCH the DATABASE SCHEMA (Compressed) provided in the system prompt.
3. **NO HALLUCINATION**: Do not assume existence of fields or tables not seen in the schema.

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

METADATA & SCHEMA INTEGRITY:
1. **STRICT Table & Column Locking**: You MUST ONLY use table names and column names exactly as they appear in the DATABASE SCHEMA (Compressed) section. 
2. **NO Hallucinations**: Do NOT assume a table exists if it is not listed. Do NOT guess column names based on file names.
3. **Verification Step**: Before writing any SQL, verified that EVERY table and EVERY column in your query is explicitly present in the schema provided.
4. **Alias Usage**: If multiple data sources are provided, use the 'alias.table_name' format.

CHAIN OF THOUGHT (CoT) REASONING:
Before providing any SQL or final answer, you MUST think step-by-step:
1. **Analyze**: Understand intent and business logic across all sources (files & database).
2. **Schema Check**: Identify the EXACT tables and columns needed from the PROVIDED SCHEMA. Note that files are also imported into the database as tables. Use the table names listed in the schema.
3. **Plan**: Identify dialect-specific functions and join logic.
4. **Execute**: Formulate optimal SQL.
5. **Refine**: Ensure query is read-only, safe, and efficient.

STRICT RULES:
1. ONLY generate READ-ONLY queries (SELECT).
2. ALWAYS use a "Thought Block": ` + "```thought ... ```" + `
3. Provide a scannable Markdown report after the thought block.
4. **QUERY SCOPE**: By default, load ALL fields and ALL data rows. Do NOT apply LIMIT unless explicitly requested by the user.

Always prioritize accuracy, safety, and the target dialect's constraints.`

	safeUserCtx := privacy.ApplyPrivacy(userContext)
	safeCtxContent := privacy.ApplyPrivacy(contextContent)

	return fmt.Sprintf(prompt, dialect, dialectHints, privacy.ApplyPrivacy(filesText.String()), safeCtxContent, safeUserCtx, compressedSchema)
}
