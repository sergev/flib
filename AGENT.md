# Agent Notes

## Project Overview

`flib` is a Go 1.22 command-line tool for reading a freeLib SQLite catalog in read-only mode. It searches and lists books from `freeLib.sqlite`; it does not modify the database. The module path is `github.com/freeLib/freeLib/flib`.

Primary user-facing commands:

- `flib show PATTERN [--max NUM]`: case-insensitive Go regexp search over `book.name`; emits a YAML array to stdout and progress to stderr. Default `--max` is `20`.
- `flib by author`: lists all books grouped by formatted author name.
- `flib by genre`: lists all books grouped by compiled-in genre display name.
- `flib by language`: lists all books grouped by language.
- `flib help`: prints usage.

The database path comes from `FLIB_DB`, or defaults to `~/Documents/freeLib.sqlite`.

## Important Files

- `main.go`: CLI dispatch, usage text, `openCatalog()`.
- `dbpath.go`: database path resolution and `~/` expansion for `FLIB_DB`.
- `sqlite.go`: opens SQLite through `modernc.org/sqlite`; uses `mode=ro` for normal CLI reads.
- `show.go`: `show` command implementation, YAML output model, progress output, author and genre lookup helpers.
- `list.go`: `by author`, `by genre`, `by language`, grouping and sorting, legacy genre table/column compatibility.
- `paths.go`: relative book path formatting as `archive:file.format` or `file.format`, omitting trailing dot when format is empty.
- `genre.go`: genre display fallback and `go:generate` directive.
- `genres_generated.go`: generated map from genre id to display name; do not edit by hand.
- `cmd/genregen/main.go`: generator for `genres_generated.go` from freeLib `src/genre.tsv`.
- `main_test.go`: tests using temporary SQLite databases; good examples for expected behavior.
- `Database.md`: detailed freeLib SQLite schema notes and migration/background context.
- `Makefile`: top-level build/test helpers.

## Build And Test

Prefer the top-level Makefile for common local tasks:

```bash
make        # go build
make test   # gotestsum --format dots -- ./...
make cover  # gotestsum -- -coverprofile=coverage.out ./...; go tool cover -func=coverage.out
make clean  # go clean and remove configured binary artifact
```

`make test` and `make cover` auto-install `gotestsum` with `go install gotest.tools/gotestsum@latest` if it is missing.

Standard Go commands still work from the repo root:

```bash
go test ./...
go build -o flib .
```

Run manually against a catalog:

```bash
FLIB_DB="/path/to/freeLib.sqlite" ./flib show "asimov|foundation" --max 50
FLIB_DB="/path/to/freeLib.sqlite" ./flib by genre
```

The current test suite passes with `go test ./...`.

Makefile review notes:

- `all` currently runs plain `go build`, which produces the default binary name for the package directory (`flib`).
- `PROG` is currently set to `floppy`, so `install`, `uninstall`, and `clean` may not line up with the binary produced by `all`.
- `install` writes to `$(DESTDIR)/usr/bin/${PROG}`, while `uninstall` removes `$(DESTDIR)/bin/${PROG}`. Do not document install/uninstall as reliable until this is reconciled.

## Data And Output Contracts

- All normal data output goes to stdout; errors and `show` progress go to stderr.
- `show` output is YAML using `gopkg.in/yaml.v3` and the compact `bookOut` struct. It intentionally omits `library` and `lib_path`.
- `show` scans all books ordered by `id_lib, id`, matches only against title, then enriches matched rows with one author and genre names.
- `show --max 0` is accepted by argument parsing but currently returns the first matched row because the limit check happens after appending; treat changes here carefully and add tests if semantics are changed.
- `by` output is plain text. Group headers look like `=== Author: NAME`, `=== Genre: NAME`, or `=== Language: LANG`; rows are tab-separated as `language author title relative_path`.
- Group rows sort by language, then author, then title, using case-insensitive trimmed values and pushing blank values last.
- Unknown author/language groups print `(unknown)`.
- Unknown genre ids print as `genre:<id>`.

## Database Assumptions

The code expects the freeLib schema documented in `Database.md`, especially:

- `book` rows include `id`, `name`, `star`, `language`, `id_lib`, `file`, `size`, `deleted`, `date`, `format`, `keys`, `id_inlib`, `archive`, and `first_author_id`.
- Authors live in `author` and are linked through `book_author`; `first_author_id` is preferred when valid, with fallback to the first linked author.
- Genre links usually live in `book_genre.id_genre`, but legacy compatibility also supports table `book_janre` and column `id_janre`.
- There is no genre table in the current schema; display names come from generated Go code based on `src/genre.tsv`.

When changing SQL, prefer tests that create minimal temporary schemas like `main_test.go` does. Keep the CLI read-only unless the feature explicitly changes that contract.

## Generated Genre Data

`genres_generated.go` is generated and contains non-ASCII genre names. Regenerate it instead of editing:

```bash
go generate ./...
```

The generator directive in `genre.go` uses:

```bash
go run ./cmd/genregen -in ../src/genre.tsv -out genres_generated.go
```

This assumes the larger freeLib repository layout where `src/genre.tsv` is one directory above `flib`. If that file is absent, generation will fail even though normal build/tests can still pass with the checked-in generated file.

## Coding Conventions

- Keep command behavior conservative and documented in `README.md` when user-facing semantics change.
- Add focused tests for CLI parsing, output contracts, schema compatibility, and path formatting.
- Use `database/sql` with placeholders for values. Existing dynamic SQL only interpolates internally selected table/column names from allowlisted schema checks.
- Preserve read-only catalog access for normal commands.
- Do not hand-edit `genres_generated.go`.
