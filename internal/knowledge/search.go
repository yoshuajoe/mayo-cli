package knowledge

import (
	"database/sql"
	"fmt"
	"strings"
)

func IndexDocument(db *sql.DB, doc *Document, tableName string) error {
	// 1. Create FTS5 virtual table if not exists
	// FTS5 provides superior search performance compared to basic LIKE
	createStmt := fmt.Sprintf(`
		CREATE VIRTUAL TABLE IF NOT EXISTS %s USING fts5(
			source,
			content,
			tokenize='porter unicode61'
		)
	`, tableName)

	_, err := db.Exec(createStmt)
	if err != nil {
		// Fallback for older SQLite environments without Fts5 (though modern Go modernc.org/sqlite usually has it)
		createStmtFallback := fmt.Sprintf(`
			CREATE TABLE IF NOT EXISTS %s (
				id INTEGER PRIMARY KEY,
				source TEXT,
				content TEXT
			)
		`, tableName)
		_, err = db.Exec(createStmtFallback)
		if err != nil {
			return fmt.Errorf("failed to create knowledge table: %v", err)
		}
	}

	// 2. Chunking (Improved: semantic chunking based on paragraphs/lines)
	chunks := strings.Split(doc.Content, "\n")
	var refinedChunks []string
	var currentChunk strings.Builder
	
	const maxChunkSize = 800
	for _, line := range chunks {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" { continue }
		
		if currentChunk.Len() + len(trimmed) > maxChunkSize {
			refinedChunks = append(refinedChunks, currentChunk.String())
			currentChunk.Reset()
		}
		currentChunk.WriteString(trimmed + " ")
	}
	if currentChunk.Len() > 0 {
		refinedChunks = append(refinedChunks, currentChunk.String())
	}

	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// 3. Clear existing source data if we are re-indexing same file
	_, _ = tx.Exec(fmt.Sprintf("DELETE FROM %s WHERE source = ?", tableName), doc.Source)

	stmt, err := tx.Prepare(fmt.Sprintf("INSERT INTO %s (source, content) VALUES (?, ?)", tableName))
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, chunk := range refinedChunks {
		if len(strings.TrimSpace(chunk)) < 20 { continue }
		_, _ = stmt.Exec(doc.Source, chunk)
	}

	return tx.Commit()
}

func SearchKnowledge(db *sql.DB, tableName string, query string, limit int) ([]string, error) {
	// Try FTS5 MATCH first
	searchQuery := fmt.Sprintf("SELECT content FROM %s WHERE %s MATCH ? ORDER BY rank LIMIT ?", tableName, tableName)
	rows, err := db.Query(searchQuery, query, limit)
	if err != nil {
		// Fallback to LIKE if FTS5 failed
		searchQuery = fmt.Sprintf("SELECT content FROM %s WHERE content LIKE ? LIMIT ?", tableName)
		rows, err = db.Query(searchQuery, "%"+query+"%", limit)
		if err != nil {
			return nil, err
		}
	}
	defer rows.Close()

	var results []string
	for rows.Next() {
		var content string
		if err := rows.Scan(&content); err == nil {
			results = append(results, content)
		}
	}
	return results, nil
}
