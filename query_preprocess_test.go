package main

import (
	"strings"
	"testing"

	graphschema "github.com/chirino/graphql/schema"
)

func TestPreprocessGraphQLQueryRemoveTypename_Basic(t *testing.T) {
	in := `
query {
	getUser {
		id
		__typename
		name
	}
}
`

	out := preprocessGraphQLQueryRemoveTypename(in)
	if strings.Contains(out, "__typename") {
		t.Fatalf("expected __typename to be removed, got:\n%s", out)
	}

	doc := &graphschema.QueryDocument{}
	if err := doc.Parse(out); err != nil {
		t.Fatalf("expected preprocessed query to parse: %v\nquery:\n%s", err, out)
	}
}

func TestPreprocessGraphQLQueryRemoveTypename_WithAliasAndDirective(t *testing.T) {
	in := `
query {
	getUser {
		id
		alias1: __typename @skip(if: true)
	}
}
`

	out := preprocessGraphQLQueryRemoveTypename(in)
	if strings.Contains(out, "__typename") {
		t.Fatalf("expected __typename to be removed, got:\n%s", out)
	}

	doc := &graphschema.QueryDocument{}
	if err := doc.Parse(out); err != nil {
		t.Fatalf("expected preprocessed query to parse: %v\nquery:\n%s", err, out)
	}
}
