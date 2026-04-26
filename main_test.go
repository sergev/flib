package main

import (
	"archive/zip"
	"bytes"
	"database/sql"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func TestResolveDBPath_FLIB_DB(t *testing.T) {
	t.Setenv("FLIB_DB", "/tmp/custom.sqlite")
	p, err := resolveDBPath()
	if err != nil {
		t.Fatal(err)
	}
	if p != "/tmp/custom.sqlite" {
		t.Fatalf("got %q", p)
	}
}

func TestResolveDBPath_DefaultUsesHome(t *testing.T) {
	t.Setenv("FLIB_DB", "")
	home := t.TempDir()
	t.Setenv("HOME", home)
	p, err := resolveDBPath()
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(home, "Documents", "freeLib.sqlite")
	if p != want {
		t.Fatalf("got %q want %q", p, want)
	}
}

func TestRunHelp(t *testing.T) {
	if err := run([]string{"help"}); err != nil {
		t.Fatal(err)
	}
}

func TestRunShowMissingDB(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "missing.sqlite")
	t.Setenv("FLIB_DB", dbPath)
	err := run([]string{"show", ".*"})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Fatalf("got %v", err)
	}
}

func TestParseShowArgs(t *testing.T) {
	pat, max, err := parseShowArgs([]string{"foo", "--max", "3"})
	if err != nil || pat != "foo" || max != 3 {
		t.Fatalf("%q %d %v", pat, max, err)
	}
	_, _, err = parseShowArgs([]string{"x", "--max"})
	if err == nil {
		t.Fatal("expected err")
	}
	_, _, err = parseShowArgs([]string{"x", "--max", "nope"})
	if err == nil {
		t.Fatal("expected err")
	}
	_, _, err = parseShowArgs([]string{"x", "--unknown"})
	if err == nil {
		t.Fatal("expected err")
	}
}

func TestCmdShowIntegration(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.sqlite")
	db, err := sql.Open("sqlite", "file:"+dbPath+"?_pragma=foreign_keys(1)")
	if err != nil {
		t.Fatal(err)
	}
	schema := `
CREATE TABLE lib (id INTEGER PRIMARY KEY, name TEXT, path TEXT);
CREATE TABLE author (id INTEGER, name1 TEXT, name2 TEXT, name3 TEXT, id_lib INTEGER, PRIMARY KEY(id));
CREATE TABLE book (
  id INTEGER PRIMARY KEY,
  name TEXT,
  star INTEGER,
  language TEXT,
  id_lib INTEGER,
  file TEXT,
  size INTEGER,
  deleted BOOL,
  date DATETIME,
  format TEXT,
  keys TEXT,
  id_inlib INTEGER,
  archive TEXT,
  first_author_id INTEGER,
  FOREIGN KEY(id_lib) REFERENCES lib(id)
);
CREATE TABLE book_author (id_book INTEGER, id_author INTEGER, id_lib INTEGER,
  PRIMARY KEY(id_book, id_author, id_lib),
  FOREIGN KEY(id_lib) REFERENCES lib(id),
  FOREIGN KEY(id_author) REFERENCES author(id),
  FOREIGN KEY(id_book) REFERENCES book(id)
);
CREATE TABLE book_genre (id_book INTEGER, id_genre INTEGER, id_lib INTEGER,
  PRIMARY KEY(id_book, id_genre, id_lib),
  FOREIGN KEY(id_lib) REFERENCES lib(id),
  FOREIGN KEY(id_book) REFERENCES book(id)
);
`
	if _, err := db.Exec(schema); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO lib(id, name, path) VALUES (1, 'L1', '/lib');`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO author(id, name1, name2, name3, id_lib) VALUES (10, 'Doe', 'Jane', '', 1);`); err != nil {
		t.Fatal(err)
	}
	for i := 1; i <= 25; i++ {
		title := "Other"
		if i <= 3 {
			title = "Alpha match"
		}
		var firstAuthor any
		if i == 1 {
			firstAuthor = 10
		}
		_, err := db.Exec(`INSERT INTO book(id, name, star, language, id_lib, file, size, deleted, date, format, keys, id_inlib, archive, first_author_id)
			VALUES (?, ?, 0, 'en', 1, 'f.fb2', 100, 0, '2020-01-01', 'fb2', '', ?, '', ?)`,
			i, title, i, firstAuthor)
		if err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.Exec(`UPDATE book SET first_author_id = NULL WHERE id = 2`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO book_author(id_book, id_author, id_lib) VALUES (2, 10, 1);`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO book_genre(id_book, id_genre, id_lib) VALUES (1, 100, 1), (1, 101, 1);`); err != nil {
		t.Fatal(err)
	}
	_ = db.Close()

	dbRO, err := openSQLite(dbPath, true)
	if err != nil {
		t.Fatal(err)
	}
	defer dbRO.Close()

	var buf bytes.Buffer
	if err := cmdShowTo(&buf, dbRO, []string{`Alpha`, `--max`, `2`}); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if strings.Count(out, "title:") != 2 {
		t.Fatalf("want 2 matches, got:\n%s", out)
	}
	if !strings.Contains(out, "Jane") {
		t.Fatalf("expected author: %s", out)
	}
	if strings.Contains(out, "library:") || strings.Contains(out, "lib_path:") {
		t.Fatalf("unexpected library fields: %s", out)
	}
	if !strings.Contains(out, "genres:") || !strings.Contains(out, "Научная фантастика") || !strings.Contains(out, "Фэнтези") {
		t.Fatalf("expected genres on book 1: %s", out)
	}

	buf.Reset()
	if err := cmdShowTo(&buf, dbRO, []string{`Alpha`}); err != nil {
		t.Fatal(err)
	}
	out2 := buf.String()
	if strings.Count(out2, "title:") != 3 {
		t.Fatalf("want 3 matches: %s", out2)
	}

	buf.Reset()
	if err := cmdShowTo(&buf, dbRO, []string{`alpha`}); err != nil {
		t.Fatal(err)
	}
	out3 := buf.String()
	if strings.Count(out3, "title:") != 3 {
		t.Fatalf("want case-insensitive 3 matches: %s", out3)
	}
	if strings.Contains(out3, "library:") || strings.Contains(out3, "lib_path:") {
		t.Fatalf("unexpected library fields: %s", out3)
	}
}

