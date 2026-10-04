package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/url"
	"sort"
	"strings"
	"unicode"

	_ "github.com/lib/pq"

	graphschema "github.com/chirino/graphql/schema"
	"github.com/aegion-dynamic/graphjin-slim/core/v3"
	"github.com/aegion-dynamic/graphjin-slim/core/v3/langadapter"
	"github.com/aegion-dynamic/graphjin-slim/core/v3/sdata"
	_ "github.com/aegion-dynamic/graphjin-slim/graphql/v3"
)

type TableColumnValidation struct {
	Table string `json:"table"`

	// ExistingColumns are DB-backed columns that were requested in the query.
	ExistingColumns []string `json:"existing_columns"`

	// MissingColumns are requested column-fields that don't exist on the DB table.
	MissingColumns []string `json:"missing_columns"`
}

type QueryValidationReport struct {
	// ExplainErrors are GraphJin compilation errors returned by ExplainQuery.
	ExplainErrors []string `json:"explain_errors,omitempty"`

	// MissingTables are tables referenced by the query that don't exist in the DB schema.
	MissingTables []string `json:"missing_tables,omitempty"`

	// Tables contains per-table column validation results.
	Tables map[string]*TableColumnValidation `json:"tables"`
}

// NewGraphJinFromDevConfig creates an initialized GraphJin engine for query compilation
// (schema discovery + query compiler), without enforcing production allow-lists/role blocking.
func NewGraphJinFromDevConfig(dev graphjinDevConfig) (*core.GraphJin, *sql.DB, error) {
	if strings.TrimSpace(dev.Database.Type) == "" {
		dev.Database.Type = "postgres"
	}
	if strings.TrimSpace(dev.Database.Schema) == "" {
		// GraphJin defaults to `public` when a schema isn't configured. We want
		// to introspect/resolve tables from our application schema instead.
		dev.Database.Schema = "application"
	}

	connStr := dev.Database.ConnString
	if strings.TrimSpace(connStr) == "" {
		// GraphJin uses database/sql, so we need a driver-registered SQL DSN.
		sslmode := "disable"
		if dev.Database.EnableTLS {
			sslmode = "require"
		}

		// URL-escape user/password so special characters don't break the DSN.
		user := url.QueryEscape(dev.Database.User)
		pass := url.QueryEscape(dev.Database.Password)
		host := strings.TrimSpace(dev.Database.Host)
		if host == "" {
			host = "localhost"
		}
		dbname := strings.TrimSpace(dev.Database.DBName)

		connStr = fmt.Sprintf("postgres://%s:%s@%s:%d/%s?sslmode=%s",
			user, pass, host, dev.Database.Port, dbname, sslmode,
		)
	}

	// GraphJin's schema discovery uses Postgres `current_schema()`, which is
	// derived from the connection's `search_path`. Ensure we point at the
	// intended schema so it doesn't default to `public`.
	if schema := strings.TrimSpace(dev.Database.Schema); schema != "" && !strings.EqualFold(schema, "public") {
		// Avoid duplicating options if a caller provided a DSN with them.
		if !strings.Contains(connStr, "options=") {
			optionsVal := fmt.Sprintf("-csearch_path=%s", schema)
			delimiter := "?"
			if strings.Contains(connStr, "?") {
				delimiter = "&"
			}
			connStr = fmt.Sprintf("%s%voptions=%s", connStr, delimiter, url.QueryEscape(optionsVal))
		}
	}

	db, err := sql.Open("postgres", connStr)
	if err != nil {
		return nil, nil, fmt.Errorf("open postgres: %w", err)
	}

	// Fail fast on bad connection info.
	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, nil, fmt.Errorf("ping postgres: %w", err)
	}

	// Minimal core.Config: schema discovery + compilation works with DB introspection.
	// Role-based blocking is bypassed by compiling as `user`, and production security is disabled.
	conf := &core.Config{
		DBType:           dev.Database.Type,
		EnableCamelcase: dev.EnableCamelcase,
		Production:      dev.Production,
		SecretKey:       dev.SecretKey,

		// Ensure GraphJin resolves tables/columns from the intended schema
		// instead of defaulting to `public`.
		Databases: map[string]core.DatabaseConfig{
			core.DefaultDBName: {
				Type:   dev.Database.Type,
				Schema: dev.Database.Schema,
			},
		},

		DisableAllowList:    true,  // bypass allow-list workflow
		DisableProdSecurity: true,  // bypass production-level enforcement
		EnableSchema:        false, // avoid DDL side-effects
		EnableIntrospection: false, // avoid writing introspection files
	}

	gj, err := core.NewGraphJin(conf, db)
	if err != nil {
		_ = db.Close()
		return nil, nil, fmt.Errorf("core.NewGraphJin: %w", err)
	}

	return gj, db, nil
}

