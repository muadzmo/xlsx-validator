package report

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReportJSON(t *testing.T) {
	r := NewReport("test.xlsx", "Sheet1")
	r.Summary = Summary{
		CheckedRows:         10,
		ValidRows:           8,
		InvalidRows:         2,
		SkippedRows:         1,
		ErrorCount:          2,
		StructureErrorCount: 0,
	}
	r.Errors = []Violation{
		{
			Sheet:   "Sheet1",
			Row:     5,
			Cell:    "A5",
			Column:  "EMP_NO",
			Value:   "E1",
			Code:    CodeDuplicate,
			Message: "duplicate value",
			ColIdx:  1,
		},
		{
			Sheet:   "Sheet1",
			Row:     2,
			Cell:    "B2",
			Column:  "EMAIL",
			Value:   "bad-email",
			Code:    CodeInvalidFormat,
			Message: "invalid email",
			ColIdx:  2,
		},
	}

	var buf bytes.Buffer
	if err := r.FormatJSON(&buf); err != nil {
		t.Fatalf("unexpected error formatting JSON: %v", err)
	}

	// Verify JSON is valid
	var decoded map[string]any
	if err := json.Unmarshal(buf.Bytes(), &decoded); err != nil {
		t.Fatalf("failed to unmarshal generated JSON: %v", err)
	}

	// Errors should be sorted by row: row 2 before row 5
	errs := decoded["errors"].([]any)
	if len(errs) != 2 {
		t.Fatalf("expected 2 errors, got %d", len(errs))
	}
	firstErr := errs[0].(map[string]any)
	if int(firstErr["row"].(float64)) != 2 {
		t.Errorf("expected first error row 2, got %v", firstErr["row"])
	}

	// Test success JSON has errors: []
	emptyReport := NewReport("test.xlsx", "Sheet1")
	var emptyBuf bytes.Buffer
	if err := emptyReport.FormatJSON(&emptyBuf); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(emptyBuf.String(), `"errors": []`) {
		t.Errorf("expected empty JSON report to contain '\"errors\": []', got: %s", emptyBuf.String())
	}
}

func TestReportCSV(t *testing.T) {
	r := NewReport("test.xlsx", "Sheet1")
	r.Errors = []Violation{
		{
			Sheet:   "Sheet1",
			Row:     2,
			Cell:    "A2",
			Column:  "NAME",
			Value:   "=1+1", // Formula injection
			Code:    CodeInvalidFormat,
			Message: "-malicious message", // Formula injection
			ColIdx:  1,
		},
		{
			Sheet:   "Sheet1",
			Row:     3,
			Cell:    "B3",
			Column:  "NOTE",
			Value:   "@SUM(1,2)", // Formula injection
			Code:    CodeNotAllowed,
			Message: "+plus leading",
			ColIdx:  2,
		},
		{
			Sheet:   "Sheet1",
			Row:     4,
			Cell:    "C4",
			Column:  "TAB_CELL",
			Value:   "\tTabValue",
			Code:    CodeRequired,
			Message: "standard",
			ColIdx:  3,
		},
	}

	var buf bytes.Buffer
	if err := r.FormatCSV(&buf); err != nil {
		t.Fatalf("unexpected error formatting CSV: %v", err)
	}

	cr := csv.NewReader(&buf)
	records, err := cr.ReadAll()
	if err != nil {
		t.Fatalf("failed to read generated CSV: %v", err)
	}

	// Header + 3 records
	if len(records) != 4 {
		t.Fatalf("expected 4 records, got %d", len(records))
	}

	// Check header
	expectedHeader := []string{"sheet", "row", "cell", "column", "value", "code", "message"}
	for i, h := range expectedHeader {
		if records[0][i] != h {
			t.Errorf("expected header %q, got %q", h, records[0][i])
		}
	}

	// Check formula neutralization
	row1 := records[1]
	if row1[4] != "'=1+1" {
		t.Errorf("expected neutralized value ''=1+1', got %q", row1[4])
	}
	if row1[6] != "'-malicious message" {
		t.Errorf("expected neutralized message ''-malicious message', got %q", row1[6])
	}

	row2 := records[2]
	if row2[4] != "'@SUM(1,2)" {
		t.Errorf("expected neutralized value ''@SUM(1,2)', got %q", row2[4])
	}
	if row2[6] != "'+plus leading" {
		t.Errorf("expected neutralized message ''+plus leading', got %q", row2[6])
	}

	row3 := records[3]
	if row3[4] != "'\tTabValue" {
		t.Errorf("expected neutralized tab value, got %q", row3[4])
	}

	// Success CSV: only header
	emptyReport := NewReport("test.xlsx", "Sheet1")
	var emptyBuf bytes.Buffer
	if err := emptyReport.FormatCSV(&emptyBuf); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	crEmpty := csv.NewReader(&emptyBuf)
	emptyRecords, err := crEmpty.ReadAll()
	if err != nil {
		t.Fatalf("failed reading empty CSV: %v", err)
	}
	if len(emptyRecords) != 1 {
		t.Errorf("expected exactly 1 record (header only) on empty CSV, got %d", len(emptyRecords))
	}
}

