package main

import "strings"

// bookRelPath returns archive:file.format when archive is set; otherwise file.format.
// If format is empty, the trailing dot is omitted.
func bookRelPath(archive, file, format string) string {
	archive = strings.TrimSpace(archive)
	file = strings.TrimSpace(file)
	format = strings.TrimSpace(format)
	suffix := ""
	if format != "" {
		suffix = "." + format
	}
	if archive != "" {
		return archive + ":" + file + suffix
	}
	return file + suffix
}
