package config

import (
	"os"
	"strings"

	"woden/internal/errs"
	"woden/internal/fsx"
	"woden/internal/tomlutil"
	"woden/internal/validate"
)

// Editor applies minimal textual edits to a woden.toml file.
//
// Woden must never rewrite the whole manifest: comments, spacing and the
// original line endings have to survive. The editor therefore works on the raw
// lines of the file instead of re-encoding the decoded configuration. Every
// edit is validated by Edit (which re-decodes the result) before it is written.
type Editor struct {
	lines   []string
	newline string
	changed bool
}

// NewEditor parses data and returns an editor for it.
func NewEditor(data []byte) (*Editor, error) {
	text, newline, err := normalizeNewlines(data)
	if err != nil {
		return nil, err
	}
	lines := splitLines(text)
	e := &Editor{lines: lines, newline: newline}
	if err := e.checkStructure(); err != nil {
		return nil, err
	}
	return e, nil
}

// Bytes returns the edited file. The result always ends with exactly one line
// ending, using the line ending style of the original file.
func (e *Editor) Bytes() []byte {
	lines := trimTrailingBlankLines(e.lines)
	text := strings.Join(lines, "\n")
	if len(lines) > 0 {
		text += "\n"
	}
	if e.newline == "\r\n" {
		text = strings.ReplaceAll(text, "\n", "\r\n")
	}
	return []byte(text)
}

// Changed reports whether an edit was applied.
func (e *Editor) Changed() bool { return e.changed }

// AddDependency appends a new [dependencies.<name>] section at the end of the
// file, separated from the previous content by one blank line.
func (e *Editor) AddDependency(dep Dependency) error {
	if err := validateDependencyFields(dep); err != nil {
		return err
	}
	if e.hasTable([]string{"dependencies", dep.Name}) {
		return errs.Userf("dependency %q is already declared in woden.toml", dep.Name)
	}
	e.lines = trimTrailingBlankLines(e.lines)
	header := "[dependencies." + tomlutil.Key(dep.Name) + "]"
	if len(e.lines) > 0 {
		e.lines = append(e.lines, "")
	}
	e.lines = append(e.lines, header)
	e.lines = append(e.lines, "git = "+tomlutil.Quote(dep.Git))
	if dep.Branch != "" {
		e.lines = append(e.lines, "branch = "+tomlutil.Quote(dep.Branch))
	}
	if dep.Tag != "" {
		e.lines = append(e.lines, "tag = "+tomlutil.Quote(dep.Tag))
	}
	if dep.Rev != "" {
		e.lines = append(e.lines, "rev = "+tomlutil.Quote(dep.Rev))
	}
	e.changed = true
	return nil
}

// RemoveDependency deletes the [dependencies.<name>] section together with the
// comment lines directly above it and the blank line separating it from its
// neighbours. It is a no-op when the dependency is not declared as its own
// section.
func (e *Editor) RemoveDependency(name string) error {
	path := []string{"dependencies", name}
	start, headerEnd, tableForm, err := e.section(path)
	if err != nil {
		return err
	}
	if !tableForm {
		if e.hasTable(path) || e.hasInlineAssignment(path) {
			return errs.Userf("dependency %q is not declared as its own [dependencies.%s] table and cannot be edited safely", name, name)
		}
		return nil
	}
	end := sectionEnd(e.lines, headerEnd)
	// Comments and blank lines at the end of the section belong to the next
	// section, not to this one.
	for end > headerEnd {
		line := e.lines[end-1]
		if strings.TrimSpace(line) == "" || isCommentLine(line) {
			end--
			continue
		}
		break
	}
	// Include the comment block directly above the section header.
	for start > 0 && isCommentLine(e.lines[start-1]) {
		start--
	}
	// Keep exactly one blank line between the surrounding sections.
	if start > 0 && strings.TrimSpace(e.lines[start-1]) == "" && end < len(e.lines) && strings.TrimSpace(e.lines[end]) == "" {
		start--
	} else if end < len(e.lines) && strings.TrimSpace(e.lines[end]) == "" {
		end++
	} else if start > 0 && strings.TrimSpace(e.lines[start-1]) == "" && end >= len(e.lines) {
		start--
	}
	e.removeRange(start, end)
	e.changed = true
	return nil
}

