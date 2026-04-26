# flib

`flib` is a command-line tool for searching and listing books stored in a freeLib SQLite catalog.

The tool reads the same database used by freeLib and prints results to stdout. Errors and progress messages are printed to stderr.

## Quick Start

Build:

```bash
make
```

Show help:

```bash
./flib help
```

Use a specific database:

```bash
FLIB_DB="/path/to/freeLib.sqlite" ./flib show "dune"
```

Extract all books into a file tree:

```bash
FLIB_DB="/path/to/freeLib.sqlite" \
FLIB_PATH="/Volumes/Seagate4Tb/Flibusta/Flibusta.Net" \
./flib extract --destdir ./exported
```

## Development

Common project tasks are available through the top-level `Makefile`:

```bash
make        # build the CLI
make test   # run tests with gotestsum
make cover  # run tests and print coverage summary
make clean  # remove build artifacts
```

`make test` and `make cover` install `gotestsum` automatically if it is not already available. You can also use standard Go commands directly:

```bash
go test ./...
go build -o flib .
```

## Database Location

`flib` opens the database in read-only mode.

- If `FLIB_DB` is set, that path is used.
- Otherwise default path is `~/Documents/freeLib.sqlite`.

If the file does not exist, `flib` exits with a clear error message.

## Commands

### 1) Show books by title pattern

```bash
flib show PATTERN [--max NUM]
```

- `PATTERN` is a Go regular expression.
- Matching is case-insensitive.
- Search is done against book title (`book.name`).
- `--max` limits number of matched books in output.
- Default `--max` is `20`.

Output format:

- YAML array, one object per book.
- Fields are compact (`omitempty` behavior).
- Includes `id`, `title`, `author`, `genres`, `language`, `format`, `file`, `archive`, `size`, `date`, `deleted`, `star`, `keys`, `id_inlib`.
- Does **not** include `library` or `lib_path`.

Progress:

- During `show`, progress is printed to stderr:
  - `flib: progress <N>%, matched <M>`
- Percentage is an integer and updates only when it changes.

Example:

```bash
./flib show "asimov|foundation" --max 50 > books.yaml
```

### 2) List all books grouped by author

```bash
flib by author
```

Output:

- Plain text.
- Group headers:
  - `=== Author: <name>`
- One book per line in each group.
- Line columns are tab-separated:
  - `<language>\t<author>\t<name>\t<relative_path>`

Grouping/sorting:

- Group key: normalized author name built from `name1 name2 name3`.
- Books without linked author are grouped under `(unknown)`.
- Inside each group, sort order is:
  1) language
  2) author
  3) name

### 3) List all books grouped by genre

```bash
flib by genre
```

Output:

- Same line format as `by author`.
- Group headers:
  - `=== Genre: <genre_name>`

Genre resolution:

- Genre names come from compiled-in genre data generated from `src/genre.tsv`.
- Unknown genre ids are shown as `genre:<id>`.
- Legacy schema compatibility:
  - table `book_genre` or `book_janre`
  - column `id_genre` or `id_janre`

### 4) List all books grouped by language

```bash
flib by language
```

Output:

- Same line format as other `by` modes.
- Group headers:
  - `=== Language: <lang>`
- Empty language values are grouped as `(unknown)`.

### 5) Extract all books to files

```bash
flib extract [--destdir DIR]
```

- Uses `FLIB_PATH` to locate the Flibusta library root containing zip archives.
- Reads the catalog from `FLIB_DB` (or default DB path) and extracts each book from `archive` using member name `file.format`.
- Output tree format: `language/author/book.format`.
- If `--destdir` is omitted, files are written under the current directory.
- Existing output files are overwritten.
- If multiple books map to the same output path, later books receive `-<id>` suffixes (for example: `book-123.fb2`).
- Each created relative path is printed to stderr for progress.

## Relative Path Format

The last column in `by` output is the relative path:

- If archive is present: `archive:file.format`
- If archive is empty: `file.format`
- If format is empty, trailing dot is omitted.

## Exit Codes

- `0` on success
- non-zero on error (invalid args, regex errors, DB errors, etc.)

## Notes

- `flib` is read-only and does not modify the database.
- `show` currently filters by title regex only.
- `by` commands print all rows available in the database for that grouping.
- `extract` reads zip archives from `FLIB_PATH` and writes regular files to destination directories.

## Project Link

- freeLib: [https://github.com/petrovvlad/freeLib](https://github.com/petrovvlad/freeLib)
