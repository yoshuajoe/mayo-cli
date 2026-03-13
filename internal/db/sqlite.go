package db

import (
	"database/sql"
	"fmt"
	"mayo-cli/internal/files"
	"path/filepath"
	"regexp"
	"strings"
)

// ImportFileDataToSQLite creates a table in SQLite and inserts all data from FileData
func ImportFileDataToSQLite(sqlDB *sql.DB, data *files.FileData) (string, error) {
	// 1. Sanitize table name
	tableName := SanitizeName(filepath.Base(data.Name))
	// Remove extension if present
	tableName = regexp.MustCompile(`\.(csv|xlsx|xls|pdf)$`).ReplaceAllString(tableName, "")
	if tableName == "" {
		tableName = "imported_data"
	}

	// 2. Create table schema
	var cols []string
	for _, h := range data.Headers {
		safeH := SanitizeName(h)
		if safeH == "" {
			safeH = "column_unknown"
		}
		cols = append(cols, fmt.Sprintf("\"%s\" TEXT", safeH))
	}

	createStmt := fmt.Sprintf("CREATE TABLE IF NOT EXISTS \"%s\" (%s)", tableName, strings.Join(cols, ", "))
	_, err := sqlDB.Exec(createStmt)
	if err != nil {
		return "", fmt.Errorf("failed to create table: %v", err)
	}

	// 3. Clear existing data if table existed
	_, _ = sqlDB.Exec(fmt.Sprintf("DELETE FROM \"%s\"", tableName))

	// 4. Insert data
	if len(data.AllRows) > 0 {
		placeholders := make([]string, len(data.Headers))
		for i := range placeholders {
			placeholders[i] = "?"
		}
		insertStmt := fmt.Sprintf("INSERT INTO \"%s\" VALUES (%s)", tableName, strings.Join(placeholders, ", "))

		tx, err := sqlDB.Begin()
		if err != nil {
			return "", err
		}

		stmt, err := tx.Prepare(insertStmt)
		if err != nil {
			tx.Rollback()
			return "", err
		}
		defer stmt.Close()

		for _, row := range data.AllRows {
			// Ensure row matches header length
			vals := make([]interface{}, len(data.Headers))
			for i := range data.Headers {
				if i < len(row) {
					vals[i] = row[i]
				} else {
					vals[i] = ""
				}
			}
			_, _ = stmt.Exec(vals...)
		}

		if err := tx.Commit(); err != nil {
			return "", err
		}
	}

	return tableName, nil
}

func SanitizeName(name string) string {
	// Only allow alphanumeric and underscore
	re := regexp.MustCompile(`[^a-zA-Z0-9_]`)
	return re.ReplaceAllString(name, "_")
}
