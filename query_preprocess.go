package main

import (
	"strings"

	graphschema "github.com/chirino/graphql/schema"
)

// preprocessGraphQLQueryRemoveTypename strips any `__typename` field selections from
// the GraphQL document before GraphJin compilation/AST validation.
func preprocessGraphQLQueryRemoveTypename(query string) string {
	if !strings.Contains(query, "__typename") {
		return query
	}

	doc := &graphschema.QueryDocument{}
	if err := doc.Parse(query); err != nil {
		// If the query can't be parsed, fall back to the original content.
		return query
	}

	changed := false

	var filterSelections func(sels graphschema.SelectionList) (graphschema.SelectionList, bool)
	filterSelections = func(sels graphschema.SelectionList) (graphschema.SelectionList, bool) {
		out := make(graphschema.SelectionList, 0, len(sels))
		localChanged := false

		for _, sel := range sels {
			switch s := sel.(type) {
			case *graphschema.FieldSelection:
				// FieldSelection.Name is the actual field name (not the alias).
				if s.Name == "__typename" {
					localChanged = true
					continue
				}

				if len(s.Selections) > 0 {
					filtered, childChanged := filterSelections(s.Selections)
					if childChanged {
						localChanged = true

						// Avoid emitting fields with empty selection sets.
						if len(filtered) == 0 {
							continue
						}
						s.Selections = filtered
					}
				}

				out = append(out, sel)

			case *graphschema.InlineFragment:
				if len(s.Selections) > 0 {
					filtered, childChanged := filterSelections(s.Selections)
					if childChanged {
						localChanged = true

						// Avoid emitting inline fragments with empty selection sets.
						if len(filtered) == 0 {
							continue
						}
						s.Selections = filtered
					}
				}

				out = append(out, sel)

			case *graphschema.FragmentSpread:
				// Fragment spreads don't directly contain selections; fragment definitions
				// are filtered separately below.
				out = append(out, sel)

			default:
				out = append(out, sel)
			}
		}

		return out, localChanged
	}

	for _, op := range doc.Operations {
		newSels, changedHere := filterSelections(op.Selections)
		// If we filtered everything out for the operation, keep the original to
		// avoid rendering an invalid query.
		if changedHere && len(newSels) > 0 {
			op.Selections = newSels
			changed = true
		}
	}

	for _, frag := range doc.Fragments {
		newSels, changedHere := filterSelections(frag.Selections)
		// Same safety as operations: if filtering would empty a fragment, keep it.
		if changedHere && len(newSels) > 0 {
			frag.Selections = newSels
			changed = true
		}
	}

	if !changed {
		return query
	}

	// Rerender; GraphQL semantics are preserved as long as we didn't remove all
	// selections from any operation/fragment.
	return strings.TrimSpace(doc.String()) + "\n"
}
