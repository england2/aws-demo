# sqlc With Go and SQLite

Research date: 2026-05-12

Primary sources:

- Official docs home: <https://docs.sqlc.dev/en/latest/>
- Installation: <https://docs.sqlc.dev/en/latest/overview/install.html>
- Configuration reference: <https://docs.sqlc.dev/en/latest/reference/config.html>
- Query annotations: <https://docs.sqlc.dev/en/latest/reference/query-annotations.html>
- Named parameters: <https://docs.sqlc.dev/en/latest/howto/named_parameters.html>
- Type overrides: <https://docs.sqlc.dev/en/latest/howto/overrides.html>
- Transactions: <https://docs.sqlc.dev/en/latest/howto/transactions.html>
- Struct configuration: <https://docs.sqlc.dev/en/latest/howto/structs.html>
- Vet and CI/CD: <https://docs.sqlc.dev/en/latest/howto/vet.html>, <https://docs.sqlc.dev/en/latest/howto/ci-cd.html>

## What sqlc Is

`sqlc` is a code generator that turns SQL schema files and named SQL queries into type-safe Go code. For this project, sqlc should be treated as the compiler for SQLite access: write SQLite schema and SQLite queries, run `sqlc generate`, then call the generated Go methods from scheduler/database code.

The basic workflow is:

1. Write SQLite schema SQL.
2. Write SQLite queries.
3. Annotate each query with a name and command, such as `-- name: GetJob :one`.
4. Configure `sqlc.yaml` with `engine: "sqlite"`.
5. Run `sqlc generate`.
6. Use the generated Go package with `database/sql` and the project's SQLite driver.

`sqlc` does not replace migrations. Use migrations or schema setup code to change real SQLite database files. sqlc uses schema files so it can infer table, column, parameter, and result types during code generation.

## Why Use It

`sqlc` is useful here because it keeps SQLite queries explicit while removing repetitive Go scan code:

- SQL remains visible and reviewable.
- Generated Go methods have typed parameters and typed results.
- Result scanning is generated.
- Schema/query mismatches are caught during generation.
- Transaction support is generated through `WithTx`.
- CI can check generated code drift with `sqlc diff`.

The main tradeoff is that query shape becomes part of the generated Go API. Renaming selected columns, changing query annotations, or changing result sets can change generated structs and method signatures.

## Installation

The official docs list several install paths:

```bash
brew install sqlc
sudo snap install sqlc
go install github.com/sqlc-dev/sqlc/cmd/sqlc@latest
docker pull sqlc/sqlc
```

For this repo, Go is installed at:

```bash
/home/t/.go/bin/go
```

If installing via Go, make sure the Go bin directory is on `PATH`, or call the binary by its full path after installation.

## Current Repo Shape

This repo already contains a version 2 sqlc config:

```yaml
# database-and-scheduler/sqlc/sqlc.yaml
version: "2"
sql:
  - engine: "sqlite"
    schema: "database.sql"
    queries: "queries"
    gen:
      go:
        package: "sqlc-gen"
        out: "db-scheduler/sqlc-gen"
        emit_json_tags: true
```

This means:

- The SQL engine is SQLite.
- `database-and-scheduler/sqlc/database.sql` defines the schema.
- Query files live under `database-and-scheduler/sqlc/queries`.
- Generated Go code is currently configured to write to `database-and-scheduler/sqlc/db-scheduler/sqlc-gen` when `sqlc generate` is run from `database-and-scheduler/sqlc`.
- Generated structs include JSON tags.

Run generation from the directory containing `sqlc.yaml`:

```bash
cd database-and-scheduler/sqlc
sqlc generate
```

The current query files are empty, so no useful query methods will be generated until queries are added. Also, the current `package: "sqlc-gen"` value is not a valid Go package identifier, and the current `out` path does not land in the existing `database-and-scheduler/db-scheduler` tree. See "Suggested Adjustments for This Repo" below.

## SQLite Config

Use config version 2:

```yaml
version: "2"
sql:
  - name: "scheduler"
    engine: "sqlite"
    schema: "database.sql"
    queries: "queries"
    gen:
      go:
        package: "sqlcgen"
        out: "../db-scheduler/generated/sqlcgen"
        emit_json_tags: true
        emit_empty_slices: true
        query_parameter_limit: 0
```

Important fields:

- `engine: "sqlite"`: tells sqlc to parse SQLite syntax and infer SQLite types.
- `schema`: a SQL file, directory of SQL files, or list of paths.
- `queries`: a SQL file, directory of SQL files, or list of paths.
- `gen.go.package`: the generated Go package name. It must be a valid Go identifier.
- `gen.go.out`: output directory for generated Go files, relative to the config file's working directory.

Useful Go generator options:

