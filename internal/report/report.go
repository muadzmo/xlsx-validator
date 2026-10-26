package report

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// Standard stable error codes
const (
	CodeSheetNotFound     = "SHEET_NOT_FOUND"
	CodeMissingHeader     = "MISSING_HEADER"
	CodeDuplicateHeader   = "DUPLICATE_HEADER"
	CodeFormulaHeader     = "FORMULA_HEADER"
	CodeRequired          = "REQUIRED"
	CodeDuplicate         = "DUPLICATE"
	CodeInvalidType       = "INVALID_TYPE"
	CodeInvalidFormat     = "INVALID_FORMAT"
	CodeMinLength         = "MIN_LENGTH"
	CodeMaxLength         = "MAX_LENGTH"
	CodeMinValue          = "MIN_VALUE"
	CodeMaxValue          = "MAX_VALUE"
	CodeNotAllowed        = "NOT_ALLOWED"
	CodePatternMismatch   = "PATTERN_MISMATCH"
	CodeFormulaNotAllowed = "FORMULA_NOT_ALLOWED"
	CodeExcelError        = "EXCEL_ERROR"
)

// Supported output formats
const (
	FormatText = "text"
	FormatJSON = "json"
	FormatCSV  = "csv"
)

// Violation represents a single validation failure.
type Violation struct {
	Sheet   string `json:"sheet"`
	Row     int    `json:"row"`
	Cell    string `json:"cell"`
	Column  string `json:"column"`
	Value   string `json:"value"`
	Code    string `json:"code"`
	Message string `json:"message"`
	ColIdx  int    `json:"-"` // 1-based column position used for deterministic sorting
}

// Summary contains summary statistics of validation.
type Summary struct {
	CheckedRows         int `json:"checkedRows"`
	ValidRows           int `json:"validRows"`
	InvalidRows         int `json:"invalidRows"`
	SkippedRows         int `json:"skippedRows"`
	ErrorCount          int `json:"errorCount"`
	StructureErrorCount int `json:"structureErrorCount"`
}

// Report holds the complete validation result.
type Report struct {
	File    string      `json:"file"`
	Sheet   string      `json:"sheet"`
	Summary Summary     `json:"summary"`
	Errors  []Violation `json:"errors"`
}

// NewReport creates an initialized Report with non-nil Errors slice.
func NewReport(file, sheet string) *Report {
	return &Report{
		File:   file,
		Sheet:  sheet,
		Errors: make([]Violation, 0),
	}
}

// Sort deterministic: sort by Excel row, column position, then rule code, then message.
func (r *Report) Sort() {
	sort.SliceStable(r.Errors, func(i, j int) bool {
		if r.Errors[i].Row != r.Errors[j].Row {
			return r.Errors[i].Row < r.Errors[j].Row
		}
		if r.Errors[i].ColIdx != r.Errors[j].ColIdx {
			return r.Errors[i].ColIdx < r.Errors[j].ColIdx
		}
		if r.Errors[i].Code != r.Errors[j].Code {
			return r.Errors[i].Code < r.Errors[j].Code
		}
		return r.Errors[i].Message < r.Errors[j].Message
	})
}

