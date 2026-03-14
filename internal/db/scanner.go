package db

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	_ "github.com/go-sql-driver/mysql"
	_ "github.com/lib/pq"
	_ "modernc.org/sqlite"
)

type Column struct {
	Name          string   `json:"name"`
	Type          string   `json:"type"`
	IsNullable    bool     `json:"is_nullable"`
	SampleValues  []string `json:"sample_values,omitempty"`
	IsCategorical bool     `json:"is_categorical,omitempty"`
	Categories    []string `json:"categories,omitempty"`
}

type Table struct {
	Name        string   `json:"name"`
	SchemaName  string   `json:"schema_name,omitempty"`
	Columns     []Column `json:"columns"`
	PK          []string `json:"primary_keys"`
	Description string   `json:"description,omitempty"`
	RowCount    int64    `json:"row_count,omitempty"`
}

type Schema struct {
	Tables []Table `json:"tables"`
}

func Connect(driver, dsn string) (*sql.DB, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return ConnectWithContext(ctx, driver, dsn)
}

func ConnectWithContext(ctx context.Context, driver, dsn string) (*sql.DB, error) {
	db, err := sql.Open(driver, dsn)
	if err != nil {
		return nil, err
	}
	if err := db.PingContext(ctx); err != nil {
		return nil, err
	}
	return db, nil
}

func ScanSchema(ctx context.Context, db *sql.DB, driver string) (*Schema, error) {
	if driver == "postgres" {
		return scanPostgres(ctx, db)
	} else if driver == "mysql" {
		return scanMySQL(ctx, db)
	} else if driver == "sqlite" {
		return scanSQLite(ctx, db)
	}
	return nil, fmt.Errorf("unsupported driver: %s", driver)
}

func scanPostgres(ctx context.Context, db *sql.DB) (*Schema, error) {
	query := `
		SELECT table_schema, table_name, column_name, data_type, is_nullable
		FROM information_schema.columns
		WHERE table_schema NOT IN ('information_schema', 'pg_catalog')
		ORDER BY table_schema, table_name, ordinal_position;
	`
	rows, err := db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	schema := &Schema{}
	tableMap := make(map[string]*Table)

	for rows.Next() {
		var tableSchema, tableName, colName, dataType, isNullable string
		if err := rows.Scan(&tableSchema, &tableName, &colName, &dataType, &isNullable); err != nil {
			return nil, err
		}

		key := fmt.Sprintf("%s.%s", tableSchema, tableName)
		if _, ok := tableMap[key]; !ok {
			table := &Table{
				Name:       tableName,
				SchemaName: tableSchema,
			}
			schema.Tables = append(schema.Tables, *table)
			tableMap[key] = &schema.Tables[len(schema.Tables)-1]
		}

		tableMap[key].Columns = append(tableMap[key].Columns, Column{
			Name:       colName,
			Type:       dataType,
			IsNullable: isNullable == "YES",
		})
	}

	return schema, nil
}

func scanMySQL(ctx context.Context, db *sql.DB) (*Schema, error) {
	query := `
		SELECT table_schema, table_name, column_name, data_type, is_nullable
		FROM information_schema.columns
		WHERE table_schema NOT IN ('information_schema', 'mysql', 'performance_schema', 'sys')
		ORDER BY table_schema, table_name, ordinal_position;
	`
	rows, err := db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	schema := &Schema{}
	tableMap := make(map[string]*Table)

	for rows.Next() {
		var tableSchema, tableName, colName, dataType, isNullable string
		if err := rows.Scan(&tableSchema, &tableName, &colName, &dataType, &isNullable); err != nil {
			return nil, err
		}

		key := fmt.Sprintf("%s.%s", tableSchema, tableName)
		if _, ok := tableMap[key]; !ok {
			table := &Table{
				Name:       tableName,
				SchemaName: tableSchema,
			}
			schema.Tables = append(schema.Tables, *table)
			tableMap[key] = &schema.Tables[len(schema.Tables)-1]
		}

		tableMap[key].Columns = append(tableMap[key].Columns, Column{
			Name:       colName,
			Type:       dataType,
			IsNullable: isNullable == "YES",
		})
	}

	return schema, nil
}

