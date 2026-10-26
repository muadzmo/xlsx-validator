package config

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"os"
	"regexp"
	"strings"
)

// Supported column types
const (
	TypeString  = "string"
	TypeInteger = "integer"
	TypeNumber  = "number"
	TypeDate    = "date"
	TypeBoolean = "boolean"
)

// Supported string formats
const (
	FormatEmail = "email"
)

// Config represents the top-level validation configuration.
type Config struct {
	Sheet         string                  `json:"sheet"`
	HeaderRow     *int                    `json:"headerRow"`
	SkipEmptyRows *bool                   `json:"skipEmptyRows"`
	TrimSpace     *bool                   `json:"trimSpace"`
	Columns       map[string]ColumnConfig `json:"columns"`
}

// ColumnConfig represents validation rules for a single column.
type ColumnConfig struct {
	Type             string         `json:"type"`
	Required         bool           `json:"required"`
	Unique           bool           `json:"unique"`
	MinLength        *int           `json:"minLength"`
	MaxLength        *int           `json:"maxLength"`
	Pattern          string         `json:"pattern"`
	CompiledPattern  *regexp.Regexp `json:"-"`
	Format           string         `json:"format"`
	DateLayouts      []string       `json:"dateLayouts"`
	AllowExcelSerial bool           `json:"allowExcelSerial"`
	Min              *float64       `json:"min"`
	Max              *float64       `json:"max"`
	AllowedValues    []string       `json:"allowedValues"`
}

// Load reads and validates configuration from a file.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading config file: %w", err)
	}
	return Parse(data)
}

// LoadReader reads and validates configuration from an io.Reader.
func LoadReader(r io.Reader) (*Config, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("reading config: %w", err)
	}
	return Parse(data)
}

// Parse parses and validates configuration JSON data.
func Parse(data []byte) (*Config, error) {
	if len(bytes.TrimSpace(data)) == 0 {
		return nil, fmt.Errorf("config is empty")
	}

	// 1. Check for duplicate keys at any nesting level
	if err := checkDuplicateJSONKeys(data); err != nil {
		return nil, err
	}

	// 2. Decode into Config with unknown fields disallowed
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()

	var cfg Config
	if err := dec.Decode(&cfg); err != nil {
		return nil, fmt.Errorf("parsing config JSON: %w", err)
	}

	// 3. Ensure no trailing tokens
	if dec.More() {
		return nil, fmt.Errorf("unexpected trailing data after JSON object")
	}
	var extra json.RawMessage
	if err := dec.Decode(&extra); err != io.EOF {
		return nil, fmt.Errorf("unexpected trailing data after JSON object")
	}

	// 4. Validate and apply defaults
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	return &cfg, nil
}