- `emit_json_tags: true`: add JSON tags to generated structs.
- `emit_db_tags: true`: add DB tags to generated structs.
- `emit_interface: true`: generate a `Querier` interface for easier testing and dependency injection.
- `emit_empty_slices: true`: return empty slices instead of `nil` for `:many` queries.
- `emit_exact_table_names: true`: keep table names exactly instead of singularizing generated struct names.
- `query_parameter_limit: 0`: always generate a params struct, even for one-argument queries. This keeps signatures more stable as queries evolve.
- `rename`: override generated Go field names.
- `overrides`: override SQLite-type-to-Go-type mappings when needed.

## Schema Files

`sqlc` needs SQLite schema SQL to infer generated Go types. Keep schema files in sync with whatever creates or migrates the actual SQLite database.

For this project, a simple structure is fine:

```text
database-and-scheduler/
  db-scheduler/
    generated/
      sqlcgen/
  sqlc/
    database.sql
    queries/
      agent_jobs.sql
    sqlc.yaml
```

SQLite example:

```sql
CREATE TABLE IF NOT EXISTS agent_job_info (
    id INTEGER PRIMARY KEY,
    agent_name TEXT NOT NULL,
    status TEXT NOT NULL,
    spawn_sqs_message_id INTEGER NOT NULL,
    agent_report TEXT,
    affected_repositories TEXT,
    pull_request_url TEXT,
    failure_reason TEXT,
    ecs_task_arn TEXT,
    ecs_last_status TEXT,
    ecs_stopped_reason TEXT,
    created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    started_at TEXT,
    completed_at TEXT,
    updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);
```

Best practices:

- Be explicit with `NOT NULL`; nullability directly affects generated Go types.
- Use `INTEGER PRIMARY KEY` when a rowid-backed primary key is desired.
- Store timestamps consistently. The current schema uses `TEXT` timestamps with `CURRENT_TIMESTAMP`, which is a reasonable SQLite pattern if the application treats them consistently.
- Do not rely on `PRAGMA foreign_keys = ON` in the schema file for runtime behavior. SQLite foreign keys must be enabled for actual database connections too.

## Query Files

Every sqlc query needs a name and command annotation:

```sql
-- name: GetAgentJob :one
SELECT id, agent_name, status, agent_report, created_at, completed_at
FROM agent_job_info
WHERE id = ?;
```

The generated Go method name comes from the query name. The command controls the return type and execution method.

Common SQLite-relevant annotations:

- `:one`: query returns one row. Generated method returns one model or result struct plus `error`.
- `:many`: query returns multiple rows. Generated method returns a slice plus `error`.
- `:exec`: statement does not return rows. Generated method returns `error`.
- `:execrows`: statement returns affected row count plus `error`.
- `:execresult`: statement returns the underlying driver result plus `error`.
- `:execlastid`: useful for SQLite insert flows that need the last inserted ID.

SQLite uses `?` placeholders. sqlc also supports named parameters through `sqlc.arg(name)` and, in SQLite, the shorter `@name` syntax.

SQLite examples:

```sql
-- name: ListAgentJobsByStatus :many
SELECT id,
       agent_name,
       status,
       spawn_sqs_message_id,
       agent_report,
       affected_repositories,
       pull_request_url,
       failure_reason,
       ecs_task_arn,
       ecs_last_status,
       ecs_stopped_reason,
       created_at,
       started_at,
       completed_at,
       updated_at
FROM agent_job_info
WHERE status = ?
ORDER BY created_at DESC;

-- name: CreateAgentJob :one
INSERT INTO agent_job_info (
    agent_name,
    status,
    spawn_sqs_message_id
) VALUES (
    ?, ?, ?
)
RETURNING id,
          agent_name,
          status,
          spawn_sqs_message_id,
          agent_report,
          affected_repositories,
          pull_request_url,
          failure_reason,
          ecs_task_arn,
          ecs_last_status,
          ecs_stopped_reason,
          created_at,
          started_at,
          completed_at,
          updated_at;

-- name: MarkAgentJobComplete :execrows
UPDATE agent_job_info
SET status = 'completed',
    completed_at = CURRENT_TIMESTAMP,
    updated_at = CURRENT_TIMESTAMP
WHERE id = ?;
```

Best practices:

- Avoid `SELECT *`; explicit column lists make generated result structs stable.
- Use stable query names because they become Go method names.
- Alias computed columns and aggregates so generated field names are readable.
- Prefer small, purpose-specific queries over one large query with many optional branches.
- Use `RETURNING` for inserts and updates when the application needs the resulting row.

## Named Parameters

Positional `?` placeholders are valid SQLite, but named parameters make generated params structs easier to read.

```sql
-- name: UpdateAgentJobStatus :execrows
UPDATE agent_job_info
SET status = @status,
    updated_at = CURRENT_TIMESTAMP
WHERE id = @id;
```