func TestCmdShowNoGenreTable(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "show_nogenre.sqlite")
	db, err := sql.Open("sqlite", "file:"+dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`
CREATE TABLE lib (id INTEGER PRIMARY KEY, name TEXT, path TEXT);
CREATE TABLE book (
  id INTEGER PRIMARY KEY,
  name TEXT,
  star INTEGER,
  language TEXT,
  id_lib INTEGER,
  file TEXT,
  size INTEGER,
  deleted BOOL,
  date DATETIME,
  format TEXT,
  keys TEXT,
  id_inlib INTEGER,
  archive TEXT,
  first_author_id INTEGER
);
CREATE TABLE author (id INTEGER, name1 TEXT, name2 TEXT, name3 TEXT, id_lib INTEGER, PRIMARY KEY(id));
CREATE TABLE book_author (id_book INTEGER, id_author INTEGER, id_lib INTEGER, PRIMARY KEY(id_book, id_author, id_lib));
INSERT INTO lib(id, name, path) VALUES (1, 'L1', '/lib');
INSERT INTO book(id, name, star, language, id_lib, file, size, deleted, date, format, keys, id_inlib, archive, first_author_id)
VALUES (1, 'Only', 0, 'en', 1, '', 0, 0, '', '', '', 1, '', NULL);
`); err != nil {
		t.Fatal(err)
	}
	_ = db.Close()
	dbRO, err := openSQLite(dbPath, true)
	if err != nil {
		t.Fatal(err)
	}
	defer dbRO.Close()
	var buf bytes.Buffer
	if err := cmdShowTo(&buf, dbRO, []string{`Only`}); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if strings.Contains(out, "genres:") {
		t.Fatalf("did not expect genres key: %s", out)
	}
	if strings.Contains(out, "library:") {
		t.Fatalf("did not expect library: %s", out)
	}
}