// SetCollection sets the path of collection name, adding the [collections]
// table when it does not exist yet.
func (e *Editor) SetCollection(name, path string) error {
	if err := validateCollectionFields(name, path); err != nil {
		return err
	}
	headerStart, headerEnd, tableForm, err := e.section([]string{"collections"})
	if err != nil {
		return err
	}
	if !tableForm {
		if e.hasInlineAssignment([]string{"collections"}) {
			return errs.Userf("woden.toml declares [collections] as an inline table, which cannot be edited safely")
		}
		// Append a new [collections] table at the end of the file.
		e.lines = trimTrailingBlankLines(e.lines)
		if len(e.lines) > 0 {
			e.lines = append(e.lines, "")
		}
		e.lines = append(e.lines, "[collections]", tomlutil.Key(name)+" = "+tomlutil.Quote(path))
		e.changed = true
		return nil
	}
	_ = headerStart
	// Look for an existing assignment inside the section.
	for i := headerEnd; i < sectionEnd(e.lines, headerEnd); i++ {
		key, valueStart, valueEnd, ok, err := parseAssignmentLine(e.lines[i])
		if err != nil {
			return err
		}
		if !ok || key != name {
			continue
		}
		if valueStart < 0 {
			return errs.Userf("collection %q does not use a plain string value and cannot be edited safely", name)
		}
		e.lines[i] = e.lines[i][:valueStart] + tomlutil.Quote(path) + e.lines[i][valueEnd:]
		e.changed = true
		return nil
	}
	// Insert a new assignment at the end of the section, after the last
	// non-blank line.
	insertAt := headerEnd
	for i := headerEnd; i < sectionEnd(e.lines, headerEnd); i++ {
		if strings.TrimSpace(e.lines[i]) != "" {
			insertAt = i + 1
		}
	}
	line := tomlutil.Key(name) + " = " + tomlutil.Quote(path)
	e.lines = append(e.lines[:insertAt], append([]string{line}, e.lines[insertAt:]...)...)
	e.changed = true
	return nil
}

// RemoveCollection deletes the assignment of collection name. It is a no-op
// when the collection is not declared.
func (e *Editor) RemoveCollection(name string) error {
	headerStart, headerEnd, tableForm, err := e.section([]string{"collections"})
	if err != nil {
		return err
	}
	if !tableForm {
		return nil
	}
	_ = headerStart
	for i := headerEnd; i < sectionEnd(e.lines, headerEnd); i++ {
		key, _, _, ok, err := parseAssignmentLine(e.lines[i])
		if err != nil {
			return err
		}
		if !ok || key != name {
			continue
		}
		e.removeRange(i, i+1)
		e.changed = true
		return nil
	}
	return nil
}

// checkStructure rejects files whose managed tables are declared in a form the
// editor cannot edit safely (inline tables).
func (e *Editor) checkStructure() error {
	for _, key := range []string{"package", "collections", "profile", "dependencies"} {
		if e.hasInlineAssignment([]string{key}) {
			return errs.Userf("woden.toml declares %q as an inline value, which cannot be edited safely; use table syntax instead", key)
		}
	}
	return nil
}

// section returns the line range of the table path: the header is at
// headerStart and headerEnd is the first line after the header. tableForm is
// false when no such header exists.
func (e *Editor) section(path []string) (headerStart, headerEnd int, tableForm bool, err error) {
	for i, line := range e.lines {
		header, ok, err := parseHeaderLine(line)
		if err != nil {
			return 0, 0, false, err
		}
		if !ok || !equalPath(header, path) {
			continue
		}
		return i, i + 1, true, nil
	}
	return 0, 0, false, nil
}

// hasTable reports whether a table with exactly path exists.
func (e *Editor) hasTable(path []string) bool {
	for _, line := range e.lines {
		header, ok, err := parseHeaderLine(line)
		if err == nil && ok && equalPath(header, path) {
			return true
		}
	}
	return false
}

// hasInlineAssignment reports whether path is assigned with "=" anywhere in the
// file (e.g. "collections = { app = \"src\" }"). The assignment may appear
// after a table header, where TOML would attach it to that table; it is still
// a form the editor cannot edit safely.
func (e *Editor) hasInlineAssignment(path []string) bool {
	for _, line := range e.lines {
		key, _, _, ok, err := parseAssignmentLine(line)
		if err != nil || !ok {
			continue
		}
		if equalPath([]string{key}, path) {
			return true
		}
	}
	return false
}

// removeRange deletes lines [start, end).
func (e *Editor) removeRange(start, end int) {
	if start < 0 {
		start = 0
	}
	if end > len(e.lines) {
		end = len(e.lines)
	}
	if start >= end {
		return
	}
	e.lines = append(e.lines[:start], e.lines[end:]...)
}

