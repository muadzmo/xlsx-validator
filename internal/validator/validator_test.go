package validator

import (
	"path/filepath"
	"testing"

	"github.com/xuri/excelize/v2"

	"xlsx-validator/internal/config"
	"xlsx-validator/internal/report"
)

func intPtr(i int) *int           { return &i }
func floatPtr(f float64) *float64 { return &f }
func boolPtr(b bool) *bool        { return &b }

func TestValidatorValidWorkbook(t *testing.T) {
	f := excelize.NewFile()
	sheet := "Employees"
	f.NewSheet(sheet)
	f.DeleteSheet("Sheet1")

	// Headers
	_ = f.SetCellValue(sheet, "A1", "EMP_NO")
	_ = f.SetCellValue(sheet, "B1", "FULL_NAME")
	_ = f.SetCellValue(sheet, "C1", "EMAIL")
	_ = f.SetCellValue(sheet, "D1", "JOIN_DATE")
	_ = f.SetCellValue(sheet, "E1", "SALARY")
	_ = f.SetCellValue(sheet, "F1", "EMP_STATUS")

	// Row 2: valid
	_ = f.SetCellValue(sheet, "A2", "1001")
	_ = f.SetCellValue(sheet, "B2", "Budi Santoso")
	_ = f.SetCellValue(sheet, "C2", "budi@example.com")
	_ = f.SetCellValue(sheet, "D2", "2023-01-15")
	_ = f.SetCellValue(sheet, "E2", 5000000)
	_ = f.SetCellValue(sheet, "F2", "PERMANENT")

	// Row 3: valid with serial date
	_ = f.SetCellValue(sheet, "A3", "1002")
	_ = f.SetCellValue(sheet, "B3", "Siti Aminah")
	_ = f.SetCellValue(sheet, "C3", "siti@example.com")
	// 44941 is ~2023-01-16
	_ = f.SetCellValue(sheet, "D3", 44941)
	_ = f.SetCellValue(sheet, "E3", 6500000)
	_ = f.SetCellValue(sheet, "F3", "CONTRACT")

	// Row 4: blank row to test skipEmptyRows
	// Row 5: valid
	_ = f.SetCellValue(sheet, "A5", "1003")
	_ = f.SetCellValue(sheet, "B5", "Andi Wijaya")
	_ = f.SetCellValue(sheet, "C5", "") // optional email
	_ = f.SetCellValue(sheet, "D5", "15/02/2023")
	_ = f.SetCellValue(sheet, "E5", 0) // min 0 allowed
	_ = f.SetCellValue(sheet, "F5", "PROBATION")

	tmp := filepath.Join(t.TempDir(), "valid.xlsx")
	if err := f.SaveAs(tmp); err != nil {
		t.Fatal(err)
	}

	cfg := &config.Config{
		Sheet:         sheet,
		HeaderRow:     intPtr(1),
		SkipEmptyRows: boolPtr(true),
		TrimSpace:     boolPtr(true),
		Columns: map[string]config.ColumnConfig{
			"EMP_NO": {
				Type:      config.TypeString,
				Required:  true,
				Unique:    true,
				MaxLength: intPtr(20),
				Pattern:   "^[0-9]+$",
			},
			"FULL_NAME": {
				Type:      config.TypeString,
				Required:  true,
				MinLength: intPtr(1),
				MaxLength: intPtr(150),
			},
			"EMAIL": {
				Type:   config.TypeString,
				Format: config.FormatEmail,
			},
			"JOIN_DATE": {
				Type:             config.TypeDate,
				Required:         true,
				DateLayouts:      []string{"2006-01-02", "02/01/2006"},
				AllowExcelSerial: true,
			},
			"SALARY": {
				Type: config.TypeNumber,
				Min:  floatPtr(0),
			},
			"EMP_STATUS": {
				Type:          config.TypeString,
				AllowedValues: []string{"PERMANENT", "CONTRACT", "PROBATION"},
			},
		},
	}
	if err := cfg.Validate(); err != nil {
		t.Fatal(err)
	}

	rep, err := ValidateFile(tmp, cfg)
	if err != nil {
		t.Fatalf("unexpected validation error: %v", err)
	}

	if rep.Summary.ErrorCount != 0 {
		t.Fatalf("expected 0 errors, got %d: %+v", rep.Summary.ErrorCount, rep.Errors)
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
	if rep.Summary.SkippedRows != 1 {
		t.Errorf("expected 1 skipped row, got %d", rep.Summary.SkippedRows)
	}
}

func TestStructureViolations(t *testing.T) {
	t.Run("MissingSheet", func(t *testing.T) {
		f := excelize.NewFile()
		tmp := filepath.Join(t.TempDir(), "missing_sheet.xlsx")
		_ = f.SaveAs(tmp)

		cfg := &config.Config{
			Sheet: "NonExistent",
			Columns: map[string]config.ColumnConfig{
				"A": {Type: config.TypeString},
			},
		}
		_ = cfg.Validate()

		rep, err := ValidateFile(tmp, cfg)
		if err != nil {
			t.Fatal(err)
		}
		if rep.Summary.StructureErrorCount != 1 || len(rep.Errors) != 1 {
			t.Fatalf("expected 1 structure error, got %d", len(rep.Errors))
		}
		if rep.Errors[0].Code != report.CodeSheetNotFound {
			t.Errorf("expected SHEET_NOT_FOUND, got %s", rep.Errors[0].Code)
		}
		if rep.Errors[0].Row != 0 || rep.Errors[0].Cell != "" || rep.Errors[0].Column != "" {
			t.Errorf("structure violation without cell should have row 0 and empty cell/column, got %+v", rep.Errors[0])
		}
	})

	t.Run("MissingHeader", func(t *testing.T) {
		f := excelize.NewFile()
		sheet := "Sheet1"
		_ = f.SetCellValue(sheet, "A1", "EMP_NO")
		// Missing FULL_NAME
		tmp := filepath.Join(t.TempDir(), "missing_header.xlsx")
		_ = f.SaveAs(tmp)

		cfg := &config.Config{
			Sheet: sheet,
			Columns: map[string]config.ColumnConfig{
				"EMP_NO":    {Type: config.TypeString},
				"FULL_NAME": {Type: config.TypeString},
			},
		}
		_ = cfg.Validate()

		rep, err := ValidateFile(tmp, cfg)
		if err != nil {
			t.Fatal(err)
		}
		if rep.Summary.StructureErrorCount != 1 || len(rep.Errors) != 1 {
			t.Fatalf("expected 1 structure error, got %d", len(rep.Errors))
		}
		if rep.Errors[0].Code != report.CodeMissingHeader {
			t.Errorf("expected MISSING_HEADER, got %s", rep.Errors[0].Code)
		}
		if rep.Errors[0].Row != 0 || rep.Errors[0].Cell != "" || rep.Errors[0].Column != "" {
			t.Errorf("expected row 0, empty cell and column, got %+v", rep.Errors[0])
		}
	})

	t.Run("DuplicateHeader", func(t *testing.T) {
		f := excelize.NewFile()
		sheet := "Sheet1"
		_ = f.SetCellValue(sheet, "A1", "EMP_NO")
		_ = f.SetCellValue(sheet, "B1", "EMP_NO") // duplicate
		tmp := filepath.Join(t.TempDir(), "dup_header.xlsx")
		_ = f.SaveAs(tmp)

		cfg := &config.Config{
			Sheet: sheet,
			Columns: map[string]config.ColumnConfig{
				"EMP_NO": {Type: config.TypeString},
			},
		}
		_ = cfg.Validate()

		rep, err := ValidateFile(tmp, cfg)
		if err != nil {
			t.Fatal(err)
		}
		if rep.Summary.StructureErrorCount != 1 || len(rep.Errors) != 1 {
			t.Fatalf("expected 1 structure error, got %d", len(rep.Errors))
		}
		if rep.Errors[0].Code != report.CodeDuplicateHeader {
			t.Errorf("expected DUPLICATE_HEADER, got %s", rep.Errors[0].Code)
		}
		if rep.Errors[0].Cell != "B1" {
			t.Errorf("expected cell B1, got %s", rep.Errors[0].Cell)
		}
	})

	t.Run("FormulaHeader", func(t *testing.T) {
		f := excelize.NewFile()
		sheet := "Sheet1"
		_ = f.SetCellFormula(sheet, "A1", "CONCAT(\"EMP\",\"_NO\")")
		tmp := filepath.Join(t.TempDir(), "formula_header.xlsx")
		_ = f.SaveAs(tmp)

		cfg := &config.Config{
			Sheet: sheet,
			Columns: map[string]config.ColumnConfig{
				"EMP_NO": {Type: config.TypeString},
			},
		}
		_ = cfg.Validate()

		rep, err := ValidateFile(tmp, cfg)
		if err != nil {
			t.Fatal(err)
		}
		foundFormula := false
		for _, err := range rep.Errors {
			if err.Code == report.CodeFormulaHeader {
				foundFormula = true
			}
		}
		if !foundFormula {
			t.Fatalf("expected FORMULA_HEADER error in %+v", rep.Errors)
		}
	})
}

func TestDataViolationsAndEdgeCases(t *testing.T) {
	f := excelize.NewFile()
	sheet := "Data"
	f.NewSheet(sheet)
	f.DeleteSheet("Sheet1")

	// Headers
	_ = f.SetCellValue(sheet, "A1", "CODE")
	_ = f.SetCellValue(sheet, "B1", "NAME")
	_ = f.SetCellValue(sheet, "C1", "AGE")
	_ = f.SetCellValue(sheet, "D1", "IS_ACTIVE")
	_ = f.SetCellValue(sheet, "E1", "START_DATE")

	// Row 2: Duplicate CODE, unicode name, valid age, valid bool, valid date
	_ = f.SetCellValue(sheet, "A2", "C001")
	_ = f.SetCellValue(sheet, "B2", "Sekar 🌸") // Unicode 7 runes
	_ = f.SetCellValue(sheet, "C2", "25.0")    // mathematically integral
	_ = f.SetCellValue(sheet, "D2", true)      // native bool
	_ = f.SetCellValue(sheet, "E2", "2024-01-01")

	// Row 3: Duplicate CODE, name too short, age invalid (non-integral), invalid bool, invalid date
	_ = f.SetCellValue(sheet, "A3", "C001")
	_ = f.SetCellValue(sheet, "B3", "") // Required violation
	_ = f.SetCellValue(sheet, "C3", "25.5")
	_ = f.SetCellValue(sheet, "D3", "maybe")
	_ = f.SetCellValue(sheet, "E3", "invalid-date")

	// Row 4: Formula in data cell
	_ = f.SetCellValue(sheet, "A4", "C002")
	_ = f.SetCellFormula(sheet, "B4", "A4&\"_X\"")
	_ = f.SetCellValue(sheet, "C4", 30)
	_ = f.SetCellValue(sheet, "D4", "FALSE")
	_ = f.SetCellValue(sheet, "E4", "2024-02-01")

	// Row 5: Excel error cell
	_ = f.SetCellValue(sheet, "A5", "C003")
	_ = f.SetCellValue(sheet, "B5", "John Doe")
	_ = f.SetCellValue(sheet, "C5", "#VALUE!")
	_ = f.SetCellValue(sheet, "D5", "true")
	_ = f.SetCellValue(sheet, "E5", "2024-03-01")

	// Row 6: Fictitious 1900-02-29 date serial (serial 60 in 1900 date system)
	_ = f.SetCellValue(sheet, "A6", "C004")
	_ = f.SetCellValue(sheet, "B6", "Jane Doe")
	_ = f.SetCellValue(sheet, "C6", 40)
	_ = f.SetCellValue(sheet, "D6", "True")
	_ = f.SetCellValue(sheet, "E6", 60) // Fictitious 1900-02-29

	tmp := filepath.Join(t.TempDir(), "data_violations.xlsx")
	if err := f.SaveAs(tmp); err != nil {
		t.Fatal(err)
	}

	cfg := &config.Config{
		Sheet: sheet,
		Columns: map[string]config.ColumnConfig{
			"CODE": {
				Type:     config.TypeString,
				Required: true,
				Unique:   true,
			},
			"NAME": {
				Type:      config.TypeString,
				Required:  true,
				MinLength: intPtr(3),
				MaxLength: intPtr(10),
			},
			"AGE": {
				Type: config.TypeInteger,
				Min:  floatPtr(18),
				Max:  floatPtr(65),
			},
			"IS_ACTIVE": {
				Type: config.TypeBoolean,
			},
			"START_DATE": {
				Type:             config.TypeDate,
				DateLayouts:      []string{"2006-01-02"},
				AllowExcelSerial: true,
			},
		},
	}
	_ = cfg.Validate()

	rep, err := ValidateFile(tmp, cfg)
	if err != nil {
		t.Fatal(err)
	}

	codeCount := make(map[string]int)
	for _, err := range rep.Errors {
		codeCount[err.Code]++
	}

	// Check that DUPLICATE flagged both row 2 and row 3
	if codeCount[report.CodeDuplicate] != 2 {
		t.Errorf("expected 2 DUPLICATE errors (for row 2 and 3), got %d", codeCount[report.CodeDuplicate])
	}
	// Check REQUIRED on row 3
	if codeCount[report.CodeRequired] < 1 {
		t.Errorf("expected at least 1 REQUIRED error")
	}
	// Check INVALID_TYPE on row 3 (age 25.5 not integer, bool 'maybe', date 'invalid-date')
	if codeCount[report.CodeInvalidType] < 3 {
		t.Errorf("expected at least 3 INVALID_TYPE errors, got %d", codeCount[report.CodeInvalidType])
	}
	// Check FORMULA_NOT_ALLOWED on row 4
	if codeCount[report.CodeFormulaNotAllowed] != 1 {
		t.Errorf("expected 1 FORMULA_NOT_ALLOWED error, got %d", codeCount[report.CodeFormulaNotAllowed])
	}
	// Check EXCEL_ERROR on row 5
	if codeCount[report.CodeExcelError] != 1 {
		t.Errorf("expected 1 EXCEL_ERROR error, got %d", codeCount[report.CodeExcelError])
	}

	// Verify check rows and sum
	if rep.Summary.CheckedRows != 5 {
		t.Errorf("expected 5 checked rows, got %d", rep.Summary.CheckedRows)
	}
	if rep.Summary.ValidRows+rep.Summary.InvalidRows != rep.Summary.CheckedRows {
		t.Errorf("expected validRows + invalidRows == checkedRows, got %d + %d != %d", rep.Summary.ValidRows, rep.Summary.InvalidRows, rep.Summary.CheckedRows)
	}
}

func TestLeadingZeroesPreserved(t *testing.T) {
	f := excelize.NewFile()
	sheet := "Sheet1"
	_ = f.SetCellValue(sheet, "A1", "ID")

	// Set formatted number format 00000 for cell A2 with value 123 -> displayed as 00123
	styleID, err := f.NewStyle(&excelize.Style{
		CustomNumFmt: strPtr("00000"),
	})
	if err != nil {
		t.Fatal(err)
	}
	_ = f.SetCellValue(sheet, "A2", 123)
	_ = f.SetCellStyle(sheet, "A2", "A2", styleID)

	tmp := filepath.Join(t.TempDir(), "leading_zero.xlsx")
	_ = f.SaveAs(tmp)

	cfg := &config.Config{
		Sheet: sheet,
		Columns: map[string]config.ColumnConfig{
			"ID": {
				Type:      config.TypeString,
				Pattern:   "^00123$",
				MinLength: intPtr(5),
			},
		},
	}
	_ = cfg.Validate()

	rep, err := ValidateFile(tmp, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if rep.Summary.ErrorCount != 0 {
		t.Errorf("expected 0 errors when formatted leading zeroes are preserved, got: %+v", rep.Errors)
	}
}

func Test1904DateSystem(t *testing.T) {
	f := excelize.NewFile()
	sheet := "Sheet1"
	_ = f.SetCellValue(sheet, "A1", "DATE_COL")

	// Set 1904 date system
	_ = f.SetWorkbookProps(&excelize.WorkbookPropsOptions{
		Date1904: boolPtr(true),
	})

	// Day 0 in 1904 date system is 1904-01-01
	_ = f.SetCellValue(sheet, "A2", 0)

	tmp := filepath.Join(t.TempDir(), "1904.xlsx")
	_ = f.SaveAs(tmp)

	cfg := &config.Config{
		Sheet: sheet,
		Columns: map[string]config.ColumnConfig{
			"DATE_COL": {
				Type:             config.TypeDate,
				AllowExcelSerial: true,
			},
		},
	}
	_ = cfg.Validate()

	rep, err := ValidateFile(tmp, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if rep.Summary.ErrorCount != 0 {
		t.Errorf("expected 0 errors for serial 0 in 1904 date system, got: %+v", rep.Errors)
	}
}

func TestEmailValidation(t *testing.T) {
	f := excelize.NewFile()
	sheet := "Sheet1"
	_ = f.SetCellValue(sheet, "A1", "EMAIL")

	// Row 2: valid email
	_ = f.SetCellValue(sheet, "A2", "user@example.com")
	// Row 3: invalid email
	_ = f.SetCellValue(sheet, "A3", "plainaddress")
	// Row 4: display name email (should be rejected as non-bare email)
	_ = f.SetCellValue(sheet, "A4", "John Doe <john@example.com>")

	tmp := filepath.Join(t.TempDir(), "email.xlsx")
	_ = f.SaveAs(tmp)

	cfg := &config.Config{
		Sheet: sheet,
		Columns: map[string]config.ColumnConfig{
			"EMAIL": {
				Type:   config.TypeString,
				Format: config.FormatEmail,
			},
		},
	}
	_ = cfg.Validate()

	rep, err := ValidateFile(tmp, cfg)
	if err != nil {
		t.Fatal(err)
	}

	if rep.Summary.ErrorCount != 2 {
		t.Fatalf("expected 2 errors (for row 3 and 4), got %d: %+v", rep.Summary.ErrorCount, rep.Errors)
	}
	if rep.Errors[0].Row != 3 || rep.Errors[0].Code != report.CodeInvalidFormat {
		t.Errorf("expected row 3 INVALID_FORMAT, got %+v", rep.Errors[0])
	}
	if rep.Errors[1].Row != 4 || rep.Errors[1].Code != report.CodeInvalidFormat {
		t.Errorf("expected row 4 INVALID_FORMAT, got %+v", rep.Errors[1])
	}
}

func TestUnconfiguredColumnPreventsRowSkip(t *testing.T) {
	f := excelize.NewFile()
	sheet := "Sheet1"
	_ = f.SetCellValue(sheet, "A1", "REQ_COL")
	// Unconfigured column B
	_ = f.SetCellValue(sheet, "B1", "NOTE")

	// Row 2: REQ_COL is empty, but unconfigured column B has data!
	_ = f.SetCellValue(sheet, "A2", "")
	_ = f.SetCellValue(sheet, "B2", "Some unconfigured text")

	tmp := filepath.Join(t.TempDir(), "unconfigured.xlsx")
	_ = f.SaveAs(tmp)

	cfg := &config.Config{
		Sheet:         sheet,
		SkipEmptyRows: boolPtr(true),
		Columns: map[string]config.ColumnConfig{
			"REQ_COL": {
				Type:     config.TypeString,
				Required: true,
			},
		},
	}
	_ = cfg.Validate()

	rep, err := ValidateFile(tmp, cfg)
	if err != nil {
		t.Fatal(err)
	}

	// Because B2 has text, row 2 was NOT skipped!
	// And because REQ_COL is required and empty, it emits REQUIRED violation!
	if rep.Summary.SkippedRows != 0 {
		t.Errorf("expected 0 skipped rows because unconfigured column has text, got %d", rep.Summary.SkippedRows)
	}
	if rep.Summary.ErrorCount != 1 || rep.Errors[0].Code != report.CodeRequired {
		t.Fatalf("expected 1 REQUIRED error, got %+v", rep.Errors)
	}
}

func TestSkipEmptyRowsFalse(t *testing.T) {
	f := excelize.NewFile()
	sheet := "Sheet1"
	_ = f.SetCellValue(sheet, "A1", "REQ_COL")

	// Row 2 is completely blank
	// Row 3 has value
	_ = f.SetCellValue(sheet, "A3", "val")

	tmp := filepath.Join(t.TempDir(), "skip_false.xlsx")
	_ = f.SaveAs(tmp)

	cfg := &config.Config{
		Sheet:         sheet,
		SkipEmptyRows: boolPtr(false),
		Columns: map[string]config.ColumnConfig{
			"REQ_COL": {
				Type:     config.TypeString,
				Required: true,
			},
		},
	}
	_ = cfg.Validate()

	rep, err := ValidateFile(tmp, cfg)
	if err != nil {
		t.Fatal(err)
	}

	if rep.Summary.SkippedRows != 0 {
		t.Errorf("expected 0 skipped rows when skipEmptyRows=false, got %d", rep.Summary.SkippedRows)
	}
	if rep.Summary.CheckedRows != 2 {
		t.Errorf("expected 2 checked rows, got %d", rep.Summary.CheckedRows)
	}
	if rep.Summary.ErrorCount != 1 || rep.Errors[0].Row != 2 || rep.Errors[0].Code != report.CodeRequired {
		t.Errorf("expected REQUIRED error on row 2, got %+v", rep.Errors)
	}
}

func strPtr(s string) *string {
	return &s
}
