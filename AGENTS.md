# XLSX Validator — Agent Instructions

## Project goal

Build an open-source, native Go CLI that reads an `.xlsx` workbook, validates
its contents against a JSON configuration, and reports the results. The tool
must work locally and offline after installation. It must not modify the input
workbook.

The initial use case is employee import validation, but the engine must be
generic. Employee-specific column names and rules belong in example configs,
not in the validation engine.

## Agreed scope and technology

- Language: Go.
- Workbook library: `github.com/xuri/excelize/v2`.
- Input format: `.xlsx` only. Users convert `.xls` externally before validation.
- Configuration: JSON, parsed with the Go standard library.
- Interface: CLI. No web UI, HTTP server, database, authentication, or Node.js.
- Outputs: human-readable terminal report, JSON, and CSV.
- Prefer the standard library for flags, JSON, CSV, regular expressions, and
  basic validation. Add dependencies only when they solve a concrete need.
- Select an explicit stable Excelize version compatible with the project's Go
  version. Commit `go.mod` and `go.sum`. Do not assume a version is the latest.
- Produce a native executable per target OS/architecture, without requiring
  Microsoft Excel or LibreOffice at runtime.

## How to work in this repository

1. Inspect existing source, README, `go.mod`, and tests before changing code.
2. Preserve established repository conventions when they already exist.
3. If the repository is empty, implement a small working vertical slice first:
   config loading, XLSX reading, required-field validation, and terminal output.
   Then complete the remaining MVP features below.
4. Make routine implementation decisions autonomously. Ask only when a missing
   requirement materially changes user-visible behavior or architecture.
5. Keep code readable and idiomatic. Avoid unnecessary frameworks, generic
   plugin systems, interfaces with only one implementation, and concurrency.
6. Do not fetch, upload, or log real employee data. Use synthetic test fixtures.
7. Do not publish releases, push commits, or choose a project license unless
   the user requests it. Do not copy implementation code from proprietary work.
8. Report what changed, what checks ran, and any remaining limitations. Never
   claim tests or builds passed if they could not be executed.

## CLI contract

Use this command shape, with flags before the positional workbook path so it
also works with Go's standard `flag` package:

```sh
xlsx-validator --config examples/employee-rules.json employees.xlsx
xlsx-validator --config examples/employee-rules.json --format json employees.xlsx
xlsx-validator --config examples/employee-rules.json --format json --output result.json employees.xlsx
xlsx-validator --config examples/employee-rules.json --format csv --output result.csv employees.xlsx
```

- `--config` is required.
- Exactly one positional workbook path is required.
- `--format` accepts `text`, `json`, or `csv`; default: `text`.
- `--output` is optional. Without it, write the selected report to stdout.
- Keep stdout clean for the selected report. Send operational errors to stderr.
- Refuse an output path that resolves to the input workbook or configuration,
  including symlinks and hard links. Do not truncate them.
- Write file reports through a temporary file in the destination directory and
  rename only after successful encoding and close. Never leave a partial report.
- `--help` exits successfully and documents usage and exit codes.
- Exit `0`: validation completed with no violations.
- Exit `1`: validation completed with one or more violations, including workbook
  structure violations.
- Exit `2`: invalid arguments/config, unreadable or unsupported input, workbook
  parsing failure, or report-writing failure.
- Inspect the extension case-insensitively and let Excelize verify contents.
  Return a clear error for unsupported or invalid workbooks.

## Initial configuration contract

