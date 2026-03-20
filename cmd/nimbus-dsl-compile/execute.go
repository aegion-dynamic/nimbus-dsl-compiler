package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/dosco/graphjin/core/v3"
)

type ExecuteResult struct {
	Query     string
	Variables any
	Errors    []error
}

func Execute(config *Config, gj *core.GraphJin, verbose bool, jsonPath string) error {
	queriesDir := config.ConfigFileLocations.QueriesFolderPath
	pairs, err := discoverQueryFilePairs(queriesDir)
	if err != nil {
		return err
	}

	foundQueries := len(pairs)
	executionFailures := 0
	executionErrors := make([]ExecutionError, 0)

	for _, pair := range pairs {
		name := pair.QueryFileName
		base := pair.Base
		queryFilePath := pair.QueryFilePath
		variablesFilePath := pair.VariablesPath

		compileResult, err := processQuery(queryFilePath, variablesFilePath)
		if err != nil {
			executionFailures++
			fmt.Fprintf(os.Stderr, "failed loading query %q: %v\n", name, err)
			executionErrors = append(executionErrors, ExecutionError{
				QueryBase: base,
				QueryFile: name,
				LoadError: err.Error(),
			})
			continue
		}

		varsRaw, err := jsonRawFromVars(compileResult.Variables)
		if err != nil {
			executionFailures++
			fmt.Fprintf(os.Stderr, "failed preparing variables for %q: %v\n", name, err)
			executionErrors = append(executionErrors, ExecutionError{
				QueryBase: base,
				QueryFile: name,
				VariablesError: err.Error(),
			})
			continue
		}

		roleCtx := context.WithValue(context.Background(), core.UserRoleKey, "user")
		res, execErr := gj.GraphQL(roleCtx, compileResult.Query, varsRaw, nil)
		if verbose {
			fmt.Printf("=== %s ===\n", base)
			fmt.Println(compileResult.Query)
			if compileResult.VariablesMissing {
				fmt.Fprintf(os.Stderr, "warning: data JSON not found for %q (expected %s)\n", name, variablesFilePath)
				fmt.Println("--- data ---")
				fmt.Println("null")
			} else {
				fmt.Println("--- data ---")
				fmt.Printf("%#v\n", compileResult.Variables)
			}
		}

		if execErr != nil {
			executionFailures++
			fmt.Fprintf(os.Stderr, "execution error for %q: %v\n", name, execErr)
			graphjinMsgs := make([]string, 0)
			if res != nil && len(res.Errors) > 0 {
				graphjinMsgs = make([]string, 0, len(res.Errors))
				for _, e := range res.Errors {
					fmt.Fprintf(os.Stderr, "  - %s\n", e.Message)
					graphjinMsgs = append(graphjinMsgs, e.Message)
				}
			}
			executionErrors = append(executionErrors, ExecutionError{
				QueryBase:       base,
				QueryFile:       name,
				ExecutionError:  execErr.Error(),
				GraphJinErrors: graphjinMsgs,
			})
			continue
		}

		// Print successful response.
		fmt.Printf("=== %s ===\n", base)
		if len(res.Data) > 0 {
			fmt.Println(string(res.Data))
		} else {
			fmt.Println("null")
		}
		if len(res.Errors) > 0 {
			graphjinMsgs := make([]string, 0, len(res.Errors))
			for _, e := range res.Errors {
				fmt.Fprintf(os.Stderr, "  - %s\n", e.Message)
				graphjinMsgs = append(graphjinMsgs, e.Message)
			}
			executionErrors = append(executionErrors, ExecutionError{
				QueryBase:       base,
				QueryFile:       name,
				GraphJinErrors: graphjinMsgs,
			})
		}
	}

	if foundQueries == 0 {
		fmt.Fprintf(os.Stdout, "warning: no .graphql or .gql files found in %s\n", queriesDir)
	}

	// Persist execution errors (if any) into the same JSON file provided via --json.
	// Compile() already writes the validation summary there; we enrich it with the
	// execution section.
	if strings.TrimSpace(jsonPath) != "" && len(executionErrors) > 0 {
		var summary ValidationSummary
		if existing, err := os.ReadFile(jsonPath); err == nil {
			_ = json.Unmarshal(existing, &summary) // best-effort merge
		}

		summary.Execution = &ExecutionSummary{
			HardFailures: executionFailures,
			Errors:       executionErrors,
		}
		if err := writeValidationSummaryJSON(jsonPath, summary); err != nil {
			return fmt.Errorf("writing execution errors json: %w", err)
		}
	}

	if executionFailures > 0 {
		return fmt.Errorf("%d query/mutation execution(s) failed", executionFailures)
	}
	return nil
}
