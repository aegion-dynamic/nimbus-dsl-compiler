# nimbus-dsl-compile

`nimbus-dsl-compile` is a small CLI that reads GraphQL query files from a config folder and validates that the referenced GraphJin tables/columns exist in the configured Postgres schema.

It does **not** execute the mutations/queries. Instead, it:

1. Loads GraphJin config from `dev.yaml` (subset of keys).
2. Connects to Postgres via `config/dev.yaml`.
3. For each `config/queries/*.gql` / `*.graphql` file:
   1. Loads matching variables JSON from `config/queries/<name>.json` (optional).
   2. Calls GraphJin `ExplainQuery` to discover touched tables.
   3. Walks the GraphQL AST to verify requested fields exist as columns (and follows relationship selections).
4. Prints a per-query validation report to stdout (warnings/errors to stderr).

## Repo layout

- `main.go`: CLI entrypoint.
- `compile.go`: iterates query/variable files and prints results.
- `config.go`: loads `dev.yaml` and the queries folder path.
- `graphjin_validate.go`: performs table/column validation using GraphJin + Postgres introspection.
- `config/`: example GraphJin config and query files used by this tool.
  - `config/dev.yaml`
  - `config/queries/*.gql` and `config/queries/*.json`

## Requirements

- Go `>= 1.25` (see `go.mod`).
- A reachable Postgres database matching the schema configured in `config/dev.yaml`.
- A working GraphJin config in `config/dev.yaml`.

## Usage

### Run against the provided example config

```bash
go run . ./config
```

### Build and run

```bash
go build -o nimbus-dsl-compile ./
./nimbus-dsl-compile ./config
```

### DB connectivity check

Ping the configured database and exit:

```bash
./nimbus-dsl-compile ./config --db-status
```

## CLI arguments

- Positional `configFolder`: path to a folder containing `dev.yaml` and `queries/`.
- `--db-status`: ping the configured database and exit.

## Config format

### `dev.yaml`

This tool unmarshals `config/dev.yaml` into a GraphJin "dev" struct, but only the following keys are required for validation:

- `production`
- `secret_key`
- `enable_camelcase` (optional; defaults to `false`)
- `database`
  - `type` (defaults to `postgres`)
  - `host`
  - `port`
  - `dbname`
  - `user`
  - `password`
  - `schema` (defaults to `application`)
  - `enable_tls`
  - `server_name`
  - `connection_string`

All other GraphJin keys may be present; they are ignored by this tool.

### `queries/` folder

Query files:

- `*.gql` or `*.graphql` are processed.

Variables files (optional):

- For each query file `NAME.gql` / `NAME.graphql`, this tool looks for `NAME.json` in the same folder.
- If `NAME.json` is missing, validation still runs, but the tool prints a warning and treats variables as missing for the query’s ExplainQuery call.

## Output

For each query, the tool prints:

- The query file path and its contents.
- The variables JSON (pretty-printed) if present, otherwise `null`.
- A validation section describing:
  - Missing tables (if GraphJin cannot find them in schema introspection).
  - Missing columns/fields per table (based on GraphQL AST traversal).

## Notes / behavior

- Validation intentionally bypasses GraphJin production security and allow-list checks (so you can validate schema/columns without being blocked by runtime authorization).
- It uses a fixed role value of `user` when calling `ExplainQuery`.
- When `enable_camelcase` is enabled in `dev.yaml`, the tool normalizes requested field names from `lowerCamelCase` / `UpperCamelCase` to `snake_case` to match GraphJin’s SQL column naming behavior.

