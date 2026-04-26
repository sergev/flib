package main

import (
	"database/sql"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
)

type listRow struct {
	Lang, Author, Name, RelPath string
}

func sortField(s string) string {
	if strings.TrimSpace(s) == "" {
		return "\uffff\uffff"
	}
	return strings.ToLower(strings.TrimSpace(s))
}

func lessListRow(a, b listRow) bool {
	sa, sb := sortField(a.Lang), sortField(b.Lang)
	if sa != sb {
		return sa < sb
	}
	sa, sb = sortField(a.Author), sortField(b.Author)
	if sa != sb {
		return sa < sb
	}
	return sortField(a.Name) < sortField(b.Name)
}

func emitGrouped(w io.Writer, headerKind string, groups map[string][]listRow) error {
	keys := make([]string, 0, len(groups))
	for k := range groups {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		rows := groups[k]
		sort.SliceStable(rows, func(i, j int) bool { return lessListRow(rows[i], rows[j]) })
		if _, err := fmt.Fprintf(w, "=== %s: %s\n", headerKind, k); err != nil {
			return err
		}
		for _, r := range rows {
			if _, err := fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", r.Lang, r.Author, r.Name, r.RelPath); err != nil {
				return err
			}
		}
	}
	return nil
}

func cmdListBy(db *sql.DB, mode string) error {
	switch mode {
	case "author":
		return cmdListByAuthor(db, os.Stdout)
	case "genre":
		return cmdListByGenre(db, os.Stdout)
	case "language":
		return cmdListByLanguage(db, os.Stdout)
	default:
		return fmt.Errorf("unknown list mode: %s", mode)
	}
}

func cmdListByAuthor(db *sql.DB, w io.Writer) error {
	rows, err := db.Query(`
		SELECT
			COALESCE(NULLIF(TRIM(b.language), ''), ''),
			a.name1, a.name2, a.name3,
			COALESCE(NULLIF(TRIM(b.name), ''), ''),
			COALESCE(b.archive, ''),
			COALESCE(b.file, ''),
			COALESCE(b.format, '')
		FROM book b
		LEFT JOIN book_author ba ON ba.id_book = b.id AND ba.id_lib = b.id_lib
		LEFT JOIN author a ON a.id = ba.id_author AND a.id_lib = ba.id_lib
	`)
	if err != nil {
		return err
	}
	defer rows.Close()

	groups := make(map[string][]listRow)
	for rows.Next() {
		var lang, title, archive, file, format string
		var n1, n2, n3 sql.NullString
		if err := rows.Scan(&lang, &n1, &n2, &n3, &title, &archive, &file, &format); err != nil {
			return err
		}
		auth := formatAuthorName(n1, n2, n3)
		gk := auth
		if strings.TrimSpace(gk) == "" {
			gk = "(unknown)"
		}
		groups[gk] = append(groups[gk], listRow{
			Lang:    lang,
			Author:  auth,
			Name:    title,
			RelPath: bookRelPath(archive, file, format),
		})
	}
	if err := rows.Err(); err != nil {
		return err
	}
	return emitGrouped(w, "Author", groups)
}

func cmdListByGenre(db *sql.DB, w io.Writer) error {
	genreTable, err := resolveGenreTableName(db)
	if err != nil {
		return err
	}
	genreCol, err := resolveGenreIDColumn(db, genreTable)
	if err != nil {
		return err
	}
	rows, err := db.Query(fmt.Sprintf(`
		SELECT
			COALESCE(NULLIF(TRIM(b.language), ''), ''),
			b.id, b.id_lib, b.first_author_id,
			COALESCE(NULLIF(TRIM(b.name), ''), ''),
			COALESCE(b.archive, ''),
			COALESCE(b.file, ''),
			COALESCE(b.format, ''),
			bg.%s
		FROM book b
		JOIN %s bg ON bg.id_book = b.id AND bg.id_lib = b.id_lib
	`, genreCol, genreTable))
	if err != nil {
		return err
	}
	defer rows.Close()

	groups := make(map[string][]listRow)
	for rows.Next() {
		var lang, title, archive, file, format string
		var idBook, idLib int64
		var firstAuthorID sql.NullInt64
		var idGenre int64
		if err := rows.Scan(&lang, &idBook, &idLib, &firstAuthorID, &title, &archive, &file, &format, &idGenre); err != nil {
			return err
		}
		auth, err := authorForBook(db, idBook, idLib, firstAuthorID)
		if err != nil {
			return err
		}
		gk := genreDisplayName(int(idGenre))
		groups[gk] = append(groups[gk], listRow{
			Lang:    lang,
			Author:  auth,
			Name:    title,
			RelPath: bookRelPath(archive, file, format),
		})
	}
	if err := rows.Err(); err != nil {
		return err
	}
	return emitGrouped(w, "Genre", groups)
}