func scanSQLite(ctx context.Context, db *sql.DB) (*Schema, error) {
	query := `
		SELECT name FROM sqlite_master WHERE type='table' AND name NOT LIKE 'sqlite_%';
	`
	rows, err := db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	schema := &Schema{}
	for rows.Next() {
		var tableName string
		if err := rows.Scan(&tableName); err != nil {
			return nil, err
		}

		table := Table{Name: tableName}
		
		// Get columns for this table
		colRows, err := db.QueryContext(ctx, fmt.Sprintf("PRAGMA table_info(%s)", tableName))
		if err != nil {
			continue
		}
		for colRows.Next() {
			var cid int
			var name, ctype string
			var notnull, pk int
			var dflt interface{}
			if err := colRows.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk); err != nil {
				continue
			}
			table.Columns = append(table.Columns, Column{
				Name:       name,
				Type:       ctype,
				IsNullable: notnull == 0,
			})
		}
		colRows.Close()
		schema.Tables = append(schema.Tables, table)
	}

	return schema, nil
}

func ScanAndEnrichSchema(ctx context.Context, dbConn *sql.DB, driver string) (*Schema, error) {
	schema, err := ScanSchema(ctx, dbConn, driver)
	if err != nil {
		return nil, err
	}

	for i := range schema.Tables {
		tbl := &schema.Tables[i]
		
		fullTableName := tbl.Name
		if tbl.SchemaName != "" {
			fullTableName = fmt.Sprintf("%s.%s", tbl.SchemaName, tbl.Name)
		}

		// 1. Get Row Count
		var count int64
		countErr := dbConn.QueryRowContext(ctx, fmt.Sprintf("SELECT COUNT(*) FROM %s", fullTableName)).Scan(&count)
		if countErr == nil {
			tbl.RowCount = count
		}

		// 2. Sample data and identify Categorical fields
		for j := range tbl.Columns {
			col := &tbl.Columns[j]
			
			// Get samples
			limit := 5
			query := fmt.Sprintf("SELECT DISTINCT %s FROM %s WHERE %s IS NOT NULL LIMIT %d", col.Name, fullTableName, col.Name, limit)
			rows, err := dbConn.QueryContext(ctx, query)
			if err == nil {
				for rows.Next() {
					var v interface{}
					if err := rows.Scan(&v); err == nil {
						col.SampleValues = append(col.SampleValues, fmt.Sprintf("%v", v))
					}
				}
				rows.Close()
			}

			// Check if categorical (simple heuristic: less than 15 unique values and row count > 20)
			if tbl.RowCount > 20 {
				var distinctCount int
				q2 := fmt.Sprintf("SELECT COUNT(DISTINCT %s) FROM %s", col.Name, fullTableName)
				if err := dbConn.QueryRowContext(ctx, q2).Scan(&distinctCount); err == nil {
					if distinctCount > 0 && distinctCount <= 15 {
						col.IsCategorical = true
						// Get categories
						q3 := fmt.Sprintf("SELECT DISTINCT %s FROM %s WHERE %s IS NOT NULL ORDER BY %s LIMIT 15", col.Name, fullTableName, col.Name, col.Name)
						rows, err := dbConn.QueryContext(ctx, q3)
						if err == nil {
							for rows.Next() {
								var v interface{}
								if err := rows.Scan(&v); err == nil {
									col.Categories = append(col.Categories, fmt.Sprintf("%v", v))
								}
							}
							rows.Close()
						}
					}
				}
			}
		}
	}

	return schema, nil
}

