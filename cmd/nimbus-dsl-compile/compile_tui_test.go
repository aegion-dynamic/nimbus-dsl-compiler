package main

import (
	"strings"
	"testing"
)

func TestFormatMissingColumnsDetail(t *testing.T) {
	got := formatMissingColumnsDetail(map[string][]string{
		"users":   {"email", "id"},
		"accounts": {"balance"},
	}, 0)
	want := "accounts: balance; users: email, id"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}

	full := formatMissingColumnsDetail(map[string][]string{
		"t": {"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"},
	}, 0)
	short := formatMissingColumnsDetail(map[string][]string{
		"t": {"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"},
	}, 30)
	if !strings.HasSuffix(short, "…") || len(short) >= len(full) {
		t.Fatalf("truncation: full len %d, short %q", len(full), short)
	}
}
