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

// ImportRowsToSQLite creates a table in SQLite and inserts data from sql.Rows
func ImportRowsToSQLite(targetDB *sql.DB, tableName string, rows *sql.Rows) error {
	cols, err := rows.Columns()
	if err != nil {
		return err
	}

	// Sanitize table name
	safeTableName := SanitizeName(tableName)

	var colDefs []string
	for _, col := range cols {
		// Sanitize column name as well
		safeCol := SanitizeName(col)
		colDefs = append(colDefs, fmt.Sprintf("\"%s\" TEXT", safeCol))
	}

	createStmt := fmt.Sprintf("CREATE TABLE IF NOT EXISTS \"%s\" (%s)", safeTableName, strings.Join(colDefs, ", "))
	_, err = targetDB.Exec(createStmt)
	if err != nil {
		return fmt.Errorf("failed to create table %s: %v", safeTableName, err)
	}

	placeholders := make([]string, len(cols))
	for i := range placeholders {
		placeholders[i] = "?"
	}
	insertStmt := fmt.Sprintf("INSERT INTO \"%s\" VALUES (%s)", safeTableName, strings.Join(placeholders, ", "))

	tx, err := targetDB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(insertStmt)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for rows.Next() {
		vals := make([]interface{}, len(cols))
		ptr := make([]interface{}, len(cols))
		for i := range vals {
			ptr[i] = &vals[i]
		}
		if err := rows.Scan(ptr...); err != nil {
			return err
		}

		cleanVals := make([]interface{}, len(cols))
		for i, v := range vals {
			if v == nil {
				cleanVals[i] = nil
			} else {
				// Convert to string for broad compatibility in the bridge stage
				cleanVals[i] = fmt.Sprintf("%v", v)
			}
		}

		_, err = stmt.Exec(cleanVals...)
		if err != nil {
			return err
		}
	}

	return tx.Commit()
}