Implement one configured sheet per invocation for the MVP. Use this schema:

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
}
```

Configuration behavior:

- `sheet` and a nonempty `columns` object are required.
- Defaults when omitted: `headerRow=1`, `skipEmptyRows=true`, `trimSpace=true`.
  Respect explicit `false`; use pointer fields or another explicit defaulting
  strategy where necessary.
- `headerRow` uses Excel's 1-based row numbers and must be positive.
- Every configured column header must exist, even if `required=false`.
  `required` controls whether data cells may be empty. Ignore unconfigured
  columns for value validation.
- Column types: `string`, `integer`, `number`, `date`, and `boolean`.
  The default type is `string`.
- Reject unknown JSON fields, unknown types/formats, trailing JSON values,
  empty column names, invalid regex patterns, and incompatible rule/type
  combinations. Reject duplicate JSON object keys instead of silently using
  the last value, including duplicate column definitions.
- Validate bounds: nonnegative lengths, minimum <= maximum, finite numeric
  bounds, and at least one date layout when validating text dates.
- Length/pattern/email rules apply to strings; numeric bounds apply to integer
  and number columns; date options apply to date columns.
- Use pointer numeric bounds so `min=0` is distinguishable from an unset bound.
- Report configuration mistakes before processing workbook data.

## Validation semantics

### Structure, rows, and coordinates

- Match headers case-sensitively after applying the configured trim behavior.
- Detect missing configured headers and duplicate nonempty headers. Return
  structure violations before attempting data validation when mapping is
  ambiguous or incomplete.
- A missing sheet is a structure violation, not a successful empty result.
- Keep original Excel row numbers and cell addresses, including gaps and blank
  rows. Do not renumber rows after filtering.
- With `skipEmptyRows=true`, skip a row only when every cell in that row is
  empty/whitespace under the configured trim behavior, including unconfigured
  columns. A formula or Excel error cell makes a row nonempty.
- Validate hidden rows and sheets normally. Do not expand merged cells or fill
  blank cells automatically; merged data cells are outside the MVP contract.

### Values and rules

- Preserve both raw and formatted cell values when needed. Use formatted text
  for string rules so an Excel number format such as `00000` can preserve an
  employee code displayed as `00123`. Never silently strip leading zeroes from
  string identifiers. Already-lost Excel precision cannot be recovered.
- Numeric rules use raw values without currency symbols or thousands-separator
  guessing. Require finite values; reject NaN and infinity. Integers must be
  mathematically integral and validated without lossy float conversion.
- Do not mutate workbook values. `trimSpace` affects validation comparisons;
  the report retains the original value.
- Empty optional cells skip all other value rules. Empty required cells emit
  one `REQUIRED` violation and skip the other rules.
- Count string lengths in Unicode code points, not bytes.
- `allowedValues` and `unique` compare normalized textual values
  case-sensitively. No implicit case folding or numeric equivalence.
- Empty cells do not participate in uniqueness. Flag every member of a
  duplicate group, including the first occurrence; include related row numbers
  where practical.
- Compile regex rules once when loading config. Patterns use Go regexp
  semantics; full-string matching requires explicit `^` and `$` anchors.
- Email validation accepts a bare email address only, without a display name.
  It checks syntax locally and does not perform DNS or delivery checks.
- Boolean values accept native Excel booleans and text `true`/`false`
  case-insensitively. Do not treat `0`, `1`, `Y`, or `N` as booleans by default.
- Text dates must match one of the exact Go `time.Parse` layouts in
  `dateLayouts`. Use Go layouts, not `YYYY-MM-DD` tokens. Do not guess locales
  or accept impossible dates.
- Numeric Excel dates are accepted only with `allowExcelSerial=true` in a date
  column. Use Excelize's date conversion and the workbook's 1900/1904 date
  system. Explicitly reject the fictitious 1900-02-29 serial day rather than
  silently converting it to a different date.
- Reject formulas in configured data cells for the MVP with
  `FORMULA_NOT_ALLOWED`. Do not calculate them or trust cached results. Formula
  headers are structure violations. Detect formulas before checking emptiness.
- Report Excel error cells with `EXCEL_ERROR`, not as normal text.
- A type failure emits one `INVALID_TYPE` violation and skips dependent rules.
  Independent valid rules may produce multiple violations for the same cell.

## Reporting contract

Keep the report deterministic: sort by Excel row, column position, then rule
code. Do not rely on Go map iteration order.

JSON contains:

- `file`, `sheet`.
- `summary`: `checkedRows`, `validRows`, `invalidRows`, `skippedRows`,
  `errorCount`, and `structureErrorCount`.
- `errors`: always an array, including `[]` on success.
- Each violation: `sheet`, `row`, `cell`, `column`, `value`, `code`, `message`.
  Structure violations without a cell use row `0` and empty cell/column fields.

`invalidRows` counts distinct invalid data rows, not violations.
`validRows + invalidRows == checkedRows`. `errorCount` counts all violations,
including structure violations. When structure validation prevents reading
data, row counters remain zero and `structureErrorCount` explains the failure.

CSV contains a header and one record per violation using the same fields. A
successful report contains its CSV header with no violation records. Document
that CSV has no summary section. Properly escape CSV values and neutralize
spreadsheet formula injection in exported text beginning with `=`, `+`, `-`,
`@`, tab, or carriage return. Keep JSON values unchanged.

Use stable codes such as `SHEET_NOT_FOUND`, `MISSING_HEADER`,
`DUPLICATE_HEADER`, `FORMULA_HEADER`, `REQUIRED`, `DUPLICATE`, `INVALID_TYPE`,
`INVALID_FORMAT`, `MIN_LENGTH`, `MAX_LENGTH`, `MIN_VALUE`, `MAX_VALUE`,
`NOT_ALLOWED`, `PATTERN_MISMATCH`, `FORMULA_NOT_ALLOWED`, and `EXCEL_ERROR`.

## Suggested layout

Keep the layout proportional to the project; this is a starting point:

```text
cmd/xlsx-validator/main.go
internal/config/
internal/validator/
internal/report/
examples/employee-rules.json
testdata/
README.md
go.mod
go.sum
```

The CLI handles arguments and exit codes. Configuration loading, validation,
and report encoding should be callable from tests without spawning a process.
Do not create a package for every rule or abstract the XLSX reader for a
hypothetical future XLS implementation.

## Testing and verification

- Use Go's `testing` package. Generate small XLSX fixtures with Excelize in
  temporary directories where practical and check fixture creation errors.
- Cover valid data, missing/duplicate headers, missing sheets, required/optional
  cells, all duplicate occurrences, row gaps, blank rows, Unicode lengths,
  invalid config, numeric zero bounds, leading zeroes, native/text dates,
  1900/1904 dates, formulas, Excel errors, and deterministic reporting.
- Check report escaping, output path collisions, and CLI exit codes.
- Include at least one end-to-end valid workbook and one invalid workbook with
  documented expected counts.
- Run `gofmt`, `go mod tidy`, `go test ./...`, `go vet ./...`, and
  `go build ./cmd/xlsx-validator` before declaring the MVP complete.
- Add a minimal CI workflow for the same build and tests if the repository
  uses GitHub. Do not add deployment infrastructure.

## Documentation and completion criteria

Write user-facing README instructions in Indonesian; use English Go identifiers
and stable rule codes. Document installation/building, complete CLI examples,
config defaults, each implemented rule, date layouts, output schemas, exit
codes, and limitations. Ensure documented config fields are actually supported.

The MVP is complete when a user can build the CLI, validate a synthetic XLSX
using only a JSON config, obtain reliable cell-level errors in all three output
formats, and use the exit code in a shell script. No network access should occur
while validating a file.

Defer multi-sheet configs, database lookups, cross-column conditions, formula
evaluation, automatic repair, plugins, and GUI work unless the user requests
them. Clearly distinguish implemented features from future ideas.
