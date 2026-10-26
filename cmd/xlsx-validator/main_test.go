package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/xuri/excelize/v2"
)

func createTestWorkbook(t *testing.T, dir, filename string, withErrors bool) string {
	f := excelize.NewFile()
	sheet := "Employees"
	f.NewSheet(sheet)
	f.DeleteSheet("Sheet1")

	_ = f.SetCellValue(sheet, "A1", "EMP_NO")
	_ = f.SetCellValue(sheet, "B1", "FULL_NAME")

	_ = f.SetCellValue(sheet, "A2", "101")
	_ = f.SetCellValue(sheet, "B2", "Alice")

	if withErrors {
		// Row 3: duplicate EMP_NO and missing FULL_NAME
		_ = f.SetCellValue(sheet, "A3", "101")
		_ = f.SetCellValue(sheet, "B3", "")
	}

	path := filepath.Join(dir, filename)
	if err := f.SaveAs(path); err != nil {
		t.Fatal(err)
	}
	return path
}

func createTestConfig(t *testing.T, dir string) string {
	content := `{
		"sheet": "Employees",
		"columns": {
			"EMP_NO": {
				"type": "string",
				"required": true,
				"unique": true
			},
			"FULL_NAME": {
				"type": "string",
				"required": true
			}
		}
	}`
	path := filepath.Join(dir, "rules.json")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestCLIOptionsAndExitCodes(t *testing.T) {
	tempDir := t.TempDir()
	cfgPath := createTestConfig(t, tempDir)
	validXLSX := createTestWorkbook(t, tempDir, "valid.xlsx", false)
	invalidXLSX := createTestWorkbook(t, tempDir, "invalid.xlsx", true)

	t.Run("HelpFlagExitsZero", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		code := run([]string{"--help"}, &stdout, &stderr)
		if code != 0 {
			t.Errorf("expected exit code 0 on --help, got %d", code)
		}
		if !strings.Contains(stdout.String(), "xlsx-validator - generic .xlsx workbook validator") {
			t.Errorf("expected usage on stdout, got: %s", stdout.String())
		}
	})

	t.Run("MissingConfigExitsTwo", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		code := run([]string{validXLSX}, &stdout, &stderr)
		if code != 2 {
			t.Errorf("expected exit code 2, got %d", code)
		}
		if !strings.Contains(stderr.String(), "--config flag is required") {
			t.Errorf("expected config error in stderr, got: %s", stderr.String())
		}
	})

	t.Run("MissingWorkbookExitsTwo", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		code := run([]string{"--config", cfgPath}, &stdout, &stderr)
		if code != 2 {
			t.Errorf("expected exit code 2, got %d", code)
		}
		if !strings.Contains(stderr.String(), "missing workbook file argument") {
			t.Errorf("expected workbook argument error, got: %s", stderr.String())
		}
	})

	t.Run("InvalidFormatExitsTwo", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		code := run([]string{"--config", cfgPath, "--format", "xml", validXLSX}, &stdout, &stderr)
		if code != 2 {
			t.Errorf("expected exit code 2, got %d", code)
		}
	})

	t.Run("ValidWorkbookExitsZero", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		code := run([]string{"--config", cfgPath, validXLSX}, &stdout, &stderr)
		if code != 0 {
			t.Errorf("expected exit code 0, got %d. stderr: %s", code, stderr.String())
		}
		if !strings.Contains(stdout.String(), "PASSED (0 violations)") {
			t.Errorf("expected passed report, got: %s", stdout.String())
		}
	})

	t.Run("InvalidWorkbookExitsOne", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		code := run([]string{"--config", cfgPath, invalidXLSX}, &stdout, &stderr)
		if code != 1 {
			t.Errorf("expected exit code 1, got %d", code)
		}
		if !strings.Contains(stdout.String(), "FAILED (3 violations)") {
			t.Errorf("expected failed report with 3 violations (2 duplicate + 1 required), got: %s", stdout.String())
		}
	})

	t.Run("JSONOutputToStdout", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		code := run([]string{"--config", cfgPath, "--format", "json", validXLSX}, &stdout, &stderr)
		if code != 0 {
			t.Fatalf("expected exit code 0, got %d", code)
		}
		if !strings.Contains(stdout.String(), `"checkedRows": 1`) {
			t.Errorf("expected JSON output with checkedRows 1, got: %s", stdout.String())
		}
	})

	t.Run("FileOutputAndCollisionPrevention", func(t *testing.T) {
		outPath := filepath.Join(tempDir, "result.json")
		var stdout, stderr bytes.Buffer
		code := run([]string{"--config", cfgPath, "--format", "json", "--output", outPath, validXLSX}, &stdout, &stderr)
		if code != 0 {
			t.Fatalf("expected exit code 0, got %d. stderr: %s", code, stderr.String())
		}
		if stdout.Len() != 0 {
			t.Errorf("expected clean stdout when output file is specified, got: %s", stdout.String())
		}

		// Verify file written
		data, err := os.ReadFile(outPath)
		if err != nil || len(data) == 0 {
			t.Fatalf("failed reading output file: %v", err)
		}

		// Attempt writing to workbook path should fail and exit 2
		var colStdout, colStderr bytes.Buffer
		colCode := run([]string{"--config", cfgPath, "--format", "json", "--output", validXLSX, validXLSX}, &colStdout, &colStderr)
		if colCode != 2 {
			t.Errorf("expected collision refusal exit code 2, got %d", colCode)
		}
	})
}