func ExportSchemaToMarkdown(schema *Schema) string {
	var sb strings.Builder
	sb.WriteString("# Database Schema Metadata\n\n")
	sb.WriteString("This file contains the automatically scanned schema metadata. You can manually edit the descriptions to help the AI understand the data better.\n\n")

	for _, tbl := range schema.Tables {
		displayName := tbl.Name
		if tbl.SchemaName != "" {
			displayName = fmt.Sprintf("%s.%s", tbl.SchemaName, tbl.Name)
		}
		sb.WriteString(fmt.Sprintf("## Table: %s\n", displayName))
		if tbl.Description != "" {
			sb.WriteString(fmt.Sprintf("Description: %s\n\n", tbl.Description))
		} else {
			sb.WriteString("Description: [Add table description here]\n\n")
		}
		sb.WriteString(fmt.Sprintf("- **Rows**: %d\n", tbl.RowCount))
		if tbl.SchemaName != "" {
			sb.WriteString(fmt.Sprintf("- **Schema**: %s\n", tbl.SchemaName))
		}
		sb.WriteString("\n")

		sb.WriteString("| Column | Type | Nullable | Categorical | Sample Values / Categories |\n")
		sb.WriteString("|--------|------|----------|-------------|----------------------------|\n")
		for _, col := range tbl.Columns {
			nullable := "No"
			if col.IsNullable {
				nullable = "Yes"
			}
			categorical := "No"
			if col.IsCategorical {
				categorical = "Yes"
			}

			valStr := ""
			if col.IsCategorical {
				valStr = "Categories: " + strings.Join(col.Categories, ", ")
			} else if len(col.SampleValues) > 0 {
				valStr = "Samples: " + strings.Join(col.SampleValues, ", ")
			}

			sb.WriteString(fmt.Sprintf("| %s | %s | %s | %s | %s |\n", col.Name, col.Type, nullable, categorical, valStr))
		}
		sb.WriteString("\n")
	}

	return sb.String()
}

func ParseSchemaFromMarkdown(content string) (*Schema, error) {
	schema := &Schema{}
	lines := strings.Split(content, "\n")
	var currentTable *Table

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "## Table: ") {
			fullName := strings.TrimPrefix(trimmed, "## Table: ")
			table := Table{}
			if strings.Contains(fullName, ".") {
				parts := strings.SplitN(fullName, ".", 2)
				table.SchemaName = parts[0]
				table.Name = parts[1]
			} else {
				table.Name = fullName
			}
			schema.Tables = append(schema.Tables, table)
			currentTable = &schema.Tables[len(schema.Tables)-1]
		} else if currentTable != nil && strings.HasPrefix(trimmed, "Description: ") {
			desc := strings.TrimPrefix(trimmed, "Description: ")
			if desc != "[Add table description here]" {
				currentTable.Description = desc
			}
		} else if currentTable != nil && strings.HasPrefix(trimmed, "- **Rows**: ") {
			fmt.Sscanf(trimmed, "- **Rows**: %d", &currentTable.RowCount)
		} else if currentTable != nil && strings.HasPrefix(trimmed, "- **Schema**: ") {
			currentTable.SchemaName = strings.TrimPrefix(trimmed, "- **Schema**: ")
		} else if currentTable != nil && strings.HasPrefix(trimmed, "|") && !strings.Contains(trimmed, "Column | Type") && !strings.Contains(trimmed, "---|---") {
			parts := strings.Split(trimmed, "|")
			if len(parts) >= 6 {
				col := Column{
					Name:          strings.TrimSpace(parts[1]),
					Type:          strings.TrimSpace(parts[2]),
					IsNullable:    strings.TrimSpace(parts[3]) == "Yes",
					IsCategorical: strings.TrimSpace(parts[4]) == "Yes",
				}
				valStr := strings.TrimSpace(parts[5])
				if strings.HasPrefix(valStr, "Categories: ") {
					cats := strings.TrimPrefix(valStr, "Categories: ")
					col.Categories = strings.Split(cats, ", ")
					for k := range col.Categories {
						col.Categories[k] = strings.TrimSpace(col.Categories[k])
					}
				} else if strings.HasPrefix(valStr, "Samples: ") {
					samples := strings.TrimPrefix(valStr, "Samples: ")
					col.SampleValues = strings.Split(samples, ", ")
					for k := range col.SampleValues {
						col.SampleValues[k] = strings.TrimSpace(col.SampleValues[k])
					}
				}
				currentTable.Columns = append(currentTable.Columns, col)
			}
		}
	}
	return schema, nil
}
