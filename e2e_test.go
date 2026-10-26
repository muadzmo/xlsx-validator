package main_test

import (
	"testing"

	"xlsx-validator/internal/config"
	"xlsx-validator/internal/report"
	"xlsx-validator/internal/validator"
)

func TestEndToEndWorkbooks(t *testing.T) {
	cfg, err := config.Load("examples/employee-rules.json")
	if err != nil {
		t.Fatalf("failed to load example config: %v", err)
	}

	t.Run("ValidEmployeesWorkbook", func(t *testing.T) {
		rep, err := validator.ValidateFile("testdata/valid_employees.xlsx", cfg)
		if err != nil {
			t.Fatalf("unexpected validation error: %v", err)
		}

		if rep.Summary.CheckedRows != 3 {
			t.Errorf("expected 3 checked rows, got %d", rep.Summary.CheckedRows)
		}
		if rep.Summary.ValidRows != 3 {
			t.Errorf("expected 3 valid rows, got %d", rep.Summary.ValidRows)
		}
		if rep.Summary.InvalidRows != 0 {
			t.Errorf("expected 0 invalid rows, got %d", rep.Summary.InvalidRows)
		}
		if rep.Summary.SkippedRows != 0 {
			t.Errorf("expected 0 skipped rows, got %d", rep.Summary.SkippedRows)
		}
		if rep.Summary.ErrorCount != 0 {
			t.Errorf("expected 0 errors, got %d: %+v", rep.Summary.ErrorCount, rep.Errors)
		}
	})

	t.Run("InvalidEmployeesWorkbook", func(t *testing.T) {
		rep, err := validator.ValidateFile("testdata/invalid_employees.xlsx", cfg)
		if err != nil {
			t.Fatalf("unexpected validation error: %v", err)
		}

		// Expected summary counts:
		// Checked rows: 3 (rows 2, 3, 5)
		// Skipped rows: 1 (row 4 blank)
		// Invalid rows: 3 (all 3 checked rows have violations)
		// Valid rows: 0
		// Total errors: 8
		if rep.Summary.CheckedRows != 3 {
			t.Errorf("expected 3 checked rows, got %d", rep.Summary.CheckedRows)
		}
		if rep.Summary.ValidRows != 0 {
			t.Errorf("expected 0 valid rows, got %d", rep.Summary.ValidRows)
		}
		if rep.Summary.InvalidRows != 3 {
			t.Errorf("expected 3 invalid rows, got %d", rep.Summary.InvalidRows)
		}
		if rep.Summary.SkippedRows != 1 {
			t.Errorf("expected 1 skipped row, got %d", rep.Summary.SkippedRows)
		}
		if rep.Summary.ErrorCount != 8 {
			t.Errorf("expected 8 errors, got %d: %+v", rep.Summary.ErrorCount, rep.Errors)
		}

		// Verify specific error codes present
		codeCount := make(map[string]int)
		for _, v := range rep.Errors {
			codeCount[v.Code]++
		}

		expectedCodes := map[string]int{
			report.CodeDuplicate:       2, // Row 2 and Row 3 EMP_NO '1001'
			report.CodeRequired:        1, // Row 3 FULL_NAME
			report.CodeInvalidFormat:   1, // Row 3 EMAIL
			report.CodeInvalidType:     1, // Row 3 JOIN_DATE
			report.CodeMinValue:        1, // Row 3 SALARY
			report.CodeNotAllowed:      1, // Row 3 EMP_STATUS
			report.CodePatternMismatch: 1, // Row 5 EMP_NO 'EMP_ABC'
		}

		for code, expectedCount := range expectedCodes {
			if count := codeCount[code]; count != expectedCount {
				t.Errorf("expected %d %s errors, got %d", expectedCount, code, count)
			}
		}
	})
}