// Validate applies defaults and verifies all configuration constraints.
func (c *Config) Validate() error {
	if strings.TrimSpace(c.Sheet) == "" {
		return fmt.Errorf("config 'sheet' is required")
	}

	if len(c.Columns) == 0 {
		return fmt.Errorf("config 'columns' must contain at least one column definition")
	}

	// Defaults: headerRow = 1
	if c.HeaderRow == nil {
		defaultHeaderRow := 1
		c.HeaderRow = &defaultHeaderRow
	} else if *c.HeaderRow <= 0 {
		return fmt.Errorf("headerRow must be positive (>= 1), got %d", *c.HeaderRow)
	}

	// Defaults: skipEmptyRows = true
	if c.SkipEmptyRows == nil {
		defaultSkipEmptyRows := true
		c.SkipEmptyRows = &defaultSkipEmptyRows
	}

	// Defaults: trimSpace = true
	if c.TrimSpace == nil {
		defaultTrimSpace := true
		c.TrimSpace = &defaultTrimSpace
	}

	for colName, col := range c.Columns {
		if strings.TrimSpace(colName) == "" {
			return fmt.Errorf("column name cannot be empty")
		}

		// Default type: string
		if col.Type == "" {
			col.Type = TypeString
		}

		switch col.Type {
		case TypeString, TypeInteger, TypeNumber, TypeDate, TypeBoolean:
		default:
			return fmt.Errorf("column %q: unknown type %q", colName, col.Type)
		}

		// String-only rules validation
		if col.Type != TypeString {
			if col.MinLength != nil {
				return fmt.Errorf("column %q: minLength rule is only valid for string columns", colName)
			}
			if col.MaxLength != nil {
				return fmt.Errorf("column %q: maxLength rule is only valid for string columns", colName)
			}
			if col.Pattern != "" {
				return fmt.Errorf("column %q: pattern rule is only valid for string columns", colName)
			}
			if col.Format != "" {
				return fmt.Errorf("column %q: format rule is only valid for string columns", colName)
			}
		} else {
			if col.MinLength != nil && *col.MinLength < 0 {
				return fmt.Errorf("column %q: minLength must be non-negative, got %d", colName, *col.MinLength)
			}
			if col.MaxLength != nil && *col.MaxLength < 0 {
				return fmt.Errorf("column %q: maxLength must be non-negative, got %d", colName, *col.MaxLength)
			}
			if col.MinLength != nil && col.MaxLength != nil && *col.MinLength > *col.MaxLength {
				return fmt.Errorf("column %q: minLength (%d) cannot be greater than maxLength (%d)", colName, *col.MinLength, *col.MaxLength)
			}
			if col.Pattern != "" {
				re, err := regexp.Compile(col.Pattern)
				if err != nil {
					return fmt.Errorf("column %q: invalid regexp pattern %q: %w", colName, col.Pattern, err)
				}
				col.CompiledPattern = re
			}
			if col.Format != "" {
				if col.Format != FormatEmail {
					return fmt.Errorf("column %q: unknown format %q", colName, col.Format)
				}
			}
		}

		// Numeric-only rules validation
		if col.Type != TypeInteger && col.Type != TypeNumber {
			if col.Min != nil || col.Max != nil {
				return fmt.Errorf("column %q: min/max rules are only valid for integer and number columns", colName)
			}
		} else {
			if col.Min != nil {
				if math.IsNaN(*col.Min) || math.IsInf(*col.Min, 0) {
					return fmt.Errorf("column %q: min must be a finite number", colName)
				}
			}
			if col.Max != nil {
				if math.IsNaN(*col.Max) || math.IsInf(*col.Max, 0) {
					return fmt.Errorf("column %q: max must be a finite number", colName)
				}
			}
			if col.Min != nil && col.Max != nil && *col.Min > *col.Max {
				return fmt.Errorf("column %q: min (%v) cannot be greater than max (%v)", colName, *col.Min, *col.Max)
			}
		}

		// Date-only rules validation
		if col.Type != TypeDate {
			if len(col.DateLayouts) > 0 {
				return fmt.Errorf("column %q: dateLayouts is only valid for date columns", colName)
			}
			if col.AllowExcelSerial {
				return fmt.Errorf("column %q: allowExcelSerial is only valid for date columns", colName)
			}
		} else {
			if len(col.DateLayouts) == 0 && !col.AllowExcelSerial {
				return fmt.Errorf("column %q: date column requires at least one date layout when allowExcelSerial is false", colName)
			}
			for _, layout := range col.DateLayouts {
				if strings.TrimSpace(layout) == "" {
					return fmt.Errorf("column %q: dateLayouts cannot contain empty layout string", colName)
				}
			}
		}

		c.Columns[colName] = col
	}

	return nil
}

// checkDuplicateJSONKeys scans the JSON token stream and detects duplicate object keys.
func checkDuplicateJSONKeys(data []byte) error {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()

	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("invalid JSON syntax: %w", err)
		}

		if delim, ok := tok.(json.Delim); ok {
			if delim == '{' {
				if err := parseObjectKeys(dec); err != nil {
					return err
				}
			} else if delim == '[' {
				if err := parseArrayElements(dec); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func parseObjectKeys(dec *json.Decoder) error {
	seenKeys := make(map[string]bool)
	for dec.More() {
		keyTok, err := dec.Token()
		if err != nil {
			return fmt.Errorf("invalid JSON object key: %w", err)
		}
		key, ok := keyTok.(string)
		if !ok {
			return fmt.Errorf("expected string object key, got %v", keyTok)
		}
		if seenKeys[key] {
			return fmt.Errorf("duplicate JSON key %q in object", key)
		}
		seenKeys[key] = true

		// Parse the value
		valTok, err := dec.Token()
		if err != nil {
			return fmt.Errorf("invalid JSON value: %w", err)
		}
		if delim, ok := valTok.(json.Delim); ok {
			switch delim {
			case '{':
				if err := parseObjectKeys(dec); err != nil {
					return err
				}
			case '[':
				if err := parseArrayElements(dec); err != nil {
					return err
				}
			}
		}
	}

	// Consume closing '}'
	endTok, err := dec.Token()
	if err != nil {
		return fmt.Errorf("reading closing object delimiter: %w", err)
	}
	if delim, ok := endTok.(json.Delim); !ok || delim != '}' {
		return fmt.Errorf("expected '}', got %v", endTok)
	}
	return nil
}

func parseArrayElements(dec *json.Decoder) error {
	for dec.More() {
		tok, err := dec.Token()
		if err != nil {
			return fmt.Errorf("invalid JSON array item: %w", err)
		}
		if delim, ok := tok.(json.Delim); ok {
			switch delim {
			case '{':
				if err := parseObjectKeys(dec); err != nil {
					return err
				}
			case '[':
				if err := parseArrayElements(dec); err != nil {
					return err
				}
			}
		}
	}

	// Consume closing ']'
	endTok, err := dec.Token()
	if err != nil {
		return fmt.Errorf("reading closing array delimiter: %w", err)
	}
	if delim, ok := endTok.(json.Delim); !ok || delim != ']' {
		return fmt.Errorf("expected ']', got %v", endTok)
	}
	return nil
}
