package main

import (
	"reflect"
	"testing"
)

func TestMissingSuppliedGraphQLVariables_noVariablesDeclared(t *testing.T) {
	q := `query { accounts { id } }`
	got, err := MissingSuppliedGraphQLVariables(q, map[string]any{}, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("got %v, want empty", got)
	}
}

func TestMissingSuppliedGraphQLVariables_fileMissing_listsAllDeclared(t *testing.T) {
	q := `query Q($id: ID!, $name: String) { accounts { id } }`
	got, err := MissingSuppliedGraphQLVariables(q, nil, true)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"id", "name"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestMissingSuppliedGraphQLVariables_jsonPresent_requiredMissing(t *testing.T) {
	q := `mutation M($id: ID!, $label: String) { updateThing(id: $id) { id } }`
	got, err := MissingSuppliedGraphQLVariables(q, map[string]any{"id": "1"}, false)
	if err != nil {
		t.Fatal(err)
	}
	// $label is nullable and has no default — omission is valid in GraphQL, so not reported.
	want := []string{}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}

	got2, err := MissingSuppliedGraphQLVariables(q, map[string]any{}, false)
	if err != nil {
		t.Fatal(err)
	}
	want2 := []string{"id"}
	if !reflect.DeepEqual(got2, want2) {
		t.Fatalf("got %v, want %v", got2, want2)
	}
}

func TestMissingSuppliedGraphQLVariables_defaultAllowsOmit(t *testing.T) {
	q := `query Q($page: Int = 1) { accounts { id } }`
	got, err := MissingSuppliedGraphQLVariables(q, map[string]any{}, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("got %v, want empty (default covers $page)", got)
	}
}
