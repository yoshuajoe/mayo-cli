package files

import (
	"encoding/csv"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/xuri/excelize/v2"
)

type FileData struct {
	Name    string     `json:"name"`
	Type    string     `json:"type"`
	Headers []string   `json:"headers"`
	Rows    [][]string `json:"rows"` // Sample rows for context
	AllRows [][]string `json:"-"`    // All rows for DB export
	Summary string     `json:"summary"`
}

func ParseCSV(path string) (*FileData, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	reader := csv.NewReader(file)
	records, err := reader.ReadAll()
	if err != nil {
		return nil, err
	}

	if len(records) == 0 {
		return nil, fmt.Errorf("empty CSV file")
	}

	data := &FileData{
		Name:    path,
		Type:    "csv",
		Headers: records[0],
		AllRows: records[1:],
	}

	// Limit rows for context efficiency
	maxRows := 10
	if len(data.AllRows) < maxRows {
		maxRows = len(data.AllRows)
	}
	data.Rows = data.AllRows[:maxRows]
	data.Summary = fmt.Sprintf("CSV file with %d columns and %d total records.", len(data.Headers), len(data.AllRows))

	return data, nil
}

func ParseXLSX(path string) (*FileData, error) {
	f, err := excelize.OpenFile(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	// Use the first sheet by default
	sheetName := f.GetSheetName(0)
	rows, err := f.GetRows(sheetName)
	if err != nil {
		return nil, err
	}

	if len(rows) == 0 {
		return nil, fmt.Errorf("empty XLSX file")
	}

	data := &FileData{
		Name:    path,
		Type:    "xlsx",
		Headers: rows[0],
		AllRows: rows[1:],
	}

	maxRows := 10
	if len(data.AllRows) < maxRows {
		maxRows = len(data.AllRows)
	}
	data.Rows = data.AllRows[:maxRows]
	data.Summary = fmt.Sprintf("Excel file (Sheet: %s) with %d columns and %d total rows.", sheetName, len(data.Headers), len(data.AllRows))

	return data, nil
}

func (f *FileData) ToMarkdown() string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("### File: %s (%s)\n", f.Name, f.Type))
	sb.WriteString(fmt.Sprintf("%s\n\n", f.Summary))
	if len(f.Headers) > 0 {
		sb.WriteString("| " + strings.Join(f.Headers, " | ") + " |\n")
		sb.WriteString("| " + strings.Repeat("--- | ", len(f.Headers)) + "\n")
		for _, row := range f.Rows {
			sb.WriteString("| " + strings.Join(row, " | ") + " |\n")
		}
	}
	return sb.String()
}

func ScanDirectory(dirPath string) ([]*FileData, error) {
	filesList, err := os.ReadDir(dirPath)
	if err != nil {
		return nil, err
	}

	var results []*FileData
	for _, f := range filesList {
		if f.IsDir() {
			continue
		}
		ext := strings.ToLower(filepath.Ext(f.Name()))
		path := filepath.Join(dirPath, f.Name())
		var data *FileData
		var err error

		switch ext {
		case ".csv":
			data, err = ParseCSV(path)
		case ".xlsx":
			data, err = ParseXLSX(path)
		case ".pdf":
			data = &FileData{Name: path, Type: "pdf", Summary: "PDF file (Text extraction pending)."}
		}

		if err == nil && data != nil {
			results = append(results, data)
		}
	}
	return results, nil
}