func TestCmdShowProgressOutput(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "progress.sqlite")
	db, err := sql.Open("sqlite", "file:"+dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`
CREATE TABLE lib (id INTEGER PRIMARY KEY, name TEXT, path TEXT);
CREATE TABLE book (
  id INTEGER PRIMARY KEY,
  name TEXT,
  star INTEGER,
  language TEXT,
  id_lib INTEGER,
  file TEXT,
  size INTEGER,
  deleted BOOL,
  date DATETIME,
  format TEXT,
  keys TEXT,
  id_inlib INTEGER,
  archive TEXT,
  first_author_id INTEGER
);
CREATE TABLE author (id INTEGER, name1 TEXT, name2 TEXT, name3 TEXT, id_lib INTEGER, PRIMARY KEY(id));
CREATE TABLE book_author (id_book INTEGER, id_author INTEGER, id_lib INTEGER, PRIMARY KEY(id_book, id_author, id_lib));
INSERT INTO lib(id, name, path) VALUES (1, 'L1', '/lib');
INSERT INTO book(id, name, star, language, id_lib, file, size, deleted, date, format, keys, id_inlib, archive, first_author_id)
VALUES (1, 'Alpha', 0, 'en', 1, '', 0, 0, '', '', '', 1, '', NULL);
`); err != nil {
		t.Fatal(err)
	}
	_ = db.Close()
	dbRO, err := openSQLite(dbPath, true)
	if err != nil {
		t.Fatal(err)
	}
	defer dbRO.Close()

	var out bytes.Buffer
	var progress bytes.Buffer
	if err := cmdShowWithProgress(&out, &progress, dbRO, []string{"Alpha"}); err != nil {
		t.Fatal(err)
	}
	p := progress.String()
	if !strings.Contains(p, "progress 100%, matched 1") {
		t.Fatalf("unexpected progress output: %q", p)
	}
}

func TestInvalidRegex(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "t.sqlite")
	db, err := sql.Open("sqlite", "file:"+dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`
CREATE TABLE lib(id INTEGER PRIMARY KEY);
CREATE TABLE book(id INTEGER PRIMARY KEY, name TEXT, star INTEGER, language TEXT, id_lib INTEGER, file TEXT, size INTEGER, deleted INTEGER, date TEXT, format TEXT, keys TEXT, id_inlib INTEGER, archive TEXT, first_author_id INTEGER);
INSERT INTO lib VALUES(1);
INSERT INTO book VALUES(1,'x',0,'en',1,'',0,0,'','','',0,'',NULL);
`); err != nil {
		t.Fatal(err)
	}
	_ = db.Close()
	dbRO, err := openSQLite(dbPath, true)
	if err != nil {
		t.Fatal(err)
	}
	defer dbRO.Close()
	err = cmdShowTo(io.Discard, dbRO, []string{`(`})
	if err == nil {
		t.Fatal("expected invalid regex")
	}
}

func TestRegexpTitleOnly(t *testing.T) {
	re := regexp.MustCompile(`^A`)
	if !re.MatchString("Alpha") || re.MatchString("xAlpha") {
		t.Fatal("regex semantics")
	}
}

func TestBookRelPath(t *testing.T) {
	if got := bookRelPath("books.zip", "a/b", "fb2"); got != "books.zip:a/b.fb2" {
		t.Fatalf("got %q", got)
	}
	if got := bookRelPath("", "x", "epub"); got != "x.epub" {
		t.Fatalf("got %q", got)
	}
	if got := bookRelPath("z", "f", ""); got != "z:f" {
		t.Fatalf("got %q", got)
	}
}

func TestGenreDisplayNameKnownAndFallback(t *testing.T) {
	if g := genreDisplayName(100); g != "Научная фантастика" {
		t.Fatalf("got %q", g)
	}
	if g := genreDisplayName(999999999); g != "genre:999999999" {
		t.Fatalf("got %q", g)
	}
}