func TestReportText(t *testing.T) {
	r := NewReport("test.xlsx", "Sheet1")
	r.Summary = Summary{
		CheckedRows: 5,
		ValidRows:   5,
	}

	var buf bytes.Buffer
	if err := r.FormatText(&buf); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "PASSED (0 violations)") {
		t.Errorf("expected PASSED message, got: %s", out)
	}
}

func TestDeterministicSorting(t *testing.T) {
	r := NewReport("test.xlsx", "Sheet1")
	r.Errors = []Violation{
		{Row: 5, ColIdx: 2, Code: "B_CODE", Message: "b"},
		{Row: 2, ColIdx: 1, Code: "A_CODE", Message: "a"},
		{Row: 5, ColIdx: 1, Code: "A_CODE", Message: "a"},
		{Row: 5, ColIdx: 2, Code: "A_CODE", Message: "a"},
		{Row: 0, ColIdx: 0, Code: "MISSING_HEADER", Message: "missing header"},
	}

	r.Sort()

	if r.Errors[0].Row != 0 {
		t.Errorf("expected row 0 first, got %d", r.Errors[0].Row)
	}
	if r.Errors[1].Row != 2 {
		t.Errorf("expected row 2 second, got %d", r.Errors[1].Row)
	}
	if r.Errors[2].Row != 5 || r.Errors[2].ColIdx != 1 {
		t.Errorf("expected row 5 col 1 third, got row %d col %d", r.Errors[2].Row, r.Errors[2].ColIdx)
	}
	if r.Errors[3].Row != 5 || r.Errors[3].ColIdx != 2 || r.Errors[3].Code != "A_CODE" {
		t.Errorf("expected row 5 col 2 A_CODE fourth, got %+v", r.Errors[3])
	}
	if r.Errors[4].Row != 5 || r.Errors[4].ColIdx != 2 || r.Errors[4].Code != "B_CODE" {
		t.Errorf("expected row 5 col 2 B_CODE fifth, got %+v", r.Errors[4])
	}
}

func TestWriteOutputCollisionPrevention(t *testing.T) {
	tempDir := t.TempDir()
	inputFile := filepath.Join(tempDir, "input.xlsx")
	if err := os.WriteFile(inputFile, []byte("fake xlsx"), 0644); err != nil {
		t.Fatal(err)
	}

	r := NewReport(inputFile, "Sheet1")

	// Attempting to write to exact input file
	err := WriteOutput(inputFile, FormatJSON, r, inputFile)
	if err == nil || !strings.Contains(err.Error(), "resolves to input file") {
		t.Fatalf("expected collision refusal error, got: %v", err)
	}

	// Symlink pointing to input file
	symlinkFile := filepath.Join(tempDir, "symlink.xlsx")
	if err := os.Symlink(inputFile, symlinkFile); err != nil {
		t.Fatal(err)
	}
	err = WriteOutput(symlinkFile, FormatJSON, r, inputFile)
	if err == nil || !strings.Contains(err.Error(), "resolves to input file") {
		t.Fatalf("expected collision refusal for symlink, got: %v", err)
	}

	// Valid separate destination
	outputFile := filepath.Join(tempDir, "output.json")
	if err := WriteOutput(outputFile, FormatJSON, r, inputFile); err != nil {
		t.Fatalf("unexpected error writing to output file: %v", err)
	}

	// Verify file was written
	data, err := os.ReadFile(outputFile)
	if err != nil {
		t.Fatalf("failed to read output file: %v", err)
	}
	if len(data) == 0 {
		t.Errorf("output file is empty")
	}
}
