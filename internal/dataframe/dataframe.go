package dataframe

import (
	"database/sql"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"mayo-cli/internal/config"

	"github.com/xuri/excelize/v2"
	_ "github.com/mattn/go-sqlite3"
)

// Frame represents a saved dataframe
type Frame struct {
	Name      string    `json:"name"`
	SQL       string    `json:"sql"`
	Columns   []string  `json:"columns"`
	RowCount  int       `json:"row_count"`
	CreatedAt time.Time `json:"created_at"`
}

func dbPath() string {
	return filepath.Join(config.GetConfigDir(), "dataframes.db")
}

func openDB() (*sql.DB, error) {
	path := dbPath()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite3", path)
	if err != nil {
		return nil, err
	}
	// Create registry table (metadata)
	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS _frames (
		name       TEXT PRIMARY KEY,
		sql_query  TEXT,
		columns    TEXT,
		row_count  INTEGER,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	)`)
	return db, err
}

// Save stores a result set as a named dataframe in SQLite.
// columns: []string, rows: [][]string (real/detokenized data — stored locally, never sent to AI raw)
func Save(name string, sqlQuery string, columns []string, rows [][]string) error {
	db, err := openDB()
	if err != nil {
		return err
	}
	defer db.Close()

	// Sanitize table name
	tableName := "df_" + sanitize(name)

	// Drop old table if exists
	db.Exec(fmt.Sprintf("DROP TABLE IF EXISTS \"%s\"", tableName))

	// Build CREATE TABLE
	var colDefs []string
	for _, col := range columns {
		colDefs = append(colDefs, fmt.Sprintf("\"%s\" TEXT", sanitize(col)))
	}
	createSQL := fmt.Sprintf("CREATE TABLE \"%s\" (%s)", tableName, strings.Join(colDefs, ", "))
	if _, err := db.Exec(createSQL); err != nil {
		return fmt.Errorf("failed to create table: %v", err)
	}

	// Insert rows
	if len(rows) > 0 {
		placeholders := strings.Repeat("?, ", len(columns))
		placeholders = strings.TrimSuffix(placeholders, ", ")
		insertSQL := fmt.Sprintf("INSERT INTO \"%s\" VALUES (%s)", tableName, placeholders)
		stmt, err := db.Prepare(insertSQL)
		if err != nil {
			return err
		}
		defer stmt.Close()
		for _, row := range rows {
			args := make([]interface{}, len(row))
			for i, v := range row {
				args[i] = v
			}
			if _, err := stmt.Exec(args...); err != nil {
				return err
			}
		}
	}

	// Save metadata
	colsJSON, _ := json.Marshal(columns)
	_, err = db.Exec(`INSERT OR REPLACE INTO _frames (name, sql_query, columns, row_count, created_at) VALUES (?, ?, ?, ?, ?)`,
		name, sqlQuery, string(colsJSON), len(rows), time.Now())
	return err
}

// List returns all saved dataframes
func List() ([]Frame, error) {
	db, err := openDB()
	if err != nil {
		return nil, err
	}
	defer db.Close()

	rows, err := db.Query("SELECT name, sql_query, columns, row_count, created_at FROM _frames ORDER BY created_at DESC")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var frames []Frame
	for rows.Next() {
		var f Frame
		var colsJSON string
		if err := rows.Scan(&f.Name, &f.SQL, &colsJSON, &f.RowCount, &f.CreatedAt); err != nil {
			continue
		}
		json.Unmarshal([]byte(colsJSON), &f.Columns)
		frames = append(frames, f)
	}
	return frames, nil
}

// Load retrieves rows from a saved dataframe
func Load(name string) (columns []string, rows [][]string, err error) {
	db, err := openDB()
	if err != nil {
		return nil, nil, err
	}
	defer db.Close()

	// Get columns from metadata
	var colsJSON string
	if err := db.QueryRow("SELECT columns FROM _frames WHERE name = ?", name).Scan(&colsJSON); err != nil {
		return nil, nil, fmt.Errorf("dataframe '%s' not found", name)
	}
	json.Unmarshal([]byte(colsJSON), &columns)

	tableName := "df_" + sanitize(name)
	sqlRows, err := db.Query(fmt.Sprintf("SELECT * FROM \"%s\"", tableName))
	if err != nil {
		return nil, nil, err
	}
	defer sqlRows.Close()

	for sqlRows.Next() {
		vals := make([]interface{}, len(columns))
		valPtrs := make([]interface{}, len(columns))
		for i := range vals {
			valPtrs[i] = &vals[i]
		}
		sqlRows.Scan(valPtrs...)
		row := make([]string, len(columns))
		for i, v := range vals {
			if b, ok := v.([]byte); ok {
				row[i] = string(b)
			} else {
				row[i] = fmt.Sprintf("%v", v)
			}
		}
		rows = append(rows, row)
	}
	return columns, rows, nil
}

// ExportMarkdown exports a saved dataframe as a Markdown table string
func ExportMarkdown(name string) (string, error) {
	columns, rows, err := Load(name)
	if err != nil {
		return "", err
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("## Dataframe: %s\n\n", name))
	sb.WriteString("| " + strings.Join(columns, " | ") + " |\n")
	sb.WriteString("|" + strings.Repeat(" --- |", len(columns)) + "\n")
	for _, row := range rows {
		sb.WriteString("| " + strings.Join(row, " | ") + " |\n")
	}
	return sb.String(), nil
}

func ExportCSV(name string, path string) error {
	columns, rows, err := Load(name)
	if err != nil {
		return err
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	w := csv.NewWriter(f)
	w.Write(columns)
	w.WriteAll(rows)
	return w.Error()
}

func ExportJSON(name string, path string) error {
	columns, rows, err := Load(name)
	if err != nil {
		return err
	}
	var data []map[string]interface{}
	for _, row := range rows {
		item := make(map[string]interface{})
		for i, col := range columns {
			item[col] = row[i]
		}
		data = append(data, item)
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return json.NewEncoder(f).Encode(data)
}

func ExportExcel(name string, path string) error {
	columns, rows, err := Load(name)
	if err != nil {
		return err
	}
	f := excelize.NewFile()
	sheet := "Sheet1"
	f.SetSheetName("Sheet1", name)
	sheet = name
	
	// Headers
	for i, col := range columns {
		cell, _ := excelize.CoordinatesToCellName(i+1, 1)
		f.SetCellValue(sheet, cell, col)
	}
	// Data
	for r, row := range rows {
		for c, val := range row {
			cell, _ := excelize.CoordinatesToCellName(c+1, r+2)
			f.SetCellValue(sheet, cell, val)
		}
	}
	return f.SaveAs(path)
}

// Delete removes a saved dataframe
func Delete(name string) error {
	db, err := openDB()
	if err != nil {
		return err
	}
	defer db.Close()

	tableName := "df_" + sanitize(name)
	db.Exec(fmt.Sprintf("DROP TABLE IF EXISTS \"%s\"", tableName))
	_, err = db.Exec("DELETE FROM _frames WHERE name = ?", name)
	return err
}

// Query executes a read-only SQL query against the dataframes database
func Query(sqlQuery string) (columns []string, rows [][]string, err error) {
	db, err := openDB()
	if err != nil {
		return nil, nil, err
	}
	defer db.Close()

	// Ensure read-only
	upper := strings.ToUpper(sqlQuery)
	if strings.Contains(upper, "DELETE") || strings.Contains(upper, "DROP") || strings.Contains(upper, "UPDATE") || strings.Contains(upper, "INSERT") {
		return nil, nil, fmt.Errorf("only SELECT queries are allowed on dataframes")
	}

	sqlRows, err := db.Query(sqlQuery)
	if err != nil {
		return nil, nil, err
	}
	defer sqlRows.Close()

	columns, _ = sqlRows.Columns()
	for sqlRows.Next() {
		vals := make([]interface{}, len(columns))
		valPtrs := make([]interface{}, len(columns))
		for i := range vals {
			valPtrs[i] = &vals[i]
		}
		if err := sqlRows.Scan(valPtrs...); err != nil {
			return nil, nil, err
		}
		row := make([]string, len(columns))
		for i, v := range vals {
			if b, ok := v.([]byte); ok {
				row[i] = string(b)
			} else {
				row[i] = fmt.Sprintf("%v", v)
			}
		}
		rows = append(rows, row)
	}
	return columns, rows, nil
}

func sanitize(s string) string {
	replacer := strings.NewReplacer(
		" ", "_", "-", "_", ".", "_", "/", "_", "\\", "_",
	)
	return replacer.Replace(strings.ToLower(s))
}
