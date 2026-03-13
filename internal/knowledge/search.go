package knowledge

import (
	"database/sql"
	"fmt"
	"strings"
)

func IndexDocument(db *sql.DB, doc *Document, tableName string) error {
	// 1. Create table if not exists
	// We'll store: id, source, content
	// In a real vector DB, we'd store embeddings too.
	createStmt := fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS %s (
			id INTEGER PRIMARY KEY,
			source TEXT,
			content TEXT
		)
	`, tableName)
	
	// Adjust for Postgres if needed, but SQLite/Standard SQL works for basic
	_, err := db.Exec(createStmt)
	if err != nil {
		return fmt.Errorf("failed to create knowledge table: %v", err)
	}

	// 2. Chunking (Simple by double newline or fixed size)
	chunks := strings.Split(doc.Content, "\n\n")
	
	tx, err := db.Begin()
	if err != nil {
		return err
	}

	stmt, err := tx.Prepare(fmt.Sprintf("INSERT INTO %s (source, content) VALUES (?, ?)", tableName))
	if err != nil {
		// Try without placeholders if it's some other weird driver, but Standard is better
		tx.Rollback()
		return err
	}
	defer stmt.Close()

	for _, chunk := range chunks {
		trimmed := strings.TrimSpace(chunk)
		if len(trimmed) < 10 { continue }
		_, _ = stmt.Exec(doc.Source, trimmed)
	}

	return tx.Commit()
}
