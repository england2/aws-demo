# GoAWK Notes

Research date: 2026-05-16

Primary sources:

- Repository and README: <https://github.com/benhoyt/goawk>
- Official Go package docs, module root: <https://pkg.go.dev/github.com/benhoyt/goawk>
- Official Go API docs, interpreter package: <https://pkg.go.dev/github.com/benhoyt/goawk/interp>
- Official Go API docs, parser package: <https://pkg.go.dev/github.com/benhoyt/goawk/parser>
- CSV/TSV docs: <https://github.com/benhoyt/goawk/blob/master/docs/csv.md>

## What GoAWK Is

GoAWK is a POSIX-compatible AWK implementation written in Go. It can be used as a command-line `goawk` binary or embedded as a Go library.

The project adds several features beyond basic POSIX AWK, most notably CSV and TSV input/output modes, named CSV fields through `@"field"` syntax, negative field indexes, code coverage support, embeddability in Go programs, and custom Go functions callable from AWK.

The upstream README says the Go API is intended to avoid breaking changes during v1.x.y releases. Current pkg.go.dev docs show v1.31.0 as the published module version on December 23, 2025.

## Install and CLI Use

Install the command with:

```bash
go install github.com/benhoyt/goawk@latest
```

Basic command examples:

```bash
goawk 'BEGIN { print "foo", 42 }'
echo 1 2 3 | goawk '{ print $1 + $3 }'
```

CSV mode is enabled with CLI flags such as `-i csv`; `-H` treats the first row as a header, enabling named-field access:

```bash
goawk -i csv -H '{ total += @"amount" } END { print total }'
```

## Go API Shape

Use `interp.Exec` for the simplest one-shot execution:

```go
err := interp.Exec("$0 { print $1 }", " ", inputReader, outputWriter)
```

Use `parser.ParseProgram` plus `interp.ExecProgram` when the caller needs configuration:

```go
program, err := parser.ParseProgram([]byte(`{ print NR, tolower($0) }`), nil)
if err != nil {
	return err
}

_, err = interp.ExecProgram(program, &interp.Config{
	Stdin: strings.NewReader(input),
	Vars:  []string{"OFS", ":"},
})
```

Use `interp.New(program)` when the same parsed program should run repeatedly against different inputs. The returned interpreter supports `Execute`, `ExecuteContext`, `ResetVars`, `ResetRand`, and `Array`.

## Important API Types

`interp.Config` controls execution. Useful fields include:

- `Stdin`, `Output`, and `Error` for IO.
- `Argv0`, `Args`, and `NoArgVars` for AWK argument behavior.
- `Vars` for initial AWK variables such as `FS` or `OFS`.
- `Funcs` for Go functions callable from AWK. The same function map must also be passed to `parser.ParserConfig`.
- `NoExec`, `NoFileWrites`, and `NoFileReads` for limiting unsafe behavior when running untrusted scripts.
- `ShellCommand` for the shell used by `system()` and pipes.
- `Environ` to control the AWK `ENVIRON` array. Use an empty non-nil slice when scripts do not need environment variables.
- `InputMode`, `OutputMode`, `CSVInput`, and `CSVOutput` for CSV/TSV behavior.
- `Chars` to count Unicode characters instead of bytes for string functions.
- `NewlineOutput` for output newline behavior.

`parser.ParserConfig` controls parsing. The main practical field is `Funcs`, which declares the Go function names available to the AWK parser.

`parser.Program` is the parsed and compiled representation of an AWK program. It can be executed by `interp.ExecProgram` or used to create an `interp.Interpreter`.

## CSV and TSV Notes

GoAWK has first-class CSV and TSV support through `interp.CSVMode` and `interp.TSVMode`.

For input:

- Set `interp.Config.InputMode` to `interp.CSVMode` or `interp.TSVMode`.
- Use `interp.Config.CSVInput` for separator, comment character, and header-row behavior.
- Header mode enables `@"field"` named-field syntax and the `FIELDS` special array.

For output:

- Set `interp.Config.OutputMode` to `interp.CSVMode` or `interp.TSVMode`.
- Use `interp.Config.CSVOutput` for output separator customization.

CSV/TSV modes can also be controlled from AWK by assigning `INPUTMODE` or `OUTPUTMODE` in `Vars` or a `BEGIN` block.

## Safety Notes

If this project ever runs AWK sourced from user input, configure `interp.Config` defensively:

```go
config := &interp.Config{
	NoExec:       true,
	NoFileReads:  true,
	NoFileWrites: true,
	Environ:      []string{},
}
```

This disables shell execution, file reads, file writes, and avoids exposing the process environment unless intentionally supplied.

## Which Docs To Use

For library embedding, start with:

1. `interp` docs: <https://pkg.go.dev/github.com/benhoyt/goawk/interp>
2. `parser` docs: <https://pkg.go.dev/github.com/benhoyt/goawk/parser>
3. CSV docs: <https://github.com/benhoyt/goawk/blob/master/docs/csv.md>

The module root package is the command package, so most embedding work should import `github.com/benhoyt/goawk/interp` and `github.com/benhoyt/goawk/parser`, not the module root.