// camelToSnake converts lowerCamelCase or UpperCamelCase to snake_case.
// This mirrors the behavior needed when GraphJin has enable_camelcase enabled.
func camelToSnake(s string) string {
	if s == "" {
		return s
	}

	var b strings.Builder
	b.Grow(len(s) + 4)

	// Track previous rune category to decide where underscores belong.
	var prevLowerOrDigit bool
	var prevUpper bool

	for i, r := range s {
		if i == 0 {
			b.WriteRune(unicode.ToLower(r))
			prevUpper = isUpper(r)
			prevLowerOrDigit = isLower(r) || isDigit(r)
			continue
		}

		isU := isUpper(r)
		isL := isLower(r)
		isD := isDigit(r)

		if isU {
			// Insert underscore when we transition:
			// - lower/digit -> upper (fooBar => foo_bar)
			// - upper -> upper where the previous was followed by a lower (HTTPServer => http_server)
			if prevLowerOrDigit || (prevUpper && nextIsLower(s, i)) {
				b.WriteByte('_')
			}
			b.WriteRune(unicode.ToLower(r))
		} else {
			b.WriteRune(r)
		}

		prevUpper = isU
		prevLowerOrDigit = isL || isD
	}

	return b.String()
}

func isUpper(r rune) bool { return r >= 'A' && r <= 'Z' }
func isLower(r rune) bool { return r >= 'a' && r <= 'z' }
func isDigit(r rune) bool { return r >= '0' && r <= '9' }

func nextIsLower(s string, idx int) bool {
	if idx+1 >= len(s) {
		return false
	}
	r := rune(s[idx+1])
	return isLower(r)
}

// normalizeFieldName applies the same normalization GraphJin applies when enable_camelcase is on.
func normalizeFieldName(field string, enableCamelcase bool) string {
	if enableCamelcase {
		return camelToSnake(field)
	}
	return field
}

// parseGraphQLDocument parses the query string into GraphQL AST nodes.
// This is used for column extraction since ExplainQuery only returns tables touched.
func parseGraphQLDocument(query string) (*graphschema.QueryDocument, error) {
	doc := &graphschema.QueryDocument{}
	if err := doc.Parse(query); err != nil {
		return nil, fmt.Errorf("graphql parse: %w", err)
	}
	return doc, nil
}

func variableDefinitionRequiresExplicitBinding(iv *graphschema.InputValue) bool {
	if iv == nil {
		return false
	}
	if iv.Default != nil {
		return false
	}
	_, nonNull := iv.Type.(*graphschema.NonNull)
	return nonNull
}

