package main

import (
	"database/sql"
	"fmt"
	"io"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

const defaultShowMax = 20

// bookOut is YAML output for one book (compact, omitempty).
type bookOut struct {
	ID       int64    `yaml:"id"`
	Title    string   `yaml:"title,omitempty"`
	Author   string   `yaml:"author,omitempty"`
	Genres   []string `yaml:"genres,omitempty"`
	Language string   `yaml:"language,omitempty"`
	Format   string   `yaml:"format,omitempty"`
	File     string   `yaml:"file,omitempty"`
	Archive  string   `yaml:"archive,omitempty"`
	Size     int64    `yaml:"size,omitempty"`
	Date     string   `yaml:"date,omitempty"`
	Deleted  *bool    `yaml:"deleted,omitempty"`
	Star     int64    `yaml:"star,omitempty"`
	Keys     string   `yaml:"keys,omitempty"`
	IDInLib  int64    `yaml:"id_inlib,omitempty"`
}

func parseShowArgs(args []string) (pattern string, max int, err error) {
	max = defaultShowMax
	if len(args) == 0 {
		return "", 0, fmt.Errorf("show requires PATTERN")
	}
	pattern = args[0]
	rest := args[1:]
	for i := 0; i < len(rest); i++ {
		switch rest[i] {
		case "--max":
			if i+1 >= len(rest) {
				return "", 0, fmt.Errorf("--max requires a number")
			}
			n, err := strconv.Atoi(rest[i+1])
			if err != nil || n < 0 {
				return "", 0, fmt.Errorf("invalid --max value: %q", rest[i+1])
			}
			max = n
			i++
		default:
			return "", 0, fmt.Errorf("unknown argument: %s", rest[i])
		}
	}
	return pattern, max, nil
}

func authorForBook(db *sql.DB, idBook, idLib int64, firstAuthorID sql.NullInt64) (string, error) {
	if firstAuthorID.Valid {
		var n1, n2, n3 sql.NullString
		err := db.QueryRow(
			`SELECT name1, name2, name3 FROM author WHERE id = ? AND id_lib = ?`,
			firstAuthorID.Int64, idLib,
		).Scan(&n1, &n2, &n3)
		if err == nil {
			return formatAuthorName(n1, n2, n3), nil
		}
		if err != sql.ErrNoRows {
			return "", err
		}
	}
	var n1, n2, n3 sql.NullString
	err := db.QueryRow(`
		SELECT a.name1, a.name2, a.name3
		FROM book_author ba
		JOIN author a ON a.id = ba.id_author AND a.id_lib = ba.id_lib
		WHERE ba.id_book = ? AND ba.id_lib = ?
		LIMIT 1
	`, idBook, idLib).Scan(&n1, &n2, &n3)
	if err == sql.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return formatAuthorName(n1, n2, n3), nil
}

func formatAuthorName(n1, n2, n3 sql.NullString) string {
	parts := []string{}
	if n1.Valid && strings.TrimSpace(n1.String) != "" {
		parts = append(parts, strings.TrimSpace(n1.String))
	}
	if n2.Valid && strings.TrimSpace(n2.String) != "" {
		parts = append(parts, strings.TrimSpace(n2.String))
	}
	if n3.Valid && strings.TrimSpace(n3.String) != "" {
		parts = append(parts, strings.TrimSpace(n3.String))
	}
	return strings.Join(parts, " ")
}

// genresForBook returns display names for genre ids linked to the book.
// If the database has no genre link table or an unrecognized column layout, it returns nil, nil.
func genresForBook(db *sql.DB, idBook, idLib int64) ([]string, error) {
	tbl, err := resolveGenreTableName(db)
	if err != nil {
		return []string{}, nil
	}
	col, err := resolveGenreIDColumn(db, tbl)
	if err != nil {
		return []string{}, nil
	}
	q := fmt.Sprintf(`SELECT DISTINCT %s AS gid FROM %s WHERE id_book = ? AND id_lib = ? ORDER BY gid`, col, tbl)
	rows, err := db.Query(q, idBook, idLib)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var names []string
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		names = append(names, genreDisplayName(int(id)))
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	sort.Strings(names)
	return names, nil
}

func cmdShow(db *sql.DB, args []string) error {
	return cmdShowWithProgress(os.Stdout, os.Stderr, db, args)
}

func cmdShowTo(w io.Writer, db *sql.DB, args []string) error {
	return cmdShowWithProgress(w, io.Discard, db, args)
}

func cmdShowWithProgress(w io.Writer, progress io.Writer, db *sql.DB, args []string) error {
	pattern, max, err := parseShowArgs(args)
	if err != nil {
		return err
	}
	re, err := regexp.Compile("(?i)" + pattern)
	if err != nil {
		return fmt.Errorf("invalid regular expression: %w", err)
	}

	rows, err := db.Query(`
		SELECT b.id, b.name, b.star, b.language, b.file, b.size, b.deleted, b.date, b.format, b.keys, b.id_inlib, b.archive, b.first_author_id, b.id_lib
		FROM book b
		ORDER BY b.id_lib ASC, b.id ASC
	`)
	if err != nil {
		return err
	}
	defer rows.Close()

	totalRows, err := countBooks(db)
	if err != nil {
		return err
	}

	var out []bookOut
	scanned := 0
	matched := 0
	lastPercent := -1
	for rows.Next() {
		scanned++
		p := percentInt(scanned, totalRows)
		if p != lastPercent {
			fmt.Fprintf(progress, "flib: progress %d%%, matched %d\r", p, matched)
			lastPercent = p
		}
		var (
			id, star, size, idInLib, idLib int64
			name, lang, file, date, format, keys, archive sql.NullString
			deleted       sql.NullBool
			firstAuthorID sql.NullInt64
		)
		if err := rows.Scan(
			&id, &name, &star, &lang, &file, &size, &deleted, &date, &format, &keys, &idInLib, &archive, &firstAuthorID, &idLib,
		); err != nil {
			return err
		}
		title := ""
		if name.Valid {
			title = name.String
		}
		if !re.MatchString(title) {
			continue
		}
		matched++

		author, err := authorForBook(db, id, idLib, firstAuthorID)
		if err != nil {
			return err
		}
		genres, err := genresForBook(db, id, idLib)
		if err != nil {
			return err
		}

		b := bookOut{
			ID:       id,
			Title:    title,
			Author:   author,
			Genres:   genres,
			Language: nullString(lang),
			Format:   nullString(format),
			File:     nullString(file),
			Archive:  nullString(archive),
			Size:     size,
			Date:     nullString(date),
			Star:     star,
			Keys:     nullString(keys),
			IDInLib:  idInLib,
		}
		if deleted.Valid {
			d := deleted.Bool
			b.Deleted = &d
		}

		out = append(out, b)
		if len(out) >= max {
			break
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	finalPercent := percentInt(scanned, totalRows)
	if finalPercent != lastPercent {
		fmt.Fprintf(progress, "flib: progress %d%%, matched %d\r", finalPercent, matched)
	}
	fmt.Fprintf(progress, "flib: progress %d%%, matched %d\n", finalPercent, matched)

	enc := yaml.NewEncoder(w)
	enc.SetIndent(2)
	if err := enc.Encode(out); err != nil {
		return err
	}
	return enc.Close()
}

func nullString(ns sql.NullString) string {
	if ns.Valid {
		return ns.String
	}
	return ""
}

func countBooks(db *sql.DB) (int, error) {
	var total int
	if err := db.QueryRow(`SELECT COUNT(*) FROM book`).Scan(&total); err != nil {
		return 0, err
	}
	return total, nil
}

func percentInt(current, total int) int {
	if total <= 0 {
		return 100
	}
	p := (current * 100) / total
	if p > 100 {
		return 100
	}
	if p < 0 {
		return 0
	}
	return p
}
