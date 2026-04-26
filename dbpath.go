package main

import (
	"os"
	"path/filepath"
	"strings"
)

const defaultDBRelative = "Documents/freeLib.sqlite"

// resolveDBPath returns the SQLite catalog path from FLIB_DB or ~/Documents/freeLib.sqlite.
func resolveDBPath() (string, error) {
	if p := strings.TrimSpace(os.Getenv("FLIB_DB")); p != "" {
		return expandHome(p), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, defaultDBRelative), nil
}

func expandHome(p string) string {
	if strings.HasPrefix(p, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, p[2:])
		}
	}
	return p
}
