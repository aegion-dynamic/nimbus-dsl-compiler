package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
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

func Compile(config *Config) error {
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

		fmt.Println("----------------------------------------")
	}

	if foundQueries == 0 {
		fmt.Fprintf(os.Stdout, "warning: no .graphql or .gql files found in %s\n", queriesDir)
	}

	return nil
}
