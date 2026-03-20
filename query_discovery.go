package main

import (
	"os"
	"path/filepath"
	"strings"
)

// queryFileExtensions is the shared source of truth for what file types
// `Compile()` / `Execute()` should consider as runnable GraphQL operations.
var queryFileExtensions = map[string]struct{}{
	".gql":     {},
	".graphql": {},
}

type QueryFilePair struct {
	Base            string
	QueryFileName   string
	QueryFilePath   string
	VariablesPath   string
	VariablesExists bool
}

func discoverQueryFilePairs(queriesDir string) ([]QueryFilePair, error) {
	entries, err := os.ReadDir(queriesDir)
	if err != nil {
		return nil, err
	}

	pairs := make([]QueryFilePair, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		name := entry.Name()
		ext := strings.ToLower(filepath.Ext(name))
		if _, ok := queryFileExtensions[ext]; !ok {
			continue
		}

		base := strings.TrimSuffix(name, ext)
		queryFilePath := filepath.Join(queriesDir, name)
		variablesPath := filepath.Join(queriesDir, base+".json")

		_, varsErr := os.Stat(variablesPath)
		varsExists := varsErr == nil

		pairs = append(pairs, QueryFilePair{
			Base:            base,
			QueryFileName:  name,
			QueryFilePath:  queryFilePath,
			VariablesPath:  variablesPath,
			VariablesExists: varsExists,
		})
	}

	return pairs, nil
}