func TestListByAuthorGenreLanguage(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "list.sqlite")
	db, err := sql.Open("sqlite", "file:"+dbPath+"?_pragma=foreign_keys(1)")
	if err != nil {
		t.Fatal(err)
	}
	schema := `
CREATE TABLE lib (id INTEGER PRIMARY KEY, name TEXT, path TEXT);
CREATE TABLE author (id INTEGER, name1 TEXT, name2 TEXT, name3 TEXT, id_lib INTEGER, PRIMARY KEY(id));
CREATE TABLE book (
  id INTEGER PRIMARY KEY,
  name TEXT,
  star INTEGER,
  language TEXT,
  id_lib INTEGER,
  file TEXT,
  size INTEGER,
  deleted BOOL,
  date DATETIME,
  format TEXT,
  keys TEXT,
  id_inlib INTEGER,
  archive TEXT,
  first_author_id INTEGER,
  FOREIGN KEY(id_lib) REFERENCES lib(id)
);
CREATE TABLE book_author (id_book INTEGER, id_author INTEGER, id_lib INTEGER,
  PRIMARY KEY(id_book, id_author, id_lib),
  FOREIGN KEY(id_lib) REFERENCES lib(id),
  FOREIGN KEY(id_author) REFERENCES author(id),
  FOREIGN KEY(id_book) REFERENCES book(id)
);
CREATE TABLE book_genre (id_book INTEGER, id_genre INTEGER, id_lib INTEGER,
  PRIMARY KEY(id_book, id_genre, id_lib),
  FOREIGN KEY(id_lib) REFERENCES lib(id),
  FOREIGN KEY(id_book) REFERENCES book(id)
);
`
	if _, err := db.Exec(schema); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO lib(id, name, path) VALUES (1, 'L1', '/lib');`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO author(id, name1, name2, name3, id_lib) VALUES (1, 'Smith', 'Ann', '', 1);`); err != nil {
		t.Fatal(err)
	}
	// Book 1 and 2 share author Smith Ann; langs en then de for within-group order by language.
	if _, err := db.Exec(`INSERT INTO book(id, name, star, language, id_lib, file, size, deleted, date, format, keys, id_inlib, archive, first_author_id) VALUES
		(1, 'Zebra', 0, 'en', 1, 'z.fb2', 0, 0, '', 'fb2', '', 1, 'a.zip', 1),
		(2, 'Apple', 0, 'de', 1, 'a.fb2', 0, 0, '', 'fb2', '', 2, '', 1),
		(3, 'Solo', 0, '', 1, 's.txt', 0, 0, '', 'txt', '', 3, '', NULL);`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO book_author(id_book, id_author, id_lib) VALUES (1, 1, 1), (2, 1, 1);`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO book_genre(id_book, id_genre, id_lib) VALUES (1, 100, 1), (1, 101, 1);`); err != nil {
		t.Fatal(err)
	}
	_ = db.Close()

	dbRO, err := openSQLite(dbPath, true)
	if err != nil {
		t.Fatal(err)
	}
	defer dbRO.Close()

	var buf bytes.Buffer
	if err := cmdListByAuthor(dbRO, &buf); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, "=== Author: (unknown)") || !strings.Contains(out, "=== Author: Smith Ann") {
		t.Fatalf("author headers: %s", out)
	}
	// Within Smith Ann: sort by language — "de" before "en"
	idxEn := strings.Index(out, "en\tSmith Ann\tZebra")
	idxDe := strings.Index(out, "de\tSmith Ann\tApple")
	if idxEn < 0 || idxDe < 0 || idxDe > idxEn {
		t.Fatalf("within-group sort: %s", out)
	}
	if !strings.Contains(out, "a.zip:z.fb2.fb2") {
		t.Fatalf("relpath: %s", out)
	}

	buf.Reset()
	if err := cmdListByGenre(dbRO, &buf); err != nil {
		t.Fatal(err)
	}
	gout := buf.String()
	if !strings.Contains(gout, "=== Genre: Научная фантастика") || !strings.Contains(gout, "=== Genre: Фэнтези") {
		t.Fatalf("genre headers: %s", gout)
	}
	if strings.Count(gout, "Zebra") != 2 {
		t.Fatalf("book in two genre groups: %s", gout)
	}

	buf.Reset()
	if err := cmdListByLanguage(dbRO, &buf); err != nil {
		t.Fatal(err)
	}
	lout := buf.String()
	if !strings.Contains(lout, "=== Language: (unknown)") || !strings.Contains(lout, "=== Language: de") || !strings.Contains(lout, "=== Language: en") {
		t.Fatalf("language headers: %s", lout)
	}
}

