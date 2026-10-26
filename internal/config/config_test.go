package config

import (
	"strings"
	"testing"
)

func TestParseValidConfig(t *testing.T) {
	jsonStr := `{
		"sheet": "Employees",
		"headerRow": 1,
		"skipEmptyRows": true,
		"trimSpace": true,
		"columns": {
			"EMP_NO": {
				"type": "string",
				"required": true,
				"unique": true,
				"maxLength": 20,
				"pattern": "^[0-9]+$"
			},
			"FULL_NAME": {
				"type": "string",
				"required": true,
				"minLength": 1,
				"maxLength": 150
			},
			"EMAIL": {
				"type": "string",
				"format": "email"
			},
			"JOIN_DATE": {
				"type": "date",
				"required": true,
				"dateLayouts": ["2006-01-02", "02/01/2006"],
				"allowExcelSerial": true
			},
			"SALARY": {
				"type": "number",
				"min": 0
			},
			"EMP_STATUS": {
				"type": "string",
				"allowedValues": ["PERMANENT", "CONTRACT", "PROBATION"]
			}
		}
	}`

	cfg, err := Parse([]byte(jsonStr))
	if err != nil {
		t.Fatalf("unexpected error parsing valid config: %v", err)
	}

	if cfg.Sheet != "Employees" {
		t.Errorf("expected sheet 'Employees', got %q", cfg.Sheet)
	}
	if cfg.HeaderRow == nil || *cfg.HeaderRow != 1 {
		t.Errorf("expected headerRow 1, got %v", cfg.HeaderRow)
	}
	if cfg.SkipEmptyRows == nil || !*cfg.SkipEmptyRows {
		t.Errorf("expected skipEmptyRows true")
	}
	if cfg.TrimSpace == nil || !*cfg.TrimSpace {
		t.Errorf("expected trimSpace true")
	}
	if len(cfg.Columns) != 6 {
		t.Errorf("expected 6 columns, got %d", len(cfg.Columns))
	}

	empNo := cfg.Columns["EMP_NO"]
	if empNo.Type != TypeString || !empNo.Required || !empNo.Unique || *empNo.MaxLength != 20 || empNo.CompiledPattern == nil {
		t.Errorf("EMP_NO config parsed incorrectly: %+v", empNo)
	}
}

func TestLoadEmployeeRulesExample(t *testing.T) {
	cfg, err := Load("../../examples/employee-rules.json")
	if err != nil {
		t.Fatalf("failed to load examples/employee-rules.json: %v", err)
	}
	if cfg.Sheet != "Employees" {
		t.Errorf("expected sheet 'Employees', got %q", cfg.Sheet)
	}
	if len(cfg.Columns) != 6 {
		t.Errorf("expected 6 columns, got %d", len(cfg.Columns))
	}
}

func TestConfigDefaultsAndExplicitFalse(t *testing.T) {
	// Defaults when omitted
	jsonMin := `{
		"sheet": "Data",
		"columns": {
			"NAME": { "required": false }
		}
	}`
	cfg, err := Parse([]byte(jsonMin))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if *cfg.HeaderRow != 1 {
		t.Errorf("expected default headerRow 1, got %d", *cfg.HeaderRow)
	}
	if !*cfg.SkipEmptyRows {
		t.Errorf("expected default skipEmptyRows true")
	}
	if !*cfg.TrimSpace {
		t.Errorf("expected default trimSpace true")
	}
	if cfg.Columns["NAME"].Type != TypeString {
		t.Errorf("expected default type string, got %s", cfg.Columns["NAME"].Type)
	}

	// Explicit false
	jsonExplicitFalse := `{
		"sheet": "Data",
		"headerRow": 2,
		"skipEmptyRows": false,
		"trimSpace": false,
		"columns": {
			"NAME": { "type": "string" }
		}
	}`
	cfg2, err := Parse([]byte(jsonExplicitFalse))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if *cfg2.HeaderRow != 2 {
		t.Errorf("expected headerRow 2, got %d", *cfg2.HeaderRow)
	}
	if *cfg2.SkipEmptyRows != false {
		t.Errorf("expected skipEmptyRows false, got %v", *cfg2.SkipEmptyRows)
	}
	if *cfg2.TrimSpace != false {
		t.Errorf("expected trimSpace false, got %v", *cfg2.TrimSpace)
	}
}

