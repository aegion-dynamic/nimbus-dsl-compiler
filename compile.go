package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/charmbracelet/bubbles/table"
	"github.com/charmbracelet/lipgloss"
	"github.com/dosco/graphjin/core/v3"
)

type CompileError struct {
	Message string
	Line    int
	Column  int
}

type CompileResult struct {
	Query            string
	Variables        any
	VariablesMissing bool
	Errors           []CompileError
}

type FileIssueSummary struct {
	QueryFile        string              `json:"query_file"`
	QueryBase        string              `json:"query_base"`
	VariablesMissing bool                `json:"variables_missing,omitempty"`
	ExplainErrors    []string            `json:"explain_errors,omitempty"`
	MissingTables    []string            `json:"missing_tables,omitempty"`
	MissingColumns   map[string][]string `json:"missing_columns,omitempty"`
	ValidationError  string              `json:"validation_error,omitempty"`
}

func missingColumnsCount(m map[string][]string) int {
	total := 0
	for _, cols := range m {
		total += len(cols)
	}
	return total
}

func (f FileIssueSummary) HasIssues() bool {
	if f.VariablesMissing {
		return true
	}
	if len(f.ExplainErrors) > 0 {
		return true
	}
	if len(f.MissingTables) > 0 {
		return true
	}
	if missingColumnsCount(f.MissingColumns) > 0 {
		return true
	}
	if f.ValidationError != "" {
		return true
	}
	return false
}

type SummaryTotals struct {
	TotalQueries          int `json:"total_queries"`
	FilesWithAnyIssues    int `json:"files_with_any_issues"`
	ExplainErrorFiles     int `json:"explain_error_files"`
	MissingTablesFiles    int `json:"missing_tables_files"`
	MissingColumnsFiles   int `json:"missing_columns_files"`
	ValidationErrorFiles  int `json:"validation_error_files"`
	VariablesMissingFiles int `json:"variables_missing_files"`

	TotalExplainErrors  int `json:"total_explain_errors"`
	TotalMissingTables  int `json:"total_missing_tables"`
	TotalMissingColumns int `json:"total_missing_columns"`
}

type ValidationSummary struct {
	Totals SummaryTotals      `json:"totals"`
	Files  []FileIssueSummary `json:"files"`
}

func writeValidationSummaryJSON(jsonPath string, summary ValidationSummary) error {
	if strings.TrimSpace(jsonPath) == "" {
		return fmt.Errorf("jsonPath is empty")
	}

	outBytes, err := json.MarshalIndent(summary, "", "  ")
	if err != nil {
		return fmt.Errorf("marshalling validation summary json: %w", err)
	}

	dir := filepath.Dir(jsonPath)
	if dir != "." && dir != string(filepath.Separator) {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("creating json output directory %q: %w", dir, err)
		}
	}

	if err := os.WriteFile(jsonPath, outBytes, 0o644); err != nil {
		return fmt.Errorf("writing json output to %q: %w", jsonPath, err)
	}
	return nil
}

func renderValidationSummaryTUI(summary ValidationSummary) {
	issueFiles := make([]FileIssueSummary, 0, len(summary.Files))
	for _, f := range summary.Files {
		if f.HasIssues() {
			issueFiles = append(issueFiles, f)
		}
	}

	title := lipgloss.NewStyle().Bold(true).Underline(true).Render("nimbus-dsl-compile: validation summary")
	fmt.Println(title)

	totalsLine := fmt.Sprintf(
		"Total queries: %d | Files with issues: %d",
		summary.Totals.TotalQueries,
		summary.Totals.FilesWithAnyIssues,
	)
	fmt.Println(lipgloss.NewStyle().Bold(true).Render(totalsLine))

	if len(issueFiles) == 0 {
		fmt.Println("OK: all queries validated")
		return
	}

	rows := make([]table.Row, 0, len(issueFiles))
	for _, f := range issueFiles {
		explainCount := len(f.ExplainErrors)
		missingTablesCount := len(f.MissingTables)
		missingColsCount := missingColumnsCount(f.MissingColumns)

		validationErrCell := "-"
		if f.ValidationError != "" {
			validationErrCell = "yes"
		}

		varsMissingCell := "-"
		if f.VariablesMissing {
			varsMissingCell = "yes"
		}

		rows = append(rows, table.Row{
			f.QueryBase,
			fmt.Sprintf("%d", explainCount),
			fmt.Sprintf("%d", missingTablesCount),
			fmt.Sprintf("%d", missingColsCount),
			validationErrCell,
			varsMissingCell,
		})
	}

	columns := []table.Column{
		{Title: "File", Width: 28},
		{Title: "Explain", Width: 7},
		{Title: "MissingTables", Width: 14},
		{Title: "MissingColumns", Width: 15},
		{Title: "ValidationErr", Width: 14},
		{Title: "VarsMissing", Width: 12},
	}

	t := table.New(
		table.WithColumns(columns),
		table.WithRows(rows),
		table.WithHeight(len(rows)+2),
	)
	borderStyle := lipgloss.NewStyle().BorderStyle(lipgloss.RoundedBorder()).Padding(0, 1)
	fmt.Println(borderStyle.Render(t.View()))
}

