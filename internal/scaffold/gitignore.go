package scaffold

import (
	"bytes"
	"strings"
)

// gitignoreEntries are the entries a Woden project's .gitignore always has, in
// the order they are appended.
var gitignoreEntries = []string{"/target/", "/vendor/"}

// appendMissingGitignoreLines returns data with the well-known entries that are
// not present yet appended at the end.
//
// The existing bytes are never changed: only a separating newline, when the file
// does not end with one, and the missing entries are added. Entries are compared
// after trimming surrounding whitespace, the way Cargo compares .gitignore lines,
// so a differently indented entry is not duplicated. Appended lines use the line
// ending style of the existing file.
func appendMissingGitignoreLines(data []byte) ([]byte, bool) {
	newline := "\n"
	if bytes.Contains(data, []byte("\r\n")) {
		newline = "\r\n"
	}
	present := make(map[string]bool, len(gitignoreEntries))
	for _, line := range strings.Split(string(data), "\n") {
		present[strings.TrimSpace(line)] = true
	}
	var missing []string
	for _, entry := range gitignoreEntries {
		if !present[entry] {
			missing = append(missing, entry)
		}
	}
	if len(missing) == 0 {
		return data, false
	}
	var builder bytes.Buffer
	builder.Write(data)
	if builder.Len() > 0 && !bytes.HasSuffix(data, []byte("\n")) {
		builder.WriteString(newline)
	}
	for _, entry := range missing {
		builder.WriteString(entry)
		builder.WriteString(newline)
	}
	return builder.Bytes(), true
}