func TestDuplicateKeysRejected(t *testing.T) {
	// Duplicate top-level key
	jsonDupTop := `{
		"sheet": "Employees",
		"sheet": "Other",
		"columns": { "ID": {} }
	}`
	_, err := Parse([]byte(jsonDupTop))
	if err == nil || !strings.Contains(err.Error(), "duplicate") {
		t.Fatalf("expected duplicate key error, got %v", err)
	}

	// Duplicate column definition
	jsonDupCol := `{
		"sheet": "Employees",
		"columns": {
			"EMP_NO": { "type": "string" },
			"EMP_NO": { "type": "string" }
		}
	}`
	_, err = Parse([]byte(jsonDupCol))
	if err == nil || !strings.Contains(err.Error(), "duplicate") {
		t.Fatalf("expected duplicate key error in columns, got %v", err)
	}
}

func TestUnknownFieldsRejected(t *testing.T) {
	jsonUnknown := `{
		"sheet": "Employees",
		"unknownField": 123,
		"columns": { "ID": {} }
	}`
	_, err := Parse([]byte(jsonUnknown))
	if err == nil || !strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("expected unknown field error, got %v", err)
	}
}

func TestTrailingDataRejected(t *testing.T) {
	jsonTrailing := `{
		"sheet": "Employees",
		"columns": { "ID": {} }
	} { "extra": true }`
	_, err := Parse([]byte(jsonTrailing))
	if err == nil || !strings.Contains(err.Error(), "unexpected trailing data") {
		t.Fatalf("expected trailing data error, got %v", err)
	}
}

func TestEmptyColumnNameRejected(t *testing.T) {
	jsonEmptyCol := `{
		"sheet": "Employees",
		"columns": {
			"": { "type": "string" }
		}
	}`
	_, err := Parse([]byte(jsonEmptyCol))
	if err == nil || !strings.Contains(err.Error(), "column name cannot be empty") {
		t.Fatalf("expected empty column name error, got %v", err)
	}
}

func TestInvalidBoundsAndTypes(t *testing.T) {
	tests := []struct {
		name    string
		jsonStr string
		errMsg  string
	}{
		{
			name: "negative minLength",
			jsonStr: `{
				"sheet": "S",
				"columns": { "A": { "type": "string", "minLength": -1 } }
			}`,
			errMsg: "minLength must be non-negative",
		},
		{
			name: "minLength > maxLength",
			jsonStr: `{
				"sheet": "S",
				"columns": { "A": { "type": "string", "minLength": 10, "maxLength": 5 } }
			}`,
			errMsg: "cannot be greater than maxLength",
		},
		{
			name: "min > max",
			jsonStr: `{
				"sheet": "S",
				"columns": { "A": { "type": "number", "min": 100, "max": 50 } }
			}`,
			errMsg: "cannot be greater than max",
		},
		{
			name: "invalid regex pattern",
			jsonStr: `{
				"sheet": "S",
				"columns": { "A": { "type": "string", "pattern": "[" } }
			}`,
			errMsg: "invalid regexp pattern",
		},
		{
			name: "pattern on number column",
			jsonStr: `{
				"sheet": "S",
				"columns": { "A": { "type": "number", "pattern": "^[0-9]+$" } }
			}`,
			errMsg: "pattern rule is only valid for string",
		},
		{
			name: "min on string column",
			jsonStr: `{
				"sheet": "S",
				"columns": { "A": { "type": "string", "min": 0 } }
			}`,
			errMsg: "min/max rules are only valid for integer and number",
		},
		{
			name: "unknown format",
			jsonStr: `{
				"sheet": "S",
				"columns": { "A": { "type": "string", "format": "phone" } }
			}`,
			errMsg: "unknown format",
		},
		{
			name: "date without layouts and without serial",
			jsonStr: `{
				"sheet": "S",
				"columns": { "A": { "type": "date" } }
			}`,
			errMsg: "date column requires at least one date layout",
		},
		{
			name: "empty sheet",
			jsonStr: `{
				"sheet": "",
				"columns": { "A": {} }
			}`,
			errMsg: "config 'sheet' is required",
		},
		{
			name: "no columns",
			jsonStr: `{
				"sheet": "S",
				"columns": {}
			}`,
			errMsg: "must contain at least one column definition",
		},
		{
			name: "zero headerRow",
			jsonStr: `{
				"sheet": "S",
				"headerRow": 0,
				"columns": { "A": {} }
			}`,
			errMsg: "headerRow must be positive",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Parse([]byte(tt.jsonStr))
			if err == nil {
				t.Fatalf("expected error containing %q, got nil", tt.errMsg)
			}
			if !strings.Contains(err.Error(), tt.errMsg) {
				t.Fatalf("expected error containing %q, got %v", tt.errMsg, err)
			}
		})
	}
}