func processQuery(queryFilePath, variablesFilePath string) (*CompileResult, error) {
	query, err := os.ReadFile(queryFilePath)
	if err != nil {
		return nil, err
	}

	rawQuery := string(query)
	preprocessedQuery := preprocessGraphQLQueryRemoveTypename(rawQuery)

	res := &CompileResult{
		Query:     preprocessedQuery,
		Errors:    []CompileError{},
		Variables: nil,
	}

	variablesBytes, err := os.ReadFile(variablesFilePath)
	if err != nil {
		if os.IsNotExist(err) {
			res.VariablesMissing = true
			return res, nil
		}
		return nil, err
	}

	var v any
	if err := json.Unmarshal(variablesBytes, &v); err != nil {
		return nil, err
	}
	res.Variables = v
	return res, nil
}

func Compile(config *Config, gj *core.GraphJin, verbose bool, jsonPath string) error {
	queriesDir := config.ConfigFileLocations.QueriesFolderPath
	if verbose {
		fmt.Printf("reading queries from %s\n", queriesDir)
	}

	queries, err := os.ReadDir(queriesDir)
	if err != nil {
		return err
	}

	byBase := make(map[string]*FileIssueSummary)
	foundQueries := 0
	for _, query := range queries {
		if query.IsDir() {
			continue
		}

		name := query.Name()
		ext := strings.ToLower(filepath.Ext(name))
		if ext != ".graphql" && ext != ".gql" {
			continue
		}
		foundQueries++

		base := strings.TrimSuffix(name, ext)
		queryFilePath := filepath.Join(queriesDir, query.Name())
		variablesFilePath := filepath.Join(queriesDir, base+".json")

		compileResult, err := processQuery(queryFilePath, variablesFilePath)
		if err != nil {
			return fmt.Errorf("processing query %q: %w", query.Name(), err)
		}

		issue := byBase[base]
		if issue == nil {
			issue = &FileIssueSummary{
				QueryFile: query.Name(),
				QueryBase: base,
			}
			byBase[base] = issue
		}

		if verbose {
			fmt.Printf("=== %s ===\n", base)
			fmt.Printf("--- query (%s) ---\n", queryFilePath)
			fmt.Println(compileResult.Query)

			if compileResult.VariablesMissing {
				fmt.Printf("--- data (%s) ---\n", variablesFilePath)
				fmt.Println("null")
				fmt.Fprintf(os.Stderr, "warning: data JSON not found for %q (expected %s)\n", query.Name(), variablesFilePath)
			} else {
				pretty, err := json.MarshalIndent(compileResult.Variables, "", "  ")
				if err != nil {
					return fmt.Errorf("pretty-printing variables for %q: %w", query.Name(), err)
				}
				fmt.Printf("--- data (%s) ---\n", variablesFilePath)
				fmt.Println(string(pretty))
			}
		}

		issue.VariablesMissing = compileResult.VariablesMissing

		varsRaw, err := jsonRawFromVars(compileResult.Variables)
		if err != nil {
			return fmt.Errorf("vars JSON for %q: %w", query.Name(), err)
		}

		validation, err := ValidateGraphjinQueryTablesAndColumns(
			gj,
			compileResult.Query,
			varsRaw,
			"user", // bypass permission blocking when role config isn't populated
			config.graphjinDev.EnableCamelcase,
		)
		if err != nil {
			issue.ValidationError = err.Error()
			if verbose {
				fmt.Fprintf(os.Stderr, "validation error for %q: %v\n", query.Name(), err)
				fmt.Println("----------------------------------------")
			}
			continue
		}

		issue.ExplainErrors = nil
		issue.MissingTables = nil
		issue.MissingColumns = nil

		issue.ExplainErrors = append(issue.ExplainErrors, validation.ExplainErrors...)
		issue.MissingTables = append(issue.MissingTables, validation.MissingTables...)

		missingColumnsMap := make(map[string][]string)
		for _, tr := range validation.Tables {
			if len(tr.MissingColumns) > 0 {
				missingColumnsMap[tr.Table] = tr.MissingColumns
			}
		}

		if len(missingColumnsMap) > 0 {
			issue.MissingColumns = missingColumnsMap
		}

		if verbose {
			anyMissing := len(validation.MissingTables) > 0
			for _, tr := range validation.Tables {
				if len(tr.MissingColumns) > 0 {
					anyMissing = true
					break
				}
			}

			fmt.Println("--- validation ---")
			if len(validation.ExplainErrors) > 0 {
				fmt.Printf("GraphJin ExplainQuery errors:\n")
				for _, e := range validation.ExplainErrors {
					fmt.Printf("  - %s\n", e)
				}
			}

			if !anyMissing && len(validation.ExplainErrors) == 0 {
				fmt.Printf("OK: tables=%d\n", len(validation.Tables))
			} else {
				if len(validation.MissingTables) > 0 {
					fmt.Printf("Missing tables: %s\n", strings.Join(validation.MissingTables, ", "))
				}
				for _, tr := range validation.Tables {
					if len(tr.MissingColumns) == 0 {
						continue
					}
					fmt.Printf("Table %s\n", tr.Table)
					if len(tr.ExistingColumns) > 0 {
						fmt.Printf("  existing columns: %s\n", strings.Join(tr.ExistingColumns, ", "))
					}
					fmt.Printf("  missing columns: %s\n", strings.Join(tr.MissingColumns, ", "))
				}
			}

			fmt.Println("----------------------------------------")
		}
	}

	if foundQueries == 0 {
		fmt.Fprintf(os.Stdout, "warning: no .graphql or .gql files found in %s\n", queriesDir)
	}

	// Build aggregated summary (used for TUI and JSON output modes).
	keys := make([]string, 0, len(byBase))
	for k := range byBase {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	files := make([]FileIssueSummary, 0, len(keys))
	totals := SummaryTotals{
		TotalQueries: foundQueries,
	}
	for _, k := range keys {
		f := byBase[k]
		files = append(files, *f)

		explainCount := len(f.ExplainErrors)
		missingTablesCount := len(f.MissingTables)
		missingColsCount := missingColumnsCount(f.MissingColumns)

		hasAny := f.HasIssues()
		if hasAny {
			totals.FilesWithAnyIssues++
		}

		if explainCount > 0 {
			totals.ExplainErrorFiles++
			totals.TotalExplainErrors += explainCount
		}
		if missingTablesCount > 0 {
			totals.MissingTablesFiles++
			totals.TotalMissingTables += missingTablesCount
		}
		if missingColsCount > 0 {
			totals.MissingColumnsFiles++
			totals.TotalMissingColumns += missingColsCount
		}
		if f.ValidationError != "" {
			totals.ValidationErrorFiles++
		}
		if f.VariablesMissing {
			totals.VariablesMissingFiles++
		}
	}

	summary := ValidationSummary{
		Totals: totals,
		Files:  files,
	}

	// Output behavior:
	// - --json always writes the JSON summary (and skips TUI).
	// - default (non-verbose, no json): render concise TUI summary.
	// - --verbose: retain the original per-query output (no TUI summary).
	if strings.TrimSpace(jsonPath) != "" {
		return writeValidationSummaryJSON(jsonPath, summary)
	}
	if !verbose {
		renderValidationSummaryTUI(summary)
	}
	return nil
}
