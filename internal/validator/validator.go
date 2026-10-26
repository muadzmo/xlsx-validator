package validator

import (
	"fmt"
	"math/big"
	"net/mail"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/xuri/excelize/v2"

	"xlsx-validator/internal/config"
	"xlsx-validator/internal/report"
)

var emailRegex = regexp.MustCompile("^[a-zA-Z0-9.!#$%&'*+/=?^_`{|}~-]+@[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?(?:\\.[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?)+$")

type occurrence struct {
	row    int
	cell   string
	colIdx int
	val    string
}

// ValidateFile opens and validates the specified workbook against the configuration.
func ValidateFile(workbookPath string, cfg *config.Config) (*report.Report, error) {
	// Case-insensitive .xlsx check
	ext := strings.ToLower(filepath.Ext(workbookPath))
	if ext != ".xlsx" {
		return nil, fmt.Errorf("unsupported workbook format %q: only .xlsx files are supported", ext)
	}

	f, err := excelize.OpenFile(workbookPath)
	if err != nil {
		return nil, fmt.Errorf("opening workbook %s: %w", workbookPath, err)
	}
	defer f.Close()

	return ValidateWorkbook(f, workbookPath, cfg)
}

// ValidateWorkbook validates an opened excelize.File.
func ValidateWorkbook(f *excelize.File, filePath string, cfg *config.Config) (*report.Report, error) {
	rep := report.NewReport(filePath, cfg.Sheet)

	// 1. Check if sheet exists
	sheetIdx, err := f.GetSheetIndex(cfg.Sheet)
	if err != nil || sheetIdx == -1 {
		v := report.Violation{
			Sheet:   cfg.Sheet,
			Row:     0,
			Cell:    "",
			Column:  "",
			Value:   "",
			Code:    report.CodeSheetNotFound,
			Message: fmt.Sprintf("sheet %q not found in workbook", cfg.Sheet),
			ColIdx:  0,
		}
		rep.Errors = append(rep.Errors, v)
		rep.Summary.StructureErrorCount = 1
		rep.Summary.ErrorCount = 1
		rep.Sort()
		return rep, nil
	}

	// 2. Check 1904 vs 1900 date system
	use1904 := false
	if props, err := f.GetWorkbookProps(); err == nil && props.Date1904 != nil {
		use1904 = *props.Date1904
	}

	// 3. Read sheet rows
	rows, err := f.GetRows(cfg.Sheet)
	if err != nil {
		return nil, fmt.Errorf("reading sheet rows: %w", err)
	}

	headerRowNum := *cfg.HeaderRow
	var structureViolations []report.Violation

	// If headerRow is beyond available rows in the sheet
	if headerRowNum > len(rows) {
		// All configured headers are missing
		for colName := range cfg.Columns {
			structureViolations = append(structureViolations, report.Violation{
				Sheet:   cfg.Sheet,
				Row:     0,
				Cell:    "",
				Column:  "",
				Value:   "",
				Code:    report.CodeMissingHeader,
				Message: fmt.Sprintf("missing required header %q (headerRow %d is beyond sheet content)", colName, headerRowNum),
				ColIdx:  0,
			})
		}
		rep.Errors = structureViolations
		rep.Summary.StructureErrorCount = len(structureViolations)
		rep.Summary.ErrorCount = len(structureViolations)
		rep.Sort()
		return rep, nil
	}

	// 4. Inspect header row
	headerCells := rows[headerRowNum-1]
	seenHeaders := make(map[string]int) // trimmed header -> 1-based colIdx

	for c := 1; c <= len(headerCells); c++ {
		cellName, _ := excelize.CoordinatesToCellName(c, headerRowNum)

		// Check for formula in header
		formula, _ := f.GetCellFormula(cfg.Sheet, cellName)
		if formula != "" {
			structureViolations = append(structureViolations, report.Violation{
				Sheet:   cfg.Sheet,
				Row:     headerRowNum,
				Cell:    cellName,
				Column:  headerCells[c-1],
				Value:   "=" + formula,
				Code:    report.CodeFormulaHeader,
				Message: fmt.Sprintf("header cell %s contains formula %q", cellName, formula),
				ColIdx:  c,
			})
		}

		rawHeader := headerCells[c-1]
		trimmedHeader := rawHeader
		if *cfg.TrimSpace {
			trimmedHeader = strings.TrimSpace(rawHeader)
		}

		if trimmedHeader != "" {
			if prevCol, exists := seenHeaders[trimmedHeader]; exists {
				prevCell, _ := excelize.CoordinatesToCellName(prevCol, headerRowNum)
				structureViolations = append(structureViolations, report.Violation{
					Sheet:   cfg.Sheet,
					Row:     headerRowNum,
					Cell:    cellName,
					Column:  trimmedHeader,
					Value:   rawHeader,
					Code:    report.CodeDuplicateHeader,
					Message: fmt.Sprintf("duplicate header %q found at %s (previously seen at %s)", trimmedHeader, cellName, prevCell),
					ColIdx:  c,
				})
			} else {
				seenHeaders[trimmedHeader] = c
			}
		}
	}

	// Check that every configured column header exists
	// To be deterministic, sort configured column names
	colNames := make([]string, 0, len(cfg.Columns))
	for name := range cfg.Columns {
		colNames = append(colNames, name)
	}
	sort.Strings(colNames)

	for _, colName := range colNames {
		if _, found := seenHeaders[colName]; !found {
			structureViolations = append(structureViolations, report.Violation{
				Sheet:   cfg.Sheet,
				Row:     0,
				Cell:    "",
				Column:  "",
				Value:   "",
				Code:    report.CodeMissingHeader,
				Message: fmt.Sprintf("missing required header %q", colName),
				ColIdx:  0,
			})
		}
	}

	// If there are structure violations, return before attempting data validation
	if len(structureViolations) > 0 {
		rep.Errors = structureViolations
		rep.Summary.StructureErrorCount = len(structureViolations)
		rep.Summary.ErrorCount = len(structureViolations)
		rep.Sort()
		return rep, nil
	}

	// Mapping from colName to column index
	colMap := make(map[string]int, len(cfg.Columns))
	for _, colName := range colNames {
		colMap[colName] = seenHeaders[colName]
	}

	// 5. Data rows validation
	checkedRows := 0
	skippedRows := 0
	invalidRowSet := make(map[int]bool)

	// Track uniqueness: colName -> normalized text value -> []occurrence
	uniqueTracker := make(map[string]map[string][]occurrence)
	for colName, colCfg := range cfg.Columns {
		if colCfg.Unique {
			uniqueTracker[colName] = make(map[string][]occurrence)
		}
	}

	for r := headerRowNum + 1; r <= len(rows); r++ {
		var rowCells []string
		if r-1 < len(rows) {
			rowCells = rows[r-1]
		}

		// Emptiness check for the entire row (including unconfigured columns)
		isEmptyRow := true
		for c := 1; c <= len(rowCells); c++ {
			cellName, _ := excelize.CoordinatesToCellName(c, r)
			formula, _ := f.GetCellFormula(cfg.Sheet, cellName)
			if formula != "" {
				isEmptyRow = false
				break
			}
			cellType, _ := f.GetCellType(cfg.Sheet, cellName)
			if cellType == excelize.CellTypeError {
				isEmptyRow = false
				break
			}
			cellVal := rowCells[c-1]
			if isExcelErrorString(cellVal) {
				isEmptyRow = false
				break
			}
			if *cfg.TrimSpace {
				if strings.TrimSpace(cellVal) != "" {
					isEmptyRow = false
					break
				}
			} else {
				if cellVal != "" {
					isEmptyRow = false
					break
				}
			}
		}

		if isEmptyRow {
			if *cfg.SkipEmptyRows {
				skippedRows++
				continue
			}
		}

		checkedRows++

		// Validate each configured column
		for _, colName := range colNames {
			colCfg := cfg.Columns[colName]
			colIdx := colMap[colName]
			cellName, _ := excelize.CoordinatesToCellName(colIdx, r)

			// 1. Detect formula in configured data cell
			formula, _ := f.GetCellFormula(cfg.Sheet, cellName)
			if formula != "" {
				rawVal, _ := f.GetCellValue(cfg.Sheet, cellName, excelize.Options{RawCellValue: true})
				dispVal := rawVal
				if dispVal == "" {
					dispVal = "=" + formula
				}
				rep.Errors = append(rep.Errors, report.Violation{
					Sheet:   cfg.Sheet,
					Row:     r,
					Cell:    cellName,
					Column:  colName,
					Value:   dispVal,
					Code:    report.CodeFormulaNotAllowed,
					Message: fmt.Sprintf("formula %q is not allowed", formula),
					ColIdx:  colIdx,
				})
				invalidRowSet[r] = true
				continue
			}

			// 2. Detect Excel error in cell
			cellType, _ := f.GetCellType(cfg.Sheet, cellName)
			formattedVal, _ := f.GetCellValue(cfg.Sheet, cellName)
			rawVal, _ := f.GetCellValue(cfg.Sheet, cellName, excelize.Options{RawCellValue: true})

			if cellType == excelize.CellTypeError || isExcelErrorString(formattedVal) || isExcelErrorString(rawVal) {
				errVal := formattedVal
				if errVal == "" {
					errVal = rawVal
				}
				rep.Errors = append(rep.Errors, report.Violation{
					Sheet:   cfg.Sheet,
					Row:     r,
					Cell:    cellName,
					Column:  colName,
					Value:   errVal,
					Code:    report.CodeExcelError,
					Message: fmt.Sprintf("cell contains Excel error %s", errVal),
					ColIdx:  colIdx,
				})
				invalidRowSet[r] = true
				continue
			}

			// Value selection:
			// For string rules, use formattedVal so number format (e.g. 00000) preserves displayed text (e.g. 00123)
			// For numeric rules, use rawVal without currency symbols or thousands-separator guessing
			originalVal := formattedVal
			if (colCfg.Type == config.TypeInteger || colCfg.Type == config.TypeNumber) && rawVal != "" {
				originalVal = rawVal
			}

			compVal := originalVal
			if *cfg.TrimSpace {
				compVal = strings.TrimSpace(compVal)
			}

			// 3. Emptiness & Required
			if compVal == "" {
				if colCfg.Required {
					rep.Errors = append(rep.Errors, report.Violation{
						Sheet:   cfg.Sheet,
						Row:     r,
						Cell:    cellName,
						Column:  colName,
						Value:   originalVal,
						Code:    report.CodeRequired,
						Message: fmt.Sprintf("field %q is required", colName),
						ColIdx:  colIdx,
					})
					invalidRowSet[r] = true
				}
				// Empty cells skip all other value rules
				continue
			}

			// 4. Type Validation & Dependent Rules
			typeValid := true

			switch colCfg.Type {
			case config.TypeString:
				runeCount := utf8.RuneCountInString(compVal)
				if colCfg.MinLength != nil && runeCount < *colCfg.MinLength {
					rep.Errors = append(rep.Errors, report.Violation{
						Sheet:   cfg.Sheet,
						Row:     r,
						Cell:    cellName,
						Column:  colName,
						Value:   originalVal,
						Code:    report.CodeMinLength,
						Message: fmt.Sprintf("length %d is less than minLength %d", runeCount, *colCfg.MinLength),
						ColIdx:  colIdx,
					})
					invalidRowSet[r] = true
				}
				if colCfg.MaxLength != nil && runeCount > *colCfg.MaxLength {
					rep.Errors = append(rep.Errors, report.Violation{
						Sheet:   cfg.Sheet,
						Row:     r,
						Cell:    cellName,
						Column:  colName,
						Value:   originalVal,
						Code:    report.CodeMaxLength,
						Message: fmt.Sprintf("length %d exceeds maxLength %d", runeCount, *colCfg.MaxLength),
						ColIdx:  colIdx,
					})
					invalidRowSet[r] = true
				}
				if colCfg.CompiledPattern != nil && !colCfg.CompiledPattern.MatchString(compVal) {
					rep.Errors = append(rep.Errors, report.Violation{
						Sheet:   cfg.Sheet,
						Row:     r,
						Cell:    cellName,
						Column:  colName,
						Value:   originalVal,
						Code:    report.CodePatternMismatch,
						Message: fmt.Sprintf("value %q does not match pattern %q", compVal, colCfg.Pattern),
						ColIdx:  colIdx,
					})
					invalidRowSet[r] = true
				}
				if colCfg.Format == config.FormatEmail {
					if !isValidEmail(compVal) {
						rep.Errors = append(rep.Errors, report.Violation{
							Sheet:   cfg.Sheet,
							Row:     r,
							Cell:    cellName,
							Column:  colName,
							Value:   originalVal,
							Code:    report.CodeInvalidFormat,
							Message: fmt.Sprintf("value %q is not a valid email address", compVal),
							ColIdx:  colIdx,
						})
						invalidRowSet[r] = true
					}
				}

			case config.TypeInteger:
				rat := new(big.Rat)
				if _, ok := rat.SetString(compVal); !ok || !rat.IsInt() {
					typeValid = false
				} else {
					if colCfg.Min != nil {
						minRat := new(big.Rat).SetFloat64(*colCfg.Min)
						if rat.Cmp(minRat) < 0 {
							rep.Errors = append(rep.Errors, report.Violation{
								Sheet:   cfg.Sheet,
								Row:     r,
								Cell:    cellName,
								Column:  colName,
								Value:   originalVal,
								Code:    report.CodeMinValue,
								Message: fmt.Sprintf("value %s is less than minimum %v", compVal, *colCfg.Min),
								ColIdx:  colIdx,
							})
							invalidRowSet[r] = true
						}
					}
					if colCfg.Max != nil {
						maxRat := new(big.Rat).SetFloat64(*colCfg.Max)
						if rat.Cmp(maxRat) > 0 {
							rep.Errors = append(rep.Errors, report.Violation{
								Sheet:   cfg.Sheet,
								Row:     r,
								Cell:    cellName,
								Column:  colName,
								Value:   originalVal,
								Code:    report.CodeMaxValue,
								Message: fmt.Sprintf("value %s exceeds maximum %v", compVal, *colCfg.Max),
								ColIdx:  colIdx,
							})
							invalidRowSet[r] = true
						}
					}
				}
				if !typeValid {
					rep.Errors = append(rep.Errors, report.Violation{
						Sheet:   cfg.Sheet,
						Row:     r,
						Cell:    cellName,
						Column:  colName,
						Value:   originalVal,
						Code:    report.CodeInvalidType,
						Message: fmt.Sprintf("value %q is not a valid integer", originalVal),
						ColIdx:  colIdx,
					})
					invalidRowSet[r] = true
				}

			case config.TypeNumber:
				rat := new(big.Rat)
				if _, ok := rat.SetString(compVal); !ok {
					typeValid = false
				} else {
					if colCfg.Min != nil {
						minRat := new(big.Rat).SetFloat64(*colCfg.Min)
						if rat.Cmp(minRat) < 0 {
							rep.Errors = append(rep.Errors, report.Violation{
								Sheet:   cfg.Sheet,
								Row:     r,
								Cell:    cellName,
								Column:  colName,
								Value:   originalVal,
								Code:    report.CodeMinValue,
								Message: fmt.Sprintf("value %s is less than minimum %v", compVal, *colCfg.Min),
								ColIdx:  colIdx,
							})
							invalidRowSet[r] = true
						}
					}
					if colCfg.Max != nil {
						maxRat := new(big.Rat).SetFloat64(*colCfg.Max)
						if rat.Cmp(maxRat) > 0 {
							rep.Errors = append(rep.Errors, report.Violation{
								Sheet:   cfg.Sheet,
								Row:     r,
								Cell:    cellName,
								Column:  colName,
								Value:   originalVal,
								Code:    report.CodeMaxValue,
								Message: fmt.Sprintf("value %s exceeds maximum %v", compVal, *colCfg.Max),
								ColIdx:  colIdx,
							})
							invalidRowSet[r] = true
						}
					}
				}
				if !typeValid {
					rep.Errors = append(rep.Errors, report.Violation{
						Sheet:   cfg.Sheet,
						Row:     r,
						Cell:    cellName,
						Column:  colName,
						Value:   originalVal,
						Code:    report.CodeInvalidType,
						Message: fmt.Sprintf("value %q is not a valid number", originalVal),
						ColIdx:  colIdx,
					})
					invalidRowSet[r] = true
				}

			case config.TypeBoolean:
				if cellType == excelize.CellTypeBool {
					typeValid = true
				} else {
					lower := strings.ToLower(compVal)
					if lower == "true" || lower == "false" {
						typeValid = true
					} else {
						typeValid = false
					}
				}
				if !typeValid {
					rep.Errors = append(rep.Errors, report.Violation{
						Sheet:   cfg.Sheet,
						Row:     r,
						Cell:    cellName,
						Column:  colName,
						Value:   originalVal,
						Code:    report.CodeInvalidType,
						Message: fmt.Sprintf("value %q is not a valid boolean", originalVal),
						ColIdx:  colIdx,
					})
					invalidRowSet[r] = true
				}

			case config.TypeDate:
				isDateValid := false
				if colCfg.AllowExcelSerial {
					// Check if raw value can be parsed as a float
					serialFloat, err := strconv.ParseFloat(rawVal, 64)
					if err == nil {
						if !use1904 && serialFloat >= 60.0 && serialFloat < 61.0 {
							// Explicitly reject the fictitious 1900-02-29 leap day
							isDateValid = false
						} else if serialFloat < 0 || (!use1904 && serialFloat < 1.0) {
							isDateValid = false
						} else {
							if _, err := excelize.ExcelDateToTime(serialFloat, use1904); err == nil {
								isDateValid = true
							}
						}
					}
				}

				if !isDateValid && len(colCfg.DateLayouts) > 0 {
					textDate := compVal
					for _, layout := range colCfg.DateLayouts {
						parsedT, err := time.Parse(layout, textDate)
						if err == nil {
							// Strict verification
							if parsedT.Format(layout) == textDate {
								isDateValid = true
								break
							}
						}
					}
				}

				if !isDateValid {
					typeValid = false
					rep.Errors = append(rep.Errors, report.Violation{
						Sheet:   cfg.Sheet,
						Row:     r,
						Cell:    cellName,
						Column:  colName,
						Value:   originalVal,
						Code:    report.CodeInvalidType,
						Message: fmt.Sprintf("value %q is not a valid date", originalVal),
						ColIdx:  colIdx,
					})
					invalidRowSet[r] = true
				}
			}

			// If type check failed, skip dependent rules
			if !typeValid {
				continue
			}

			// AllowedValues check
			if len(colCfg.AllowedValues) > 0 {
				allowed := false
				for _, av := range colCfg.AllowedValues {
					if compVal == av {
						allowed = true
						break
					}
				}
				if !allowed {
					rep.Errors = append(rep.Errors, report.Violation{
						Sheet:   cfg.Sheet,
						Row:     r,
						Cell:    cellName,
						Column:  colName,
						Value:   originalVal,
						Code:    report.CodeNotAllowed,
						Message: fmt.Sprintf("value %q is not in allowed values: %s", compVal, strings.Join(colCfg.AllowedValues, ", ")),
						ColIdx:  colIdx,
					})
					invalidRowSet[r] = true
				}
			}

			// Uniqueness tracking
			if colCfg.Unique {
				uniqueTracker[colName][compVal] = append(uniqueTracker[colName][compVal], occurrence{
					row:    r,
					cell:   cellName,
					colIdx: colIdx,
					val:    originalVal,
				})
			}
		}
	}

	// 6. Evaluate uniqueness across all checked rows
	for _, colName := range colNames {
		colCfg := cfg.Columns[colName]
		if !colCfg.Unique {
			continue
		}
		tracker := uniqueTracker[colName]
		for compVal, occurrences := range tracker {
			if len(occurrences) > 1 {
				rowNums := make([]string, len(occurrences))
				for i, occ := range occurrences {
					rowNums[i] = strconv.Itoa(occ.row)
				}
				rowListStr := strings.Join(rowNums, ", ")

				for _, occ := range occurrences {
					invalidRowSet[occ.row] = true
					rep.Errors = append(rep.Errors, report.Violation{
						Sheet:   cfg.Sheet,
						Row:     occ.row,
						Cell:    occ.cell,
						Column:  colName,
						Value:   occ.val,
						Code:    report.CodeDuplicate,
						Message: fmt.Sprintf("duplicate value %q found in rows %s", compVal, rowListStr),
						ColIdx:  occ.colIdx,
					})
				}
			}
		}
	}

	// Summary counters
	invalidRows := len(invalidRowSet)
	validRows := checkedRows - invalidRows

	rep.Summary.CheckedRows = checkedRows
	rep.Summary.ValidRows = validRows
	rep.Summary.InvalidRows = invalidRows
	rep.Summary.SkippedRows = skippedRows
	rep.Summary.ErrorCount = len(rep.Errors)
	rep.Summary.StructureErrorCount = 0

	rep.Sort()
	return rep, nil
}

func isValidEmail(s string) bool {
	if !emailRegex.MatchString(s) {
		return false
	}
	addr, err := mail.ParseAddress(s)
	if err != nil {
		return false
	}
	return addr.Address == s && addr.Name == ""
}

func isExcelErrorString(s string) bool {
	switch strings.ToUpper(strings.TrimSpace(s)) {
	case "#NULL!", "#DIV/0!", "#VALUE!", "#REF!", "#NAME?", "#NUM!", "#N/A", "#GETTING_DATA":
		return true
	default:
		return false
	}
}
