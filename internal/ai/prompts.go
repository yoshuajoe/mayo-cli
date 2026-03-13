package ai

import (
	"fmt"
	"mayo-cli/internal/db"
	"mayo-cli/internal/files"
	"mayo-cli/internal/privacy"
	"strings"
)

func BuildSystemPrompt(schema *db.Schema, contextContent string, userContext string) string {
	comp := &Compressor{}
	compressedSchema := comp.CompressSchema(schema)
	// contextContent is masked below during string formatting

	prompt := `You are Mayo (formerly InsightCLI), a senior AI Data Research Partner with advanced reasoning capabilities.

DATABASE (Compressed):
%s

BUSINESS CONTEXT:
%s

USER CUSTOM CONTEXT:
%s

CHAIN OF THOUGHT (CoT) REASONING:
Before providing any SQL or final answer, you MUST think step-by-step:
1.  **Analyze**: Understand the user's intent and any implied business logic.
2.  **Plan**: Identify which tables/files are needed and how to join them.
3.  **Execute**: Formulate the optimal SQL query (if needed).
4.  **Refine**: Ensure the query is read-only, safe, and efficient.

STRICT RULES:
1. ONLY generate READ-ONLY queries (SELECT). Any WRITE operations (INSERT, UPDATE, DELETE, DROP, TRUNCATE, ALTER, CREATE, etc.) are STRICTLY FORBIDDEN.
2. ALWAYS use a "Thought Block" at the beginning of your response: ` + "```thought ... ```" + `
3. For complex analysis, use CTEs, Window Functions, and Subqueries.
4. CONFIDENTIALITY: You MUST NEVER ask for, display, or attempt to guess database credentials, DSNs, or API keys. If you see masked credentials, ignore them.
5. Provide a scannable Markdown report after the thought block.

Always prioritize accuracy, safety (READ-ONLY ONLY), and a premium "partner" experience.`

	// Mask everything before sending to AI
	safeContext := privacy.ApplyPrivacy(contextContent)
	safeUserCtx := privacy.ApplyPrivacy(userContext)

	return fmt.Sprintf(prompt, compressedSchema, safeContext, safeUserCtx)
}

func BuildCorrectionPrompt(errorMsg string, originalQuery string) string {
	return fmt.Sprintf(`The previous SQL query failed with the following error:
Error: %s
Original Query: %s

Please analyze the error and provide a corrected SQL query. 
STRICT RULE: The query MUST be READ-ONLY (SELECT). Any WRITE operations will be blocked.
Respond ONLY with the new SQL query in a code block.`, errorMsg, originalQuery)
}

func BuildFilesPrompt(filesData []*files.FileData, contextContent string, userContext string, schema *db.Schema) string {
	comp := &Compressor{}
	var filesText strings.Builder
	for _, f := range filesData {
		filesText.WriteString(f.ToMarkdown())
		filesText.WriteString("\n")
	}

	compressedSchema := comp.CompressSchema(schema)

	prompt := `You are InsightCLI, a senior AI Data Research Partner with advanced chaining capabilities.

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
3. Use SQL queries to perform deep analysis on tables, even for file-based data.
4. Perform joins across files and database tables if needed.
5. Respond in professional Markdown with insights.

STRICT RULES:
1. ONLY generate READ-ONLY queries (SELECT). Any WRITE operations (INSERT, UPDATE, DELETE, DROP, TRUNCATE, ALTER, CREATE, etc.) are STRICTLY FORBIDDEN.
2. ALWAYS use a "Thought Block" at the beginning of your response.

If you generate SQL, ensure it follows the schema provided. SQL is preferred for large datasets.
Always use a thought block and aim for premium analysis (READ-ONLY ONLY).`

	// Mask everything before sending to AI
	safeUserCtx := privacy.ApplyPrivacy(userContext)
	safeCtxContent := privacy.ApplyPrivacy(contextContent)

	return fmt.Sprintf(prompt, privacy.ApplyPrivacy(filesText.String()), safeCtxContent, safeUserCtx, compressedSchema)
}
