package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"xlsx-validator/internal/config"
	"xlsx-validator/internal/report"
	"xlsx-validator/internal/validator"
)

const usageText = `xlsx-validator - generic .xlsx workbook validator

Usage:
  xlsx-validator --config <config.json> [flags] <workbook.xlsx>

Flags:
  --config string    Path to JSON validation configuration (required)
  --format string    Output format: text, json, csv (default: "text")
  --output string    Path to write output report (default: stdout)
  --help, -h         Show help and exit successfully

Exit Codes:
  0   Validation completed successfully with no violations
  1   Validation completed with one or more violations (including structure errors)
  2   Operational error (invalid arguments/config, unreadable workbook, write failure)
`

func main() {
	exitCode := run(os.Args[1:], os.Stdout, os.Stderr)
	os.Exit(exitCode)
}

func run(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("xlsx-validator", flag.ContinueOnError)
	fs.SetOutput(stderr)

	var (
		configPath string
		format     string
		outputPath string
	)

	fs.StringVar(&configPath, "config", "", "Path to JSON validation configuration (required)")
	fs.StringVar(&format, "format", "text", "Output format: text, json, or csv")
	fs.StringVar(&outputPath, "output", "", "Optional destination path for report")

	fs.Usage = func() {
		fmt.Fprint(stdout, usageText)
	}

	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		fmt.Fprintf(stderr, "Error: %v\n", err)
		return 2
	}

	// Validate config path
	if strings.TrimSpace(configPath) == "" {
		fmt.Fprintln(stderr, "Error: --config flag is required")
		return 2
	}

	// Validate positional workbook argument
	posArgs := fs.Args()
	if len(posArgs) != 1 {
		if len(posArgs) == 0 {
			fmt.Fprintln(stderr, "Error: missing workbook file argument")
		} else {
			fmt.Fprintf(stderr, "Error: expected exactly one workbook argument, got %d: %v\n", len(posArgs), posArgs)
		}
		return 2
	}
	workbookPath := posArgs[0]

	// Validate format
	normalizedFormat := strings.ToLower(strings.TrimSpace(format))
	switch normalizedFormat {
	case report.FormatText, report.FormatJSON, report.FormatCSV:
	default:
		fmt.Fprintf(stderr, "Error: invalid format %q (accepted formats: text, json, csv)\n", format)
		return 2
	}

	// Load configuration
	cfg, err := config.Load(configPath)
	if err != nil {
		fmt.Fprintf(stderr, "Error loading configuration: %v\n", err)
		return 2
	}

	// Perform validation
	rep, err := validator.ValidateFile(workbookPath, cfg)
	if err != nil {
		fmt.Fprintf(stderr, "Error validating workbook: %v\n", err)
		return 2
	}

	// Output report
	if outputPath == "" {
		if err := writeReport(stdout, normalizedFormat, rep); err != nil {
			fmt.Fprintf(stderr, "Error writing report to stdout: %v\n", err)
			return 2
		}
	} else {
		if err := report.WriteOutput(outputPath, normalizedFormat, rep, workbookPath, configPath); err != nil {
			fmt.Fprintf(stderr, "Error writing report: %v\n", err)
			return 2
		}
	}

	// Determine exit code based on violations
	if rep.Summary.ErrorCount > 0 {
		return 1
	}
	return 0
}

func writeReport(w io.Writer, format string, rep *report.Report) error {
	switch format {
	case report.FormatJSON:
		return rep.FormatJSON(w)
	case report.FormatCSV:
		return rep.FormatCSV(w)
	default:
		return rep.FormatText(w)
	}
}
