# nimbus-dsl-compile

`nimbus-dsl-compile` is a small CLI that reads GraphQL query files from a config folder and validates that the referenced GraphJin tables/columns exist in the configured Postgres schema.

By default, it does **not** execute the mutations/queries. Instead, it validates them:

1. Loads GraphJin config from `dev.yaml` (subset of keys).
2. Connects to Postgres via `config/dev.yaml`.
3. For each `config/queries/*.gql` / `*.graphql` file:
   1. Loads matching variables JSON from `config/queries/<name>.json` (optional).
   2. Calls GraphJin `ExplainQuery` to discover touched tables.
   3. Walks the GraphQL AST to verify requested fields exist as columns (and follows relationship selections).
4. Prints a per-query validation report to stdout (warnings/errors to stderr).
5. (Optional) When `--execute` is provided, it also executes the queries/mutations via GraphJin and prints the raw response `data`/`errors`.

## Repo layout

- `cmd/nimbus-dsl-compile/`: CLI (`main.go` entrypoint, compile/config/validation helpers).
- `config/`: example GraphJin config and query files used by this tool.
  - `config/dev.yaml`
  - `config/queries/*.gql` and `config/queries/*.json`

## Requirements

- Go `>= 1.25` (see `go.mod`) — only if you install or run from source with `go install` / `go run`.
- A reachable Postgres database matching the schema configured in `config/dev.yaml`.
- A working GraphJin config in `config/dev.yaml`.

## Installation

The compiled program is always invoked as **`nimbus-dsl-compile`** on your shell `PATH` (that name comes from the `cmd/nimbus-dsl-compile` package path, not from the repository name `nimbus-dsl-compiler`).

### With Go (`go install`)

Install from GitHub (binary is written to `$(go env GOPATH)/bin` unless you set `GOBIN`; that directory must be on your `PATH`):

```bash
go install github.com/aegion-dynamic/nimbus-dsl-compiler/cmd/nimbus-dsl-compile@latest
```

Pin to a specific tag when you publish releases:

```bash
go install github.com/aegion-dynamic/nimbus-dsl-compiler/cmd/nimbus-dsl-compile@v1.2.3
```

From a clone of this repository:

```bash
go install ./cmd/nimbus-dsl-compile
```

### Prebuilt binaries (GitHub Releases)

Prebuilt archives are attached to [GitHub Releases](https://github.com/aegion-dynamic/nimbus-dsl-compiler/releases) for this repo.

Linux/macOS:
```bash
OS_ASSET="$(uname -s)"
case "$OS_ASSET" in
  Linux) OS_ASSET="linux" ;;
  Darwin) OS_ASSET="darwin" ;;
  *) echo "Unsupported OS: $OS_ASSET" >&2; exit 1 ;;
esac

ARCH_ASSET="$(uname -m)"
case "$ARCH_ASSET" in
  x86_64|amd64) ARCH_ASSET="amd64" ;;
  arm64|aarch64) ARCH_ASSET="arm64" ;;
  *) echo "Unsupported architecture: $ARCH_ASSET" >&2; exit 1 ;;
esac

curl -L -o nimbus-dsl-compile.tar.gz \
  "https://github.com/aegion-dynamic/nimbus-dsl-compiler/releases/latest/download/nimbus-dsl-compile_${OS_ASSET}_${ARCH_ASSET}.tar.gz"
tar -xzf nimbus-dsl-compile.tar.gz
chmod +x nimbus-dsl-compile
# Move to somewhere on your PATH (adjust for your OS)
sudo mv nimbus-dsl-compile /usr/local/bin/
```

Windows (PowerShell):
```powershell
$arch = [System.Runtime.InteropServices.RuntimeInformation]::ProcessArchitecture
$arch = switch ($arch) {
  "X64" { "amd64" }
  "Arm64" { "arm64" }
  default { throw "Unsupported architecture: $arch" }
}

curl -L -o nimbus-dsl-compile.zip "https://github.com/aegion-dynamic/nimbus-dsl-compiler/releases/latest/download/nimbus-dsl-compile_windows_$arch.zip"
Expand-Archive -Force nimbus-dsl-compile.zip -DestinationPath .
```

## Usage

After installation, run the tool as **`nimbus-dsl-compile`**, passing the config directory as the first argument:

```bash
nimbus-dsl-compile /path/to/config
```

Example using the bundled sample config from a repository clone:

```bash
nimbus-dsl-compile ./config
```

### Run from source (no install)

```bash
go run ./cmd/nimbus-dsl-compile ./config
```

### Build a local binary

```bash
go build -o nimbus-dsl-compile ./cmd/nimbus-dsl-compile
./nimbus-dsl-compile ./config
```

### DB connectivity check

```bash
nimbus-dsl-compile ./config --db-status
```

## CLI arguments

- Positional `configFolder`: path to a folder containing `dev.yaml` and `queries/`.
- `--db-status`: ping the configured database and exit.
- `--verbose`: print the full per-query output (query text, variables, and per-table details).
- `--execute`: execute queries/mutations after validation (prints GraphJin response to stdout/stderr).
- `--json=STRING`: write a machine-readable JSON summary (totals + per-file breakdown) to the given file path. When `--json` is set, the TUI summary is skipped; if you also pass `--execute` and any execution issues occur, an `execution` section is added to this JSON.

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

By default (no `--verbose`), the tool prints a concise validation summary:

- Totals across all processed query files.
- A per-file breakdown of what categories of validation errors were found (missing tables, missing columns, GraphJin ExplainQuery errors, etc.).

When `--verbose` is set, it retains the original behavior of printing each query’s full contents and variables, along with a detailed validation section.

When `--json` is set, it additionally writes a JSON file containing the same aggregated summary model (totals + per-file breakdown).

## Notes / behavior

- Validation intentionally bypasses GraphJin production security and allow-list checks (so you can validate schema/columns without being blocked by runtime authorization).
- It uses a fixed role value of `user` when calling `ExplainQuery`.
- When `--execute` is provided, it also executes each `*.gql` / `*.graphql` file with a fixed role value of `user`.
- When `enable_camelcase` is enabled in `dev.yaml`, the tool normalizes requested field names from `lowerCamelCase` / `UpperCamelCase` to `snake_case` to match GraphJin’s SQL column naming behavior.