func collectOperationVariableDefinitions(doc *graphschema.QueryDocument) []*graphschema.InputValue {
	byName := make(map[string]*graphschema.InputValue)
	for _, op := range doc.Operations {
		for _, v := range op.Vars {
			if v == nil {
				continue
			}
			nm := strings.TrimPrefix(v.Name, "$")
			if nm == "" {
				continue
			}
			iv := *v
			iv.Name = nm
			byName[nm] = &iv
		}
	}
	out := make([]*graphschema.InputValue, 0, len(byName))
	for _, v := range byName {
		out = append(out, v)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// MissingSuppliedGraphQLVariables returns variable names that are not satisfied by the JSON variables object.
//
// If variablesFileMissing is true (no sidecar JSON file), every variable declared on the document's
// operations is listed, since the whole payload is absent.
//
// If the JSON file exists, only variables that are non-null with no default and are missing as object
// keys are listed. (Omitted nullable variables are not reported.)
func MissingSuppliedGraphQLVariables(query string, vars any, variablesFileMissing bool) ([]string, error) {
	doc, err := parseGraphQLDocument(query)
	if err != nil {
		return nil, err
	}
	defer doc.Close()

	defs := collectOperationVariableDefinitions(doc)
	if len(defs) == 0 {
		return []string{}, nil
	}

	supplied := make(map[string]struct{})
	if vars != nil {
		m, ok := vars.(map[string]any)
		if ok {
			for k := range m {
				supplied[k] = struct{}{}
			}
		}
	}

	missing := make([]string, 0, len(defs))
	for _, d := range defs {
		_, present := supplied[d.Name]
		if variablesFileMissing {
			missing = append(missing, d.Name)
			continue
		}
		if present {
			continue
		}
		if variableDefinitionRequiresExplicitBinding(d) {
			missing = append(missing, d.Name)
		}
	}
	return missing, nil
}

// jsonRawFromVars converts vars (already decoded) into json.RawMessage for ExplainQuery.
func jsonRawFromVars(vars any) (json.RawMessage, error) {
	if vars == nil {
		return json.RawMessage(`{}`), nil
	}
	b, err := json.Marshal(vars)
	if err != nil {
		return nil, fmt.Errorf("marshal vars: %w", err)
	}
	return json.RawMessage(b), nil
}

// ValidateGraphjinQueryTablesAndColumns validates:
// 1) referenced tables exist
// 2) requested column-fields exist within each referenced table
//
// Compiles the query via the langadapter compiler, then walks the AST for per-table column checks.
func ValidateGraphjinQueryTablesAndColumns(
	gj *core.GraphJin,
	query string,
	vars json.RawMessage,
	role string,
	enableCamelcase bool,
) (*QueryValidationReport, error) {
	report := &QueryValidationReport{
		Tables: make(map[string]*TableColumnValidation),
	}

	// 1) Compile the query against the engine's discovered schema using the
	// same langadapter compiler GraphJin Slim's OpenAPI generator uses. Any
	// compile error means the query is invalid GraphJin QL, and is mirrored
	// into ExplainErrors the same way ExplainQuery errors were before.
	inputs, specErr := gj.OpenAPISpecInputs(nil)
	if specErr != nil {
		return report, fmt.Errorf("openapi spec inputs: %w", specErr)
	}
	dbSchema := inputs.Databases[core.DefaultDBName]
	if dbSchema == nil {
		return report, fmt.Errorf("no discovered schema for the default database")
	}

	langDesc, lerr := langadapter.Lookup(langadapter.DefaultLanguageName)
	if lerr != nil {
		return report, lerr
	}
	compilerFactory, ok := langDesc.(langadapter.CompilerFactory)
	if !ok {
		return report, fmt.Errorf("language %q does not provide a compiler", langadapter.DefaultLanguageName)
	}
	qcc, err := compilerFactory.NewCompiler(dbSchema, langadapter.CompileConfig{
		DBSchema:        dbSchema.DBSchema(),
		EnableCamelcase: enableCamelcase,
	})
	if err != nil {
		return report, fmt.Errorf("new query compiler: %w", err)
	}
	var varsMap map[string]json.RawMessage
	if len(vars) > 0 {
		if err := json.Unmarshal(vars, &varsMap); err != nil {
			return report, fmt.Errorf("vars JSON: %w", err)
		}
	}
	if _, err := qcc.Compile([]byte(query), varsMap, langadapter.CompileOptions{}); err != nil {
		report.ExplainErrors = append(report.ExplainErrors, err.Error())
	}

	// 2) Column existence via GraphQL AST walk + sdata.DBSchema lookups.
	doc, err := parseGraphQLDocument(query)
	if err != nil {
		return report, err
	}
	defer doc.Close()

	// Per-table caches to reduce repeated Find calls.
	tableSchemaCache := make(map[string]*sdata.DBTable)
	getOrFetchTableSchema := func(tableName string) (*sdata.DBTable, error) {
		if ts, ok := tableSchemaCache[tableName]; ok {
			return ts, nil
		}
		t, err := dbSchema.Find("", tableName)
		if err != nil {
			return nil, err
		}
		tableSchemaCache[tableName] = &t
		return &t, nil
	}

	colNamesSet := func(ts *sdata.DBTable) map[string]struct{} {
		s := make(map[string]struct{}, len(ts.Columns))
		for _, c := range ts.Columns {
			s[c.Name] = struct{}{}
		}
		return s
	}

	relFieldToTable := func(ts *sdata.DBTable) map[string]string {
		items, err := dbSchema.GetFirstDegree(*ts)
		if err != nil {
			return nil
		}
		m := make(map[string]string, len(items))
		for _, r := range items {
			m[r.Name] = r.Table.Name
		}
		return m
	}

	// walkSelections walks the selection set under a known table schema,
	// collecting which column-fields were requested and which are missing.
	var walkSelections func(ts *sdata.DBTable, sels graphschema.SelectionList)
	walkSelections = func(ts *sdata.DBTable, sels graphschema.SelectionList) {
		if ts == nil {
			return
		}

		tableKey := ts.Name
		colSet := colNamesSet(ts)
		relMap := relFieldToTable(ts)

		v := report.Tables[tableKey]
		if v == nil {
			v = &TableColumnValidation{Table: tableKey}
			report.Tables[tableKey] = v
		}

		for _, sel := range sels {
			switch s := sel.(type) {
			case *graphschema.FieldSelection:
				field := normalizeFieldName(s.Name, enableCamelcase)

				if _, ok := colSet[field]; ok {
					v.ExistingColumns = appendIfMissing(v.ExistingColumns, field)
					continue
				}

				if relTable, ok := relMap[field]; ok {
					// Follow relationship only when there's a nested selection set.
					if len(s.Selections) > 0 {
						childTS, err := getOrFetchTableSchema(relTable)
						if err == nil {
							walkSelections(childTS, s.Selections)
						}
					}
					continue
				}

				// Unknown field inside a table selection set is treated as a missing column-field.
				v.MissingColumns = appendIfMissing(v.MissingColumns, field)

			default:
				// Inline fragments and fragment spreads: walk their expanded selection sets.
				if expanded := sel.GetSelections(doc); len(expanded) > 0 {
					walkSelections(ts, expanded)
				}
			}
		}
	}

	// Start the walk from each top-level table selection in each operation.
	for _, op := range doc.Operations {
		for _, sel := range op.Selections {
			rootField, ok := sel.(*graphschema.FieldSelection)
			if !ok {
				continue
			}
			rootTable := normalizeFieldName(rootField.Name, enableCamelcase)
			ts, err := getOrFetchTableSchema(rootTable)
			if err != nil {
				report.MissingTables = appendIfMissing(report.MissingTables, rootTable)
				continue
			}

			walkSelections(ts, rootField.Selections)
		}
	}

	return report, nil
}

func appendIfMissing(dst []string, v string) []string {
	for _, x := range dst {
		if x == v {
			return dst
		}
	}
	return append(dst, v)
}