The more explicit form is:

```sql
-- name: UpdateAgentJobStatus :execrows
UPDATE agent_job_info
SET status = sqlc.arg(status),
    updated_at = CURRENT_TIMESTAMP
WHERE id = sqlc.arg(id);
```

If a parameter should be nullable, use `sqlc.narg()`:

```sql
-- name: PatchAgentReport :one
UPDATE agent_job_info
SET agent_report = coalesce(sqlc.narg(agent_report), agent_report),
    failure_reason = coalesce(sqlc.narg(failure_reason), failure_reason),
    updated_at = CURRENT_TIMESTAMP
WHERE id = sqlc.arg(id)
RETURNING id, agent_name, status, agent_report, failure_reason, updated_at;
```

## Generated Code

After running:

```bash
sqlc generate
```

sqlc generally emits files like:

```text
db.go
models.go
querier.go
<query-file>.sql.go
```

Generated SQLite code is used through `database/sql`. sqlc does not choose the SQLite driver; the application does. Common Go SQLite drivers include:

- `modernc.org/sqlite`: pure Go, driver name `sqlite`.
- `github.com/mattn/go-sqlite3`: common and mature, driver name `sqlite3`, requires CGO.

Example using `modernc.org/sqlite`:

```go
package scheduler

import (
	"context"
	"database/sql"

	"database-and-scheduler/db-scheduler/generated/sqlcgen"

	_ "modernc.org/sqlite"
)

type Store struct {
	db      *sql.DB
	queries *sqlcgen.Queries
}

func OpenStore(path string) (*Store, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}

	return &Store{
		db:      db,
		queries: sqlcgen.New(db),
	}, nil
}

func (s *Store) GetJob(ctx context.Context, id int64) (sqlcgen.AgentJobInfo, error) {
	return s.queries.GetAgentJob(ctx, id)
}
```

The exact import path depends on the module path and generated output path.

## Nullability and Go Types

Nullability matters:

- `NOT NULL` columns become ordinary Go values where sqlc has a direct mapping.
- Nullable columns become nullable wrappers or pointers depending on SQLite type and config.
- With `database/sql`, nullable text commonly becomes `sql.NullString`.

Best practices:

- Prefer `NOT NULL` with sensible defaults for fields that should always exist.
- Model truly optional fields as nullable in SQL and handle the generated nullable Go type explicitly.
- Use `overrides` only for project-wide type conventions that are worth standardizing.
- Avoid leaking `sql.NullString` or similar database boundary types into higher-level application code unless that is intentional.

Example SQLite override:

```yaml
version: "2"
sql:
  - engine: "sqlite"
    schema: "database.sql"
    queries: "queries"
    gen:
      go:
        package: "sqlcgen"
        out: "../db-scheduler/generated/sqlcgen"
        overrides:
          - db_type: "text"
            nullable: true
            go_type:
              type: "string"
              pointer: true
```

Use overrides carefully. A broad `text` override affects every nullable SQLite text column in that sqlc package.

## Structs, Tags, and Naming

sqlc creates model structs from tables and result structs from custom select shapes.

Default naming behavior:

- Table structs are often singularized, for example `agent_jobs` becomes `AgentJob`.
- Column names are converted to exported Go field names.
- Common initialisms can be controlled with `initialisms`.
- JSON tag style can be controlled with `json_tags_case_style`.

Useful config:

```yaml
gen:
  go:
    package: "sqlcgen"
    out: "../db-scheduler/generated/sqlcgen"
    emit_json_tags: true
    emit_db_tags: true
    json_tags_case_style: "snake"
    initialisms:
      - "id"
      - "url"
      - "api"
```

Use `rename` when a generated field name is awkward:

```yaml
gen:
  go:
    package: "sqlcgen"
    out: "../db-scheduler/generated/sqlcgen"
    rename:
      ecs_task_arn: "ECSTaskARN"
      pull_request_url: "PullRequestURL"
```

Use column aliases when only a query-specific result needs a better name:

```sql
-- name: CountJobsByStatus :many
SELECT status, count(*) AS job_count
FROM agent_job_info
GROUP BY status;
```

## Transactions

Generated packages include a `WithTx` helper. Use it to bind a `Queries` value to a transaction for the duration of a workflow.

```go
func CompleteJob(ctx context.Context, db *sql.DB, q *sqlcgen.Queries, id int64) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	qtx := q.WithTx(tx)

	if _, err := qtx.MarkAgentJobComplete(ctx, id); err != nil {
		return err
	}

	return tx.Commit()
}
```

Best practices:

- Keep transaction scopes short.
- Defer rollback immediately after begin. A rollback after commit is harmless for `database/sql`.
- Pass the transaction-bound `qtx` through the operation instead of mixing transactional and non-transactional query handles.
- For multi-step scheduler state changes, prefer one transaction so status transitions cannot be partially written.