func TestListByGenreLegacyBookJanre(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "legacy_genre.sqlite")
	db, err := sql.Open("sqlite", "file:"+dbPath+"?_pragma=foreign_keys(1)")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`
CREATE TABLE lib (id INTEGER PRIMARY KEY, name TEXT, path TEXT);
CREATE TABLE author (id INTEGER, name1 TEXT, name2 TEXT, name3 TEXT, id_lib INTEGER, PRIMARY KEY(id));
CREATE TABLE book_author (id_book INTEGER, id_author INTEGER, id_lib INTEGER, PRIMARY KEY(id_book, id_author, id_lib));
CREATE TABLE book (
  id INTEGER PRIMARY KEY,
  name TEXT,
  star INTEGER,
  language TEXT,
  id_lib INTEGER,
  file TEXT,
  size INTEGER,
  deleted BOOL,
  date DATETIME,
  format TEXT,
  keys TEXT,
  id_inlib INTEGER,
  archive TEXT,
  first_author_id INTEGER
);
CREATE TABLE book_janre (id_book INTEGER, id_genre INTEGER, id_lib INTEGER, PRIMARY KEY(id_book, id_genre, id_lib));
INSERT INTO lib(id, name, path) VALUES (1, 'L1', '/lib');
INSERT INTO book(id, name, star, language, id_lib, file, size, deleted, date, format, keys, id_inlib, archive, first_author_id)
VALUES (1, 'Legacy', 0, 'en', 1, 'x.fb2', 0, 0, '', 'fb2', '', 1, '', NULL);
INSERT INTO book_janre(id_book, id_genre, id_lib) VALUES (1, 100, 1);
`); err != nil {
		t.Fatal(err)
	}
	_ = db.Close()
	dbRO, err := openSQLite(dbPath, true)
	if err != nil {
		t.Fatal(err)
	}
	defer dbRO.Close()

	var buf bytes.Buffer
	if err := cmdListByGenre(dbRO, &buf); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "=== Genre: Научная фантастика") {
		t.Fatalf("unexpected output: %s", buf.String())
	}
}

func TestListByGenreLegacyIdJanreColumn(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "legacy_col.sqlite")
	db, err := sql.Open("sqlite", "file:"+dbPath+"?_pragma=foreign_keys(1)")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`
CREATE TABLE lib (id INTEGER PRIMARY KEY, name TEXT, path TEXT);
CREATE TABLE author (id INTEGER, name1 TEXT, name2 TEXT, name3 TEXT, id_lib INTEGER, PRIMARY KEY(id));
CREATE TABLE book_author (id_book INTEGER, id_author INTEGER, id_lib INTEGER, PRIMARY KEY(id_book, id_author, id_lib));
CREATE TABLE book (
  id INTEGER PRIMARY KEY,
  name TEXT,
  star INTEGER,
  language TEXT,
  id_lib INTEGER,
  file TEXT,
  size INTEGER,
  deleted BOOL,
  date DATETIME,
  format TEXT,
  keys TEXT,
  id_inlib INTEGER,
  archive TEXT,
  first_author_id INTEGER
);
CREATE TABLE book_janre (id_book INTEGER, id_janre INTEGER, id_lib INTEGER, PRIMARY KEY(id_book, id_janre, id_lib));
INSERT INTO lib(id, name, path) VALUES (1, 'L1', '/lib');
INSERT INTO book(id, name, star, language, id_lib, file, size, deleted, date, format, keys, id_inlib, archive, first_author_id)
VALUES (1, 'LegacyCol', 0, 'en', 1, 'x.fb2', 0, 0, '', 'fb2', '', 1, '', NULL);
INSERT INTO book_janre(id_book, id_janre, id_lib) VALUES (1, 100, 1);
`); err != nil {
		t.Fatal(err)
	}
	_ = db.Close()
	dbRO, err := openSQLite(dbPath, true)
	if err != nil {
		t.Fatal(err)
	}
	defer dbRO.Close()

	var buf bytes.Buffer
	if err := cmdListByGenre(dbRO, &buf); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "=== Genre: Научная фантастика") {
		t.Fatalf("unexpected output: %s", buf.String())
	}
}

func TestParseExtractArgs(t *testing.T) {
	dest, err := parseExtractArgs(nil)
	if err != nil {
		t.Fatal(err)
	}
	if dest != "." {
		t.Fatalf("got %q", dest)
	}
	dest, err = parseExtractArgs([]string{"--destdir", "out"})
	if err != nil {
		t.Fatal(err)
	}
	if dest != "out" {
		t.Fatalf("got %q", dest)
	}
	if _, err := parseExtractArgs([]string{"--destdir"}); err == nil {
		t.Fatal("expected error")
	}
	if _, err := parseExtractArgs([]string{"--bad"}); err == nil {
		t.Fatal("expected error")
	}
}

