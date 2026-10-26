# xlsx-validator

`xlsx-validator` is an open-source command-line interface (CLI) tool written in Go that validates Excel workbook (`.xlsx`) contents against JSON-based rule configurations without modifying the original file.

Once installed, the tool runs entirely locally and offline, without requiring Microsoft Excel, LibreOffice, a Node.js runtime, or a network connection during validation.

---

## Table of Contents

- [Installation and Build](#installation-and-build)
- [CLI Usage](#cli-usage)
- [Exit Codes](#exit-codes)
- [JSON Configuration Format](#json-configuration-format)
  - [Default Values](#default-values)
  - [Column Data Types](#column-data-types)
  - [Supported Rules](#supported-rules)
- [Date Layout Format](#date-layout-format)
- [Output Formats](#output-formats)
  - [Text (Default)](#1-text-format-default)
  - [JSON](#2-json-format)
  - [CSV](#3-csv-format)
- [Stable Error Codes](#stable-error-codes)
- [Shell Script Usage Example](#shell-script-usage-example)
- [MVP Scope and Future Plans](#mvp-scope-and-future-plans)

---

## Installation and Build

### Prerequisites

- Go version 1.23 or later.

### Build Steps

Clone the repository and build the local binary using:

```sh
go build ./cmd/xlsx-validator
```

This command creates an executable named `xlsx-validator` (or `xlsx-validator.exe` on Windows) in the current directory.

---

## CLI Usage

Place flag options before the positional workbook path argument:

```sh
# Print a text report to the terminal (stdout)
./xlsx-validator --config examples/employee-rules.json employees.xlsx

# Print a JSON report to the terminal
./xlsx-validator --config examples/employee-rules.json --format json employees.xlsx

# Safely save a JSON report to a file (atomic write)
./xlsx-validator --config examples/employee-rules.json --format json --output result.json employees.xlsx

# Save a CSV report to a file
./xlsx-validator --config examples/employee-rules.json --format csv --output result.csv employees.xlsx

# Show command help
./xlsx-validator --help
```

### Flag Options

| Flag | Description | Default | Required? |
|------|-------------|---------|-----------|
| `--config <path>` | Path to the JSON rule configuration file | - | Yes |
| `--format <type>` | Output report format: `text`, `json`, or `csv` | `text` | No |
| `--output <path>` | Destination file path for the report (atomic write) | stdout | No |
| `--help`, `-h` | Show help and exit with code 0 | - | No |

> **Output Write Safety:** If `--output` points to the input workbook or the configuration file being read, including through a symbolic link or hard link, execution is rejected to prevent damage to the original data.

---

## Exit Codes

| Code | Description |
|------|-------------|
| `0` | Validation completed with **no violations found** (success). |
| `1` | Validation completed with **one or more violations**, including sheet or header structure violations. |
| `2` | An operational error occurred: invalid arguments, invalid configuration, an unreadable or non-`.xlsx` workbook, or an output write failure. |

---

## JSON Configuration Format

Example configuration file, `examples/employee-rules.json`:

```json
{
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
    "EMPLOYEE_NAME": {
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
    "STATUS": {
      "type": "string",
      "allowedValues": ["PERMANENT", "CONTRACT", "PROBATION"]
    }
  }
}
```

### Default Values

- `headerRow`: Defaults to `1` (the first Excel row; uses one-based indexing and must be greater than 0).
- `skipEmptyRows`: Defaults to `true`. An explicit `false` is respected. A row is skipped only if every cell in that row is empty, including cells in unconfigured columns. Formula cells and Excel error cells cause the row to be considered nonempty.
- `trimSpace`: Defaults to `true`. An explicit `false` is respected. Whitespace trimming affects rule matching; original values are preserved in the report.
- `type`: Defaults to `"string"` when omitted.

### Column Data Types

- `string`: General text. Length is measured in Unicode code points, rather than bytes.
- `integer`: A mathematical integer. Raw values are checked precisely without inaccurate floating-point conversion.
- `number`: A finite real number. NaN and infinity are rejected.
- `date`: A calendar date. Supports text formats and Excel serial numbers.
- `boolean`: A native Excel boolean value or the strings `"true"` / `"false"` (case-insensitive). Values such as `0`, `1`, `Y`, and `N` are not treated as booleans.

### Supported Rules

| Rule | Applicable Types | Description |
|------|------------------|-------------|
| `required` | All | If `true`, the cell must not be empty. An empty required cell produces a `REQUIRED` violation. Empty optional cells skip further validation. |
| `unique` | All | Checks the uniqueness of normalized text values using case-sensitive comparison. Every member of a duplicate group is flagged with `DUPLICATE`. Empty cells are excluded from uniqueness checks. |
| `allowedValues` | All | A list of permitted text values (case-sensitive). Produces `NOT_ALLOWED` if no match is found. |
| `minLength` | `string` | Minimum character count (Unicode code points, >= 0). Produces `MIN_LENGTH`. |
| `maxLength` | `string` | Maximum character count (Unicode code points, >= 0). Produces `MAX_LENGTH`. |
| `pattern` | `string` | A Go regular expression (`regexp`). Produces `PATTERN_MISMATCH`. Use `^` and `$` to match the entire value. |
| `format` | `string` | A special format. The MVP supports `"email"` (a plain email address without a display name). Produces `INVALID_FORMAT`. |
| `min` | `integer`, `number` | A finite minimum numeric value. Produces `MIN_VALUE`. Supports `min: 0`. |
| `max` | `integer`, `number` | A finite maximum numeric value. Produces `MAX_VALUE`. |
| `dateLayouts` | `date` | A list of Go parser layouts (`time.Parse`). At least one layout is required for text date input. |
| `allowExcelSerial` | `date` | If `true`, accepts Excel date serial numbers using the 1900 or 1904 date system. The fictitious leap day 1900-02-29 is explicitly rejected. |

> **Configuration Validation:** The configuration rejects unknown fields, duplicate JSON keys, empty column names, invalid regular expressions, inconsistent bounds (`min > max`, `minLength > maxLength`, or negative lengths), and rule combinations incompatible with the specified type.

---

## Date Layout Format

Date layouts in `dateLayouts` follow Go's time parser conventions (`time.Parse`), rather than tokens such as `YYYY-MM-DD`. Go uses `2006-01-02 15:04:05` as its reference date and time:

- `"2006-01-02"` for Year-Month-Day (example: `2023-01-15`)
- `"02/01/2006"` for Day/Month/Year (example: `15/01/2023`)
- `"01/02/2006"` for Month/Day/Year (example: `01/15/2023`)

---

## Output Formats

### 1. Text Format (Default)

Human-readable terminal output:

```text
File:  testdata/invalid_employees.xlsx
Sheet: Employees
Summary:
  Checked rows:           3
  Valid rows:             0
  Invalid rows:           3
  Skipped empty rows:     1
  Structure errors:       0
  Total errors:           8

Validation Status: FAILED (8 violations)
Violations:
    1. A2         DUPLICATE            duplicate value "1001" found in rows 2, 3 [EMP_NO] (value: "1001")
    2. A3         DUPLICATE            duplicate value "1001" found in rows 2, 3 [EMP_NO] (value: "1001")
    3. B3         REQUIRED             field "FULL_NAME" is required [FULL_NAME]
    4. C3         INVALID_FORMAT       value "budi@@invalid..com" is not a valid email address [EMAIL] (value: "budi@@invalid..com")
    5. D3         INVALID_TYPE         value "32/01/2023" is not a valid date [JOIN_DATE] (value: "32/01/2023")
    6. E3         MIN_VALUE            value -500000 is less than minimum 0 [SALARY] (value: "-500000")
    7. F3         NOT_ALLOWED          value "INTERN" is not in allowed values: PERMANENT, CONTRACT, PROBATION [EMP_STATUS] (value: "INTERN")
    8. A5         PATTERN_MISMATCH     value "EMP_ABC" does not match pattern "^[0-9]+$" [EMP_NO] (value: "EMP_ABC")
```

### 2. JSON Format

Standardized JSON schema structure:

```json
{
  "file": "testdata/invalid_employees.xlsx",
  "sheet": "Employees",
  "summary": {
    "checkedRows": 3,
    "validRows": 0,
    "invalidRows": 3,
    "skippedRows": 1,
    "errorCount": 8,
    "structureErrorCount": 0
  },
  "errors": [
    {
      "sheet": "Employees",
      "row": 2,
      "cell": "A2",
      "column": "EMP_NO",
      "value": "1001",
      "code": "DUPLICATE",
      "message": "duplicate value \"1001\" found in rows 2, 3"
    }
  ]
}
```

When there are no violations, `errors` is always an empty array, `[]`.

### 3. CSV Format

The CSV file contains a header and one record per violation:

```csv
sheet,row,cell,column,value,code,message
Employees,2,A2,EMP_NO,1001,DUPLICATE,"duplicate value ""1001"" found in rows 2, 3"
Employees,3,A3,EMP_NO,1001,DUPLICATE,"duplicate value ""1001"" found in rows 2, 3"
Employees,3,B3,FULL_NAME,,REQUIRED,"field ""FULL_NAME"" is required"
Employees,3,C3,EMAIL,budi@@invalid..com,INVALID_FORMAT,"value ""budi@@invalid..com"" is not a valid email address"
Employees,3,D3,JOIN_DATE,32/01/2023,INVALID_TYPE,"value ""32/01/2023"" is not a valid date"
Employees,3,E3,SALARY,'-500000,MIN_VALUE,value -500000 is less than minimum 0
Employees,3,F3,EMP_STATUS,INTERN,NOT_ALLOWED,"value ""INTERN"" is not in allowed values: PERMANENT, CONTRACT, PROBATION"
Employees,5,A5,EMP_NO,EMP_ABC,PATTERN_MISMATCH,"value ""EMP_ABC"" does not match pattern ""^[0-9]+$"""
```

- If validation succeeds without errors, the CSV contains only the header row and no data rows.
- **CSV Formula Injection Protection:** Text values beginning with `=`, `+`, `-`, `@`, a tab (`\t`), or a carriage return (`\r`) are automatically neutralized by prefixing them with a single quote (`'`) in the CSV file. Values in JSON remain unchanged.

---

## Stable Error Codes

| Code | Category | Description |
|------|----------|-------------|
| `SHEET_NOT_FOUND` | Structure | The configured sheet was not found in the workbook. |
| `MISSING_HEADER` | Structure | A configured column header was not found in `headerRow`. |
| `DUPLICATE_HEADER` | Structure | Two or more nonempty headers have the same name. |
| `FORMULA_HEADER` | Structure | A formula was found in the header row. |
| `REQUIRED` | Data | A required cell is empty or contains only whitespace. |
| `DUPLICATE` | Data | A duplicate value was detected in a column configured with `unique: true`. |
| `INVALID_TYPE` | Data | The cell value does not match the specified type, such as a noninteger, a nonboolean, or an invalid date. |
| `INVALID_FORMAT` | Data | The string does not match the required special format, such as a valid email address. |
| `MIN_LENGTH` | Data | The string length is less than `minLength`. |
| `MAX_LENGTH` | Data | The string length exceeds `maxLength`. |
| `MIN_VALUE` | Data | The numeric value is less than `min`. |
| `MAX_VALUE` | Data | The numeric value exceeds `max`. |
| `NOT_ALLOWED` | Data | The value is not listed in `allowedValues`. |
| `PATTERN_MISMATCH` | Data | The string does not match the `pattern` regular expression. |
| `FORMULA_NOT_ALLOWED` | Data | A formula was found in a configured data cell. |
| `EXCEL_ERROR` | Data | The cell contains a built-in Excel error value, such as `#VALUE!`, `#N/A`, or `#REF!`. |

---

## Shell Script Usage Example

You can integrate `xlsx-validator` into an automated pipeline or Bash script using its exit codes:

```sh
#!/bin/bash
set -e

./xlsx-validator --config examples/employee-rules.json --format json --output result.json employees.xlsx
EXIT_CODE=$?

if [ $EXIT_CODE -eq 0 ]; then
    echo "Validation succeeded: No errors found."
    # Continue with the database import
elif [ $EXIT_CODE -eq 1 ]; then
    echo "Validation failed: Invalid data found. Please check result.json."
    exit 1
else
    echo "System/configuration error: Exit code $EXIT_CODE"
    exit 2
fi
```

---

## MVP Scope and Future Plans

### Features Implemented in the MVP

- Validation of one sheet per CLI invocation.
- Comprehensive header structure checks: sheet existence, missing headers, duplicate headers, and formulas in headers.
- Data type checks: `string`, `integer`, `number`, `date`, and `boolean`.
- Validation of formula cells and Excel error cells in data rows.
- Handling of numeric precision and leading zeros in strings, such as the code `00123`.
- Three report formats: terminal text, JSON, and CSV (with formula injection protection).
- Atomic report file writes using a safe temporary-file rename.
- Duplicate JSON key detection in configuration files.
- Deterministic report ordering: Excel row, column position, then rule code.

### Future Development Ideas (Outside the MVP)

The following features have been intentionally deferred and may be implemented in future versions:

- Multi-sheet configuration in a single file and execution.
- Validation lookups against external databases.
- Cross-column conditional validation rules.
- Dynamic formula evaluation through a formula calculation engine.
- Automatic data repair or file sanitization.
- A dynamic plugin system.
- A graphical user interface (GUI) or web UI.
