package main

// ExecutionSummary is embedded into the same --json output file as the
// validation summary, when execution is triggered via --execute.
type ExecutionSummary struct {
	// HardFailures counts queries/mutations that failed before producing a
	// GraphJin response (eg. couldn't load query/vars, or GraphJin returned an
	// execution error).
	HardFailures int `json:"hard_failures"`

	Errors []ExecutionError `json:"errors,omitempty"`
}

type ExecutionError struct {
	QueryBase string `json:"query_base"`
	QueryFile string `json:"query_file"`

	// Load/variables errors happen before GraphJin execution.
	LoadError      string `json:"load_error,omitempty"`
	VariablesError string `json:"variables_error,omitempty"`

	// MissingVariables is set when required GraphQL variables are absent (or the JSON file is missing).
	MissingVariables []string `json:"missing_variables,omitempty"`

	// ExecutionError is only set when gj.GraphQL returned a non-nil `err`.
	ExecutionError string `json:"execution_error,omitempty"`

	// GraphJinErrors are set when gj.GraphQL returned a response with
	// response-level errors (res.Errors), even when `err` is nil.
	GraphJinErrors []string `json:"graphjin_errors,omitempty"`
}