func TestSanitizePathPartAndUniqueRelPath(t *testing.T) {
	if got := sanitizePathPart("  "); got != "(unknown)" {
		t.Fatalf("got %q", got)
	}
	if got := sanitizePathPart("a/b\\c"); got != "a_b_c" {
		t.Fatalf("got %q", got)
	}
	seen := map[string]int{}
	first := uniqueRelPath("en/Ann/Same.fb2", 1, seen, maxOutputNameBytes)
	second := uniqueRelPath("en/Ann/Same.fb2", 2, seen, maxOutputNameBytes)
	third := uniqueRelPath("en/Ann/Same.fb2", 2, seen, maxOutputNameBytes)
	if first != "en/Ann/Same.fb2" {
		t.Fatalf("first %q", first)
	}
	if second != "en/Ann/Same-2.fb2" {
		t.Fatalf("second %q", second)
	}
	if third != "en/Ann/Same-2-2.fb2" {
		t.Fatalf("third %q", third)
	}
}

func TestShortenFileNamePreservesExtension(t *testing.T) {
	longName := strings.Repeat("a", 400) + ".fb2"
	got := shortenFileName(longName, 120)
	if len(got) > 120 {
		t.Fatalf("still too long: %d", len(got))
	}
	if !strings.HasSuffix(got, ".fb2") {
		t.Fatalf("extension lost: %q", got)
	}
}

func TestCmdExtractIntegrationOverwriteAndCollision(t *testing.T) {
	tmp := t.TempDir()
	libRoot := filepath.Join(tmp, "lib")
	if err := os.MkdirAll(libRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	archivePath := filepath.Join(libRoot, "batch.zip")
	if err := createZipFile(archivePath, map[string]string{
		"f1.fb2": "content-one",
		"f2.fb2": "content-two",
	}); err != nil {
		t.Fatal(err)
	}

	dbPath := filepath.Join(tmp, "extract.sqlite")
	db, err := sql.Open("sqlite", "file:"+dbPath+"?_pragma=foreign_keys(1)")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`
CREATE TABLE lib (id INTEGER PRIMARY KEY, name TEXT, path TEXT);
CREATE TABLE author (id INTEGER, name1 TEXT, name2 TEXT, name3 TEXT, id_lib INTEGER, PRIMARY KEY(id));
CREATE TABLE book_author (id_book INTEGER, id_author INTEGER, id_lib INTEGER, PRIMARY KEY(id_book, id_author, id_lib));
CREATE TABLE book (
  id INTEGER PRIMARY KEY,
  name TEXT,
  star INTEGER,
  language TEXT,
  id_lib INTEGER,
  file TEXT,
  size INTEGER,
  deleted BOOL,
  date DATETIME,
  format TEXT,
  keys TEXT,
  id_inlib INTEGER,
  archive TEXT,
  first_author_id INTEGER
);
INSERT INTO lib(id, name, path) VALUES (1, 'L1', '/lib');
INSERT INTO author(id, name1, name2, name3, id_lib) VALUES (10, 'Doe', 'Jane', '', 1);
INSERT INTO book(id, name, star, language, id_lib, file, size, deleted, date, format, keys, id_inlib, archive, first_author_id) VALUES
  (1, 'Same', 0, 'en', 1, 'f1', 0, 0, '', 'fb2', '', 1, 'batch.zip', 10),
  (2, 'Same', 0, 'en', 1, 'f2', 0, 0, '', 'fb2', '', 2, 'batch.zip', 10);
INSERT INTO book_author(id_book, id_author, id_lib) VALUES (1, 10, 1), (2, 10, 1);
`); err != nil {
		t.Fatal(err)
	}
	_ = db.Close()

	dbRO, err := openSQLite(dbPath, true)
	if err != nil {
		t.Fatal(err)
	}
	defer dbRO.Close()

	dest := filepath.Join(tmp, "out")
	if err := os.MkdirAll(filepath.Join(dest, "en", "Doe Jane"), 0o755); err != nil {
		t.Fatal(err)
	}
	// Precreate destination to validate overwrite behavior.
	pre := filepath.Join(dest, "en", "Doe Jane", "Same.fb2")
	if err := os.WriteFile(pre, []byte("old-data"), 0o644); err != nil {
		t.Fatal(err)
	}

	t.Setenv("FLIB_PATH", libRoot)
	var progress bytes.Buffer
	if err := cmdExtractWithIO(dbRO, &progress, []string{"--destdir", dest}); err != nil {
		t.Fatal(err)
	}

	b1, err := os.ReadFile(filepath.Join(dest, "en", "Doe Jane", "Same.fb2"))
	if err != nil {
		t.Fatal(err)
	}
	if string(b1) != "content-one" {
		t.Fatalf("overwrite failed: %q", string(b1))
	}
	b2, err := os.ReadFile(filepath.Join(dest, "en", "Doe Jane", "Same-2.fb2"))
	if err != nil {
		t.Fatal(err)
	}
	if string(b2) != "content-two" {
		t.Fatalf("suffix extract failed: %q", string(b2))
	}

	p := progress.String()
	if !strings.Contains(p, "en/Doe Jane/Same.fb2") || !strings.Contains(p, "en/Doe Jane/Same-2.fb2") {
		t.Fatalf("unexpected progress: %q", p)
	}
	if !strings.Contains(p, "missing files: 0") {
		t.Fatalf("missing summary: %q", p)
	}
}