// Edit applies fn to the manifest at path, verifies that the result still
// decodes, and writes it atomically. The file's line ending style and
// permissions are preserved. When fn does not change anything the file is left
// untouched.
func Edit(path string, fn func(*Editor) error) error {
	if fn == nil {
		return errs.Userf("internal error: no editor function")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return errs.WrapUser(err, "cannot read %s", path)
	}
	editor, err := NewEditor(data)
	if err != nil {
		return errs.WrapUser(err, "cannot edit %s", path)
	}
	if err := fn(editor); err != nil {
		return err
	}
	if !editor.Changed() {
		return nil
	}
	out := editor.Bytes()
	if _, _, err := Decode(out); err != nil {
		return errs.WrapUser(err, "refusing to write %s: the edited file would be invalid", path)
	}
	perm := os.FileMode(0o644)
	if info, err := os.Stat(path); err == nil {
		perm = info.Mode().Perm()
	}
	if err := fsx.AtomicWrite(path, out, perm); err != nil {
		return err
	}
	return nil
}

// validateDependencyFields performs the checks the editor needs before it
// writes a dependency section.
func validateDependencyFields(dep Dependency) error {
	if err := validate.DependencyName(dep.Name); err != nil {
		return errs.WrapUser(err, "invalid dependency name")
	}
	if dep.Git == "" {
		return errs.Userf("dependency %q has no git source", dep.Name)
	}
	if err := validate.GitURL(dep.Git); err != nil {
		return errs.WrapUser(err, "invalid git source for dependency %q", dep.Name)
	}
	refs := 0
	if dep.Branch != "" {
		refs++
		if err := validate.RefName(dep.Branch); err != nil {
			return errs.WrapUser(err, "invalid branch for dependency %q", dep.Name)
		}
	}
	if dep.Tag != "" {
		refs++
		if err := validate.RefName(dep.Tag); err != nil {
			return errs.WrapUser(err, "invalid tag for dependency %q", dep.Name)
		}
	}
	if dep.Rev != "" {
		refs++
		if err := validate.Rev(dep.Rev); err != nil {
			return errs.WrapUser(err, "invalid revision for dependency %q", dep.Name)
		}
	}
	if refs > 1 {
		return errs.Userf("dependency %q may specify only one of branch, tag and rev", dep.Name)
	}
	return nil
}

func validateCollectionFields(name, path string) error {
	if err := validate.CollectionName(name); err != nil {
		return errs.WrapUser(err, "invalid collection name")
	}
	if err := validate.CollectionPath(path); err != nil {
		return errs.WrapUser(err, "invalid path for collection %q", name)
	}
	return nil
}

// normalizeNewlines detects the line ending style of data and returns the text
// with LF line endings only. Mixed line endings are rejected: Woden refuses to
// guess which style to use when writing the file back.
func normalizeNewlines(data []byte) (string, string, error) {
	text := string(data)
	if strings.HasPrefix(text, "\uFEFF") {
		return "", "", errs.Userf("woden.toml starts with a byte order mark; re-save it as UTF-8 without BOM")
	}
	hasCRLF := strings.Contains(text, "\r\n")
	loneLF := strings.Contains(strings.ReplaceAll(text, "\r\n", ""), "\n")
	hasLoneCR := strings.Contains(strings.ReplaceAll(text, "\r\n", ""), "\r")
	if hasLoneCR {
		return "", "", errs.Userf("woden.toml contains a bare carriage return")
	}
	if hasCRLF && loneLF {
		return "", "", errs.Userf("woden.toml mixes CRLF and LF line endings")
	}
	if hasCRLF {
		return strings.ReplaceAll(text, "\r\n", "\n"), "\r\n", nil
	}
	return text, "\n", nil
}

// splitLines splits text into lines. A trailing line ending does not produce a
// final empty line, so joining the result with "\n" and appending one newline
// reproduces the file.
func splitLines(text string) []string {
	if text == "" {
		return nil
	}
	text = strings.TrimSuffix(text, "\n")
	return strings.Split(text, "\n")
}

// trimTrailingBlankLines removes blank lines at the end of lines.
func trimTrailingBlankLines(lines []string) []string {
	for len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}

// sectionEnd returns the index of the next table header after the header at
// headerEnd-1, or len(lines).
func sectionEnd(lines []string, headerEnd int) int {
	for i := headerEnd; i < len(lines); i++ {
		if _, ok, err := parseHeaderLine(lines[i]); err == nil && ok {
			return i
		}
	}
	return len(lines)
}

func isCommentLine(line string) bool {
	return strings.HasPrefix(strings.TrimSpace(line), "#")
}

