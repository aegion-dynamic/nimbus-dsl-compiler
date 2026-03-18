package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

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

func processQuery(queryFilePath, variablesFilePath string) (*CompileResult, error) {
	query, err := os.ReadFile(queryFilePath)
	if err != nil {
		return nil, err
	}

	res := &CompileResult{
		Query:   string(query),
		Errors:  []CompileError{},
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

func Compile(config *Config, gj *core.GraphJin) error {
	queriesDir := config.ConfigFileLocations.QueriesFolderPath
	fmt.Printf("reading queries from %s\n", queriesDir)

	queries, err := os.ReadDir(queriesDir)
	if err != nil {
		return err
	}

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
			fmt.Fprintf(os.Stderr, "validation error for %q: %v\n", query.Name(), err)
			fmt.Println("----------------------------------------")
			continue
		}

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

	if foundQueries == 0 {
		fmt.Fprintf(os.Stdout, "warning: no .graphql or .gql files found in %s\n", queriesDir)
	}

	return nil
}