func TestCmdExtractMissingArchiveAndMemberContinue(t *testing.T) {
	tmp := t.TempDir()
	libRoot := filepath.Join(tmp, "lib")
	if err := os.MkdirAll(libRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	archivePath := filepath.Join(libRoot, "batch.zip")
	if err := createZipFile(archivePath, map[string]string{
		"present.fb2": "ok-content",
	}); err != nil {
		t.Fatal(err)
	}

	dbPath := filepath.Join(tmp, "extract_missing.sqlite")
	db, err := sql.Open("sqlite", "file:"+dbPath+"?_pragma=foreign_keys(1)")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`
CREATE TABLE lib (id INTEGER PRIMARY KEY, name TEXT, path TEXT);
CREATE TABLE author (id INTEGER, name1 TEXT, name2 TEXT, name3 TEXT, id_lib INTEGER, PRIMARY KEY(id));
CREATE TABLE book_author (id_book INTEGER, id_author INTEGER, id_lib INTEGER, PRIMARY KEY(id_book, id_author, id_lib));
CREATE TABLE book (
  id INTEGER PRIMARY KEY,
  name TEXT,
  star INTEGER,
  language TEXT,
  id_lib INTEGER,
  file TEXT,
  size INTEGER,
  deleted BOOL,
  date DATETIME,
  format TEXT,
  keys TEXT,
  id_inlib INTEGER,
  archive TEXT,
  first_author_id INTEGER
);
INSERT INTO lib(id, name, path) VALUES (1, 'L1', '/lib');
INSERT INTO author(id, name1, name2, name3, id_lib) VALUES (10, 'Doe', 'Jane', '', 1);
INSERT INTO book(id, name, star, language, id_lib, file, size, deleted, date, format, keys, id_inlib, archive, first_author_id) VALUES
  (1, 'MissingArchive', 0, 'en', 1, 'present', 0, 0, '', 'fb2', '', 1, 'missing.zip', 10),
  (2, 'MissingMember', 0, 'en', 1, 'not_there', 0, 0, '', 'fb2', '', 2, 'batch.zip', 10),
  (3, 'Present', 0, 'en', 1, 'present', 0, 0, '', 'fb2', '', 3, 'batch.zip', 10);
INSERT INTO book_author(id_book, id_author, id_lib) VALUES (1, 10, 1), (2, 10, 1), (3, 10, 1);
`); err != nil {
		t.Fatal(err)
	}
	_ = db.Close()

	dbRO, err := openSQLite(dbPath, true)
	if err != nil {
		t.Fatal(err)
	}
	defer dbRO.Close()

	t.Setenv("FLIB_PATH", libRoot)
	dest := filepath.Join(tmp, "out")
	var progress bytes.Buffer
	if err := cmdExtractWithIO(dbRO, &progress, []string{"--destdir", dest}); err != nil {
		t.Fatal(err)
	}

	got, err := os.ReadFile(filepath.Join(dest, "en", "Doe Jane", "Present.fb2"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "ok-content" {
		t.Fatalf("unexpected extracted content: %q", string(got))
	}

	p := progress.String()
	if !strings.Contains(p, "missing archive: missing.zip") {
		t.Fatalf("expected missing archive line: %q", p)
	}
	if !strings.Contains(p, "missing member: not_there.fb2 in batch.zip") {
		t.Fatalf("expected missing member line: %q", p)
	}
	if !strings.Contains(p, "en/Doe Jane/Present.fb2") {
		t.Fatalf("expected created path line: %q", p)
	}
	if !strings.Contains(p, "missing files: 2") {
		t.Fatalf("expected missing count: %q", p)
	}
}

func TestCmdExtractSkipsDeletedBooks(t *testing.T) {
	tmp := t.TempDir()
	libRoot := filepath.Join(tmp, "lib")
	if err := os.MkdirAll(libRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	archivePath := filepath.Join(libRoot, "batch.zip")
	if err := createZipFile(archivePath, map[string]string{
		"active.fb2": "live-content",
	}); err != nil {
		t.Fatal(err)
	}

	dbPath := filepath.Join(tmp, "extract_deleted.sqlite")
	db, err := sql.Open("sqlite", "file:"+dbPath+"?_pragma=foreign_keys(1)")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`
CREATE TABLE lib (id INTEGER PRIMARY KEY, name TEXT, path TEXT);
CREATE TABLE author (id INTEGER, name1 TEXT, name2 TEXT, name3 TEXT, id_lib INTEGER, PRIMARY KEY(id));
CREATE TABLE book_author (id_book INTEGER, id_author INTEGER, id_lib INTEGER, PRIMARY KEY(id_book, id_author, id_lib));
CREATE TABLE book (
  id INTEGER PRIMARY KEY,
  name TEXT,
  star INTEGER,
  language TEXT,
  id_lib INTEGER,
  file TEXT,
  size INTEGER,
  deleted BOOL,
  date DATETIME,
  format TEXT,
  keys TEXT,
  id_inlib INTEGER,
  archive TEXT,
  first_author_id INTEGER
);
INSERT INTO lib(id, name, path) VALUES (1, 'L1', '/lib');
INSERT INTO author(id, name1, name2, name3, id_lib) VALUES (10, 'Doe', 'Jane', '', 1);
INSERT INTO book(id, name, star, language, id_lib, file, size, deleted, date, format, keys, id_inlib, archive, first_author_id) VALUES
  (1, 'DeletedMissingZip', 0, 'en', 1, 'x', 0, 1, '', 'fb2', '', 1, 'gone.zip', 10),
  (2, 'DeletedMissingMember', 0, 'en', 1, 'nomember', 0, 1, '', 'fb2', '', 2, 'batch.zip', 10),
  (3, 'OkBook', 0, 'en', 1, 'active', 0, 0, '', 'fb2', '', 3, 'batch.zip', 10);
INSERT INTO book_author(id_book, id_author, id_lib) VALUES (1, 10, 1), (2, 10, 1), (3, 10, 1);
`); err != nil {
		t.Fatal(err)
	}
	_ = db.Close()

	dbRO, err := openSQLite(dbPath, true)
	if err != nil {
		t.Fatal(err)
	}
	defer dbRO.Close()

	t.Setenv("FLIB_PATH", libRoot)
	dest := filepath.Join(tmp, "out")
	var progress bytes.Buffer
	if err := cmdExtractWithIO(dbRO, &progress, []string{"--destdir", dest}); err != nil {
		t.Fatal(err)
	}

	got, err := os.ReadFile(filepath.Join(dest, "en", "Doe Jane", "OkBook.fb2"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "live-content" {
		t.Fatalf("unexpected extracted content: %q", string(got))
	}

	if _, err := os.Stat(filepath.Join(dest, "en", "Doe Jane", "DeletedMissingZip.fb2")); !os.IsNotExist(err) {
		t.Fatalf("deleted book file should not exist: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dest, "en", "Doe Jane", "DeletedMissingMember.fb2")); !os.IsNotExist(err) {
		t.Fatalf("deleted book file should not exist: %v", err)
	}

	p := progress.String()
	if strings.Contains(p, "gone.zip") {
		t.Fatalf("deleted row should not trigger missing archive: %q", p)
	}
	if strings.Contains(p, "nomember.fb2") {
		t.Fatalf("deleted row should not trigger missing member: %q", p)
	}
	if !strings.Contains(p, "en/Doe Jane/OkBook.fb2") {
		t.Fatalf("expected created path line: %q", p)
	}
	if !strings.Contains(p, "missing files: 0") {
		t.Fatalf("expected no missing files for skipped deleted rows: %q", p)
	}
}

func createZipFile(path string, files map[string]string) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	zw := zip.NewWriter(f)
	for name, content := range files {
		w, err := zw.Create(name)
		if err != nil {
			return err
		}
		if _, err := io.WriteString(w, content); err != nil {
			return err
		}
	}
	if err := zw.Close(); err != nil {
		return err
	}
	return f.Close()
}
