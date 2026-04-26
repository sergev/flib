package main

import (
	"archive/zip"
	"database/sql"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

type extractRow struct {
	id            int64
	idLib         int64
	language      string
	title         string
	file          string
	format        string
	archive       string
	firstAuthorID sql.NullInt64
}

func parseExtractArgs(args []string) (destdir string, err error) {
	destdir = "."
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--destdir":
			if i+1 >= len(args) {
				return "", fmt.Errorf("--destdir requires a directory")
			}
			destdir = args[i+1]
			i++
		default:
			return "", fmt.Errorf("unknown argument: %s", args[i])
		}
	}
	return destdir, nil
}

func resolveLibraryPath() (string, error) {
	p := strings.TrimSpace(os.Getenv("FLIB_PATH"))
	if p == "" {
		return "", fmt.Errorf("FLIB_PATH is not set")
	}
	p = expandHome(p)
	st, err := os.Stat(p)
	if err != nil {
		return "", err
	}
	if !st.IsDir() {
		return "", fmt.Errorf("FLIB_PATH is not a directory: %s", p)
	}
	return p, nil
}

func cmdExtract(db *sql.DB, args []string) error {
	return cmdExtractWithIO(db, os.Stderr, args)
}

func cmdExtractWithIO(db *sql.DB, progress io.Writer, args []string) error {
	destdir, err := parseExtractArgs(args)
	if err != nil {
		return err
	}
	libPath, err := resolveLibraryPath()
	if err != nil {
		return fmt.Errorf("library path: %w", err)
	}
	destAbs, err := filepath.Abs(destdir)
	if err != nil {
		return fmt.Errorf("destdir: %w", err)
	}
	if err := os.MkdirAll(destAbs, 0o755); err != nil {
		return fmt.Errorf("create destdir: %w", err)
	}

	rows, err := db.Query(`
		SELECT
			b.id, b.id_lib,
			COALESCE(NULLIF(TRIM(b.language), ''), ''),
			COALESCE(NULLIF(TRIM(b.name), ''), ''),
			COALESCE(b.file, ''),
			COALESCE(b.format, ''),
			COALESCE(b.archive, ''),
			b.first_author_id
		FROM book b
		ORDER BY b.id_lib ASC, b.id ASC
	`)
	if err != nil {
		return err
	}
	defer rows.Close()

	seenDest := make(map[string]int)
	archiveCache := make(map[string]*zip.ReadCloser)
	missingCount := 0
	defer func() {
		for _, zr := range archiveCache {
			_ = zr.Close()
		}
	}()

	for rows.Next() {
		var r extractRow
		if err := rows.Scan(&r.id, &r.idLib, &r.language, &r.title, &r.file, &r.format, &r.archive, &r.firstAuthorID); err != nil {
			return err
		}
		author, err := authorForBook(db, r.id, r.idLib, r.firstAuthorID)
		if err != nil {
			return err
		}
		memberName := bookRelPath("", r.file, r.format)
		srcArchive := filepath.Join(libPath, r.archive)
		zr, err := getOrOpenZip(archiveCache, srcArchive)
		if err != nil {
			missingCount++
			fmt.Fprintf(progress, "missing archive: %s (%v)\n", filepath.ToSlash(r.archive), err)
			continue
		}
		zf, err := findZipMember(zr, memberName)
		if err != nil {
			missingCount++
			fmt.Fprintf(progress, "missing member: %s in %s\n", memberName, filepath.ToSlash(r.archive))
			continue
		}

		langPart := sanitizePathPart(r.language)
		authorPart := sanitizePathPart(author)
		bookPart := sanitizePathPart(bookRelPath("", r.title, r.format))
		relDest := filepath.Join(langPart, authorPart, bookPart)
		relDest = uniqueRelPath(relDest, r.id, seenDest)
		dstPath := filepath.Join(destAbs, relDest)

		if err := os.MkdirAll(filepath.Dir(dstPath), 0o755); err != nil {
			return fmt.Errorf("create output dir for %s: %w", relDest, err)
		}
		if err := writeZipFile(zf, dstPath); err != nil {
			return fmt.Errorf("write %s: %w", relDest, err)
		}
		fmt.Fprintln(progress, filepath.ToSlash(relDest))
	}
	if err := rows.Err(); err != nil {
		return err
	}
	fmt.Fprintf(progress, "missing files: %d\n", missingCount)
	return nil
}

func getOrOpenZip(cache map[string]*zip.ReadCloser, archivePath string) (*zip.ReadCloser, error) {
	if zr, ok := cache[archivePath]; ok {
		return zr, nil
	}
	zr, err := zip.OpenReader(archivePath)
	if err != nil {
		return nil, err
	}
	cache[archivePath] = zr
	return zr, nil
}

func findZipMember(zr *zip.ReadCloser, memberName string) (*zip.File, error) {
	for _, f := range zr.File {
		if f.Name == memberName {
			return f, nil
		}
	}
	return nil, fmt.Errorf("member %q not found", memberName)
}

func writeZipFile(zf *zip.File, dstPath string) error {
	rc, err := zf.Open()
	if err != nil {
		return err
	}
	defer rc.Close()

	out, err := os.Create(dstPath)
	if err != nil {
		return err
	}
	defer out.Close()

	if _, err := io.Copy(out, rc); err != nil {
		return err
	}
	return out.Close()
}

func sanitizePathPart(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return "(unknown)"
	}
	s = strings.ReplaceAll(s, "/", "_")
	s = strings.ReplaceAll(s, "\\", "_")
	s = strings.ReplaceAll(s, string(filepath.Separator), "_")
	return s
}

func uniqueRelPath(rel string, id int64, seen map[string]int) string {
	if seen[rel] == 0 {
		seen[rel] = 1
		return rel
	}
	dir := filepath.Dir(rel)
	base := filepath.Base(rel)
	ext := filepath.Ext(base)
	name := strings.TrimSuffix(base, ext)
	candidate := filepath.Join(dir, fmt.Sprintf("%s-%d%s", name, id, ext))
	if seen[candidate] == 0 {
		seen[candidate] = 1
		return candidate
	}
	for i := 2; ; i++ {
		c := filepath.Join(dir, fmt.Sprintf("%s-%d-%d%s", name, id, i, ext))
		if seen[c] == 0 {
			seen[c] = 1
			return c
		}
	}
}