// FormatJSON writes the report as JSON.
func (r *Report) FormatJSON(w io.Writer) error {
	r.Sort()
	if r.Errors == nil {
		r.Errors = make([]Violation, 0)
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	return enc.Encode(r)
}

// FormatCSV writes the report as CSV.
// Neutralizes spreadsheet formula injection in exported text beginning with =, +, -, @, \t, or \r.
func (r *Report) FormatCSV(w io.Writer) error {
	r.Sort()
	cw := csv.NewWriter(w)

	// Write CSV header
	header := []string{"sheet", "row", "cell", "column", "value", "code", "message"}
	if err := cw.Write(header); err != nil {
		return err
	}

	for _, v := range r.Errors {
		record := []string{
			sanitizeCSV(v.Sheet),
			strconv.Itoa(v.Row),
			sanitizeCSV(v.Cell),
			sanitizeCSV(v.Column),
			sanitizeCSV(v.Value),
			sanitizeCSV(v.Code),
			sanitizeCSV(v.Message),
		}
		if err := cw.Write(record); err != nil {
			return err
		}
	}

	cw.Flush()
	return cw.Error()
}

// sanitizeCSV neutralizes spreadsheet formula injection.
func sanitizeCSV(s string) string {
	if s == "" {
		return s
	}
	first := s[0]
	if first == '=' || first == '+' || first == '-' || first == '@' || first == '\t' || first == '\r' {
		return "'" + s
	}
	return s
}

// FormatText writes a human-readable text report.
func (r *Report) FormatText(w io.Writer) error {
	r.Sort()

	fmt.Fprintf(w, "File:  %s\n", r.File)
	fmt.Fprintf(w, "Sheet: %s\n", r.Sheet)
	fmt.Fprintf(w, "Summary:\n")
	fmt.Fprintf(w, "  Checked rows:           %d\n", r.Summary.CheckedRows)
	fmt.Fprintf(w, "  Valid rows:             %d\n", r.Summary.ValidRows)
	fmt.Fprintf(w, "  Invalid rows:           %d\n", r.Summary.InvalidRows)
	fmt.Fprintf(w, "  Skipped empty rows:     %d\n", r.Summary.SkippedRows)
	fmt.Fprintf(w, "  Structure errors:       %d\n", r.Summary.StructureErrorCount)
	fmt.Fprintf(w, "  Total errors:           %d\n", r.Summary.ErrorCount)
	fmt.Fprintln(w)

	if len(r.Errors) == 0 {
		fmt.Fprintln(w, "Validation Status: PASSED (0 violations)")
		return nil
	}

	fmt.Fprintf(w, "Validation Status: FAILED (%d violations)\n", len(r.Errors))
	fmt.Fprintln(w, "Violations:")
	for i, v := range r.Errors {
		loc := v.Cell
		if loc == "" {
			if v.Row > 0 {
				loc = fmt.Sprintf("Row %d", v.Row)
			} else {
				loc = "Sheet"
			}
		}
		colInfo := ""
		if v.Column != "" {
			colInfo = fmt.Sprintf(" [%s]", v.Column)
		}
		valInfo := ""
		if v.Value != "" {
			valInfo = fmt.Sprintf(" (value: %q)", v.Value)
		}
		fmt.Fprintf(w, "  %3d. %-10s %-20s %s%s%s\n", i+1, loc, v.Code, v.Message, colInfo, valInfo)
	}

	return nil
}

// WriteOutput writes the report to a destination writer or atomic file.
func WriteOutput(destPath, format string, r *Report, protectedPaths ...string) error {
	format = strings.ToLower(strings.TrimSpace(format))
	switch format {
	case FormatText, FormatJSON, FormatCSV:
	default:
		return fmt.Errorf("unsupported output format %q (allowed: text, json, csv)", format)
	}

	if destPath == "" {
		// Output to stdout
		return writeToWriter(os.Stdout, format, r)
	}

	// Refuse output paths that resolve to any protected paths (input workbook or config)
	for _, protected := range protectedPaths {
		if protected == "" {
			continue
		}
		same, err := isSamePath(destPath, protected)
		if err != nil {
			return fmt.Errorf("checking output path collision: %w", err)
		}
		if same {
			return fmt.Errorf("output path %q resolves to input file %q", destPath, protected)
		}
	}

	// Atomic file write using a temporary file in the destination directory
	destDir := filepath.Dir(destPath)
	if err := os.MkdirAll(destDir, 0755); err != nil {
		return fmt.Errorf("creating output directory: %w", err)
	}

	tmpFile, err := os.CreateTemp(destDir, ".xlsx-validator-report-*.tmp")
	if err != nil {
		return fmt.Errorf("creating temporary report file: %w", err)
	}
	tmpName := tmpFile.Name()

	// Ensure cleanup on failure
	cleanedUp := false
	defer func() {
		if !cleanedUp {
			_ = tmpFile.Close()
			_ = os.Remove(tmpName)
		}
	}()

	if err := writeToWriter(tmpFile, format, r); err != nil {
		return fmt.Errorf("writing report: %w", err)
	}

	if err := tmpFile.Sync(); err != nil {
		return fmt.Errorf("syncing report file: %w", err)
	}

	if err := tmpFile.Close(); err != nil {
		return fmt.Errorf("closing temporary report file: %w", err)
	}

	if err := os.Rename(tmpName, destPath); err != nil {
		return fmt.Errorf("renaming report file into place: %w", err)
	}

	cleanedUp = true
	return nil
}

func writeToWriter(w io.Writer, format string, r *Report) error {
	switch format {
	case FormatJSON:
		return r.FormatJSON(w)
	case FormatCSV:
		return r.FormatCSV(w)
	default:
		return r.FormatText(w)
	}
}

// isSamePath checks whether two file paths point to the same underlying file (including symlinks and hardlinks).
func isSamePath(path1, path2 string) (bool, error) {
	// 1. Resolve symlinks first
	p1Clean := filepath.Clean(path1)
	p2Clean := filepath.Clean(path2)

	p1Eval, err1 := filepath.EvalSymlinks(p1Clean)
	p2Eval, err2 := filepath.EvalSymlinks(p2Clean)

	if err1 == nil && err2 == nil {
		if p1Eval == p2Eval {
			return true, nil
		}
	}

	// 2. Check os.SameFile if both files exist
	fi1, err1 := os.Stat(path1)
	fi2, err2 := os.Stat(path2)
	if err1 == nil && err2 == nil {
		if os.SameFile(fi1, fi2) {
			return true, nil
		}
	}

	return false, nil
}