func equalPath(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// parseHeaderLine parses a table header line such as [collections] or
// [dependencies."my-dep"]. It returns ok=false for lines that are not headers.
func parseHeaderLine(line string) (path []string, ok bool, err error) {
	trimmed := strings.TrimSpace(line)
	if !strings.HasPrefix(trimmed, "[") {
		return nil, false, nil
	}
	if strings.HasPrefix(trimmed, "[[") {
		// Arrays of tables are not used by woden.toml; treat them as foreign
		// content and do not mistake them for a table.
		return nil, false, nil
	}
	i := 1
	for {
		for i < len(trimmed) && (trimmed[i] == ' ' || trimmed[i] == '\t') {
			i++
		}
		if i >= len(trimmed) {
			return nil, false, errs.Userf("invalid table header %q", line)
		}
		var key string
		switch trimmed[i] {
		case '"':
			var err error
			key, i, err = parseBasicString(trimmed, i)
			if err != nil {
				return nil, false, err
			}
		case '\'':
			var err error
			key, i, err = parseLiteralString(trimmed, i)
			if err != nil {
				return nil, false, err
			}
		default:
			start := i
			for i < len(trimmed) && isBareKeyChar(trimmed[i]) {
				i++
			}
			if start == i {
				return nil, false, errs.Userf("invalid table header %q", line)
			}
			key = trimmed[start:i]
		}
		path = append(path, key)
		for i < len(trimmed) && (trimmed[i] == ' ' || trimmed[i] == '\t') {
			i++
		}
		if i >= len(trimmed) {
			return nil, false, errs.Userf("invalid table header %q", line)
		}
		switch trimmed[i] {
		case '.':
			i++
			continue
		case ']':
			i++
			rest := strings.TrimSpace(trimmed[i:])
			if rest != "" && !strings.HasPrefix(rest, "#") {
				return nil, false, errs.Userf("invalid table header %q", line)
			}
			return path, true, nil
		default:
			return nil, false, errs.Userf("invalid table header %q", line)
		}
	}
}

// parseAssignmentLine parses "key = value" inside a table section.
// valueStart/valueEnd are byte offsets of a plain string value in line, or -1
// when the value is not a plain string.
func parseAssignmentLine(line string) (key string, valueStart, valueEnd int, ok bool, err error) {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" || strings.HasPrefix(trimmed, "#") || strings.HasPrefix(trimmed, "[") {
		return "", 0, 0, false, nil
	}
	offset := len(line) - len(strings.TrimLeft(line, " \t"))
	i := offset
	var name string
	switch line[i] {
	case '"':
		name, i, err = parseBasicString(line, i)
		if err != nil {
			return "", 0, 0, false, err
		}
	case '\'':
		name, i, err = parseLiteralString(line, i)
		if err != nil {
			return "", 0, 0, false, err
		}
	default:
		start := i
		for i < len(line) && isBareKeyChar(line[i]) {
			i++
		}
		if start == i {
			return "", 0, 0, false, nil
		}
		name = line[start:i]
	}
	for i < len(line) && (line[i] == ' ' || line[i] == '\t') {
		i++
	}
	if i >= len(line) || line[i] != '=' {
		return "", 0, 0, false, nil
	}
	i++
	for i < len(line) && (line[i] == ' ' || line[i] == '\t') {
		i++
	}
	if i >= len(line) {
		return name, -1, -1, true, nil
	}
	switch line[i] {
	case '"':
		_, end, err := parseBasicString(line, i)
		if err != nil {
			return "", 0, 0, false, err
		}
		return name, i, end, true, nil
	case '\'':
		_, end, err := parseLiteralString(line, i)
		if err != nil {
			return "", 0, 0, false, err
		}
		return name, i, end, true, nil
	default:
		return name, -1, -1, true, nil
	}
}

// parseBasicString parses a double-quoted TOML string starting at start. It
// returns the decoded value and the index just after the closing quote.
func parseBasicString(s string, start int) (string, int, error) {
	if start >= len(s) || s[start] != '"' {
		return "", 0, errs.Userf("expected a string")
	}
	var b strings.Builder
	for i := start + 1; i < len(s); i++ {
		switch s[i] {
		case '"':
			return b.String(), i + 1, nil
		case '\\':
			if i+1 >= len(s) {
				return "", 0, errs.Userf("unterminated escape sequence")
			}
			i++
			switch s[i] {
			case 'n':
				b.WriteByte('\n')
			case 't':
				b.WriteByte('\t')
			case 'r':
				b.WriteByte('\r')
			case '"':
				b.WriteByte('"')
			case '\\':
				b.WriteByte('\\')
			default:
				b.WriteByte('\\')
				b.WriteByte(s[i])
			}
		default:
			b.WriteByte(s[i])
		}
	}
	return "", 0, errs.Userf("unterminated string")
}

// parseLiteralString parses a single-quoted TOML string starting at start.
func parseLiteralString(s string, start int) (string, int, error) {
	if start >= len(s) || s[start] != '\'' {
		return "", 0, errs.Userf("expected a literal string")
	}
	end := strings.IndexByte(s[start+1:], '\'')
	if end < 0 {
		return "", 0, errs.Userf("unterminated literal string")
	}
	return s[start+1 : start+1+end], start + 2 + end, nil
}

func isBareKeyChar(c byte) bool {
	return c == '-' || c == '_' || c >= '0' && c <= '9' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z'
}