## Generated Interfaces and Testing

Set `emit_interface: true` to generate a `Querier` interface. This helps when services should depend on an interface rather than `*Queries`.

```yaml
gen:
  go:
    package: "sqlcgen"
    out: "../db-scheduler/generated/sqlcgen"
    emit_interface: true
```

Usage:

```go
type JobService struct {
	q sqlcgen.Querier
}

func NewJobService(q sqlcgen.Querier) *JobService {
	return &JobService{q: q}
}
```

Testing options:

- Use the generated interface and a hand-written fake for small unit tests.
- Use a temporary SQLite database file, or in-memory SQLite, for integration tests.
- Run `sqlc generate` and `go test ./...` in CI so generated APIs stay current.

For in-memory SQLite tests, be aware that each independent connection can get a separate in-memory database. If using `database/sql`, configure connection pooling deliberately, for example by limiting open connections, or use a shared-cache DSN supported by the selected driver.

## Commands and CI

Useful local commands:

```bash
sqlc generate
sqlc diff
sqlc vet
go test ./...
```

What they do:

- `generate`: parse schema and query files, then write generated code.
- `diff`: compare generated output with files on disk. This catches stale generated code or manual edits to generated files.
- `vet`: run lint rules against queries.
- `go test ./...`: compile and test the Go project using the generated package.

A practical CI sequence for this project:

```bash
cd database-and-scheduler/sqlc
sqlc generate
sqlc diff

cd ..
go test ./...
```

## SQLite Runtime Notes

sqlc generates Go code, but SQLite runtime behavior still depends on how the application opens the database.

Important points:

- Enable foreign keys on actual SQLite connections. `PRAGMA foreign_keys = ON` in a schema file is not enough for all runtime connection pool scenarios.
- Keep write transactions short; SQLite permits many readers but has more constrained write concurrency than a server database.
- Consider setting busy timeout behavior in the chosen driver's DSN or connection setup.
- Use one consistent timestamp format if storing timestamps as `TEXT`.
- Treat generated code as build output; do not hand-edit it.

## Suggested Adjustments for This Repo

The current config should be changed to use a valid Go package name and the existing generated-code directory:

```yaml
version: "2"
sql:
  - engine: "sqlite"
    schema: "database.sql"
    queries: "queries"
    gen:
      go:
        package: "sqlcgen"
        out: "../db-scheduler/generated/sqlcgen"
        emit_json_tags: true
        emit_empty_slices: true
        query_parameter_limit: 0
```

Then add query files under `database-and-scheduler/sqlc/queries`, for example:

```sql
-- name: GetAgentJob :one
SELECT id,
       agent_name,
       status,
       spawn_sqs_message_id,
       agent_report,
       affected_repositories,
       pull_request_url,
       failure_reason,
       ecs_task_arn,
       ecs_last_status,
       ecs_stopped_reason,
       created_at,
       started_at,
       completed_at,
       updated_at
FROM agent_job_info
WHERE id = ?;

-- name: ListAgentJobsByStatus :many
SELECT id,
       agent_name,
       status,
       spawn_sqs_message_id,
       agent_report,
       affected_repositories,
       pull_request_url,
       failure_reason,
       ecs_task_arn,
       ecs_last_status,
       ecs_stopped_reason,
       created_at,
       started_at,
       completed_at,
       updated_at
FROM agent_job_info
WHERE status = ?
ORDER BY created_at DESC;

-- name: CreateAgentJob :one
INSERT INTO agent_job_info (
    agent_name,
    status,
    spawn_sqs_message_id
) VALUES (
    ?, ?, ?
)
RETURNING id,
          agent_name,
          status,
          spawn_sqs_message_id,
          agent_report,
          affected_repositories,
          pull_request_url,
          failure_reason,
          ecs_task_arn,
          ecs_last_status,
          ecs_stopped_reason,
          created_at,
          started_at,
          completed_at,
          updated_at;
```

Before generating, fix the schema foreign key reference if needed. `agent_job_info.spawn_sqs_message_id` currently references `sqs_messages_tickets_cloudwatch(id)`, but the visible schema defines `sqs_tickets(id)`. sqlc and SQLite schema validation may fail if the referenced table does not exist in the schema sqlc sees.

## Mental Model

Think of sqlc as a compiler boundary:

- SQLite schema and query files are source code.
- `sqlc.yaml` is compiler configuration.
- Generated Go is build output.
- Go services call generated methods instead of building SQL strings.
- Migrations or schema setup remain responsible for changing real SQLite databases.

Good sqlc usage keeps SQLite behavior explicit in SQL while giving the Go compiler enough generated types to catch mistakes early.