func resolveGenreTableName(db *sql.DB) (string, error) {
	exists, err := sqliteTableExists(db, "book_genre")
	if err != nil {
		return "", err
	}
	if exists {
		return "book_genre", nil
	}
	exists, err = sqliteTableExists(db, "book_janre")
	if err != nil {
		return "", err
	}
	if exists {
		return "book_janre", nil
	}
	return "", fmt.Errorf("database does not contain genre links table (expected book_genre)")
}

func sqliteTableExists(db *sql.DB, table string) (bool, error) {
	var cnt int
	err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name=?`, table).Scan(&cnt)
	if err != nil {
		return false, err
	}
	return cnt > 0, nil
}

func resolveGenreIDColumn(db *sql.DB, table string) (string, error) {
	hasIDGenre, err := sqliteColumnExists(db, table, "id_genre")
	if err != nil {
		return "", err
	}
	if hasIDGenre {
		return "id_genre", nil
	}
	hasIDJanre, err := sqliteColumnExists(db, table, "id_janre")
	if err != nil {
		return "", err
	}
	if hasIDJanre {
		return "id_janre", nil
	}
	return "", fmt.Errorf("table %s does not contain id_genre/id_janre column", table)
}

func sqliteColumnExists(db *sql.DB, table, column string) (bool, error) {
	q := fmt.Sprintf(`PRAGMA table_info(%s)`, table)
	rows, err := db.Query(q)
	if err != nil {
		return false, err
	}
	defer rows.Close()
	for rows.Next() {
		var (
			cid       int
			name      string
			typ       sql.NullString
			notNull   int
			defaultV  sql.NullString
			primaryK  int
		)
		if err := rows.Scan(&cid, &name, &typ, &notNull, &defaultV, &primaryK); err != nil {
			return false, err
		}
		if name == column {
			return true, nil
		}
	}
	if err := rows.Err(); err != nil {
		return false, err
	}
	return false, nil
}

func cmdListByLanguage(db *sql.DB, w io.Writer) error {
	rows, err := db.Query(`
		SELECT
			COALESCE(NULLIF(TRIM(b.language), ''), ''),
			b.id, b.id_lib, b.first_author_id,
			COALESCE(NULLIF(TRIM(b.name), ''), ''),
			COALESCE(b.archive, ''),
			COALESCE(b.file, ''),
			COALESCE(b.format, '')
		FROM book b
	`)
	if err != nil {
		return err
	}
	defer rows.Close()

	groups := make(map[string][]listRow)
	for rows.Next() {
		var lang, title, archive, file, format string
		var idBook, idLib int64
		var firstAuthorID sql.NullInt64
		if err := rows.Scan(&lang, &idBook, &idLib, &firstAuthorID, &title, &archive, &file, &format); err != nil {
			return err
		}
		auth, err := authorForBook(db, idBook, idLib, firstAuthorID)
		if err != nil {
			return err
		}
		gk := lang
		if strings.TrimSpace(gk) == "" {
			gk = "(unknown)"
		}
		groups[gk] = append(groups[gk], listRow{
			Lang:    lang,
			Author:  auth,
			Name:    title,
			RelPath: bookRelPath(archive, file, format),
		})
	}
	if err := rows.Err(); err != nil {
		return err
	}
	return emitGrouped(w, "Language", groups)
}
