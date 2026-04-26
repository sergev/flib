package main

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
)

func usage() {
	fmt.Fprint(os.Stderr, `flib — search the freeLib SQLite catalog

Usage:
  flib show PATTERN [--max NUM]   Search books by title (regular expression). Default --max is 20.
  flib extract [--destdir DIR]    Extract books into language/author/book.format tree.
  flib by author                   List all books grouped by author (tab-separated columns).
  flib by genre                    List all books grouped by genre.
  flib by language                 List all books grouped by language.
  flib help                        Show this message

Environment:
  FLIB_DB   Path to freeLib.sqlite (default: ~/Documents/freeLib.sqlite)
  FLIB_PATH Path to Flibusta library root with zip archives (required for extract)
`)
}

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "flib: %v\n", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		usage()
		return errors.New("no command given")
	}
	switch args[0] {
	case "help", "-h", "--help":
		usage()
		return nil
	case "show":
		if len(args) < 2 {
			usage()
			return errors.New("show requires PATTERN")
		}
		db, err := openCatalog()
		if err != nil {
			return err
		}
		defer db.Close()
		return cmdShow(db, args[1:])
	case "extract":
		db, err := openCatalog()
		if err != nil {
			return err
		}
		defer db.Close()
		return cmdExtract(db, args[1:])
	case "by":
		if len(args) < 2 {
			usage()
			return errors.New("by requires author, genre, or language")
		}
		sub := args[1]
		if sub != "author" && sub != "genre" && sub != "language" {
			usage()
			return fmt.Errorf("by: unknown subcommand %q (want author, genre, or language)", sub)
		}
		db, err := openCatalog()
		if err != nil {
			return err
		}
		defer db.Close()
		return cmdListBy(db, sub)
	default:
		usage()
		return fmt.Errorf("unknown command: %s", args[0])
	}
}

func openCatalog() (*sql.DB, error) {
	path, err := resolveDBPath()
	if err != nil {
		return nil, fmt.Errorf("database path: %w", err)
	}
	db, err := openSQLite(path, true)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("database file not found: %s (set FLIB_DB or place catalog at ~/Documents/freeLib.sqlite)", path)
		}
		return nil, fmt.Errorf("open database: %w", err)
	}
	return db, nil
}
