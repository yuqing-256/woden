// Package ols merges the collections managed by Woden into an Odin Language
// Server configuration file (ols.json).
//
// The merge is deliberately textual and minimal: Woden locates the top-level
// "collections" array of an existing ols.json, adds or updates the entries it
// manages, and leaves every other byte of the file untouched. The file is
// never regenerated, so comments, formatting, ordering and OLS-only settings
// survive a sync.
package ols

import (
	"bytes"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"woden/internal/errs"
	"woden/internal/fsx"
)

// FileName is the OLS configuration file managed by Woden.
const FileName = "ols.json"

// depsCollection is the collection Woden manages completely: it is added when
// the manifest maps it and removed when the manifest stops mapping it. Entries
// of every other collection are only ever added or updated, never deleted.
const depsCollection = "deps"

// Sync merges the managed collections into the ols.json file of the project at
// root and writes the result atomically when something changed.
//
// collections is the manifest's [collections] table. An entry whose name is
// present in both the manifest and ols.json is updated to the manifest path;
// entries that only exist in ols.json are preserved; the "deps" entry is
// removed when the manifest no longer maps it.
//
// Sync reports whether the file changed and whether ols.json exists at all. It
// never creates a missing ols.json: exists is then false and changed false.
func Sync(root string, collections map[string]string) (changed bool, exists bool, err error) {
	path, err := configPath(root)
	if err != nil {
		return false, false, err
	}
	data, exists, err := readConfig(path)
	if err != nil || !exists {
		return false, exists, err
	}
	out, changed, err := merge(data, collections)
	if err != nil {
		return false, true, errs.WrapUser(err, "cannot update %s", path)
	}
	if !changed {
		return false, true, nil
	}
	if err := fsx.AtomicWrite(path, out, 0o644); err != nil {
		return false, true, errs.WrapUser(err, "cannot write %s", path)
	}
	return true, true, nil
}

// Check reports whether ols.json already contains exactly the managed
// collections without modifying anything.
//
// consistent is false when Sync would change the file. Check returns an error
// when the file cannot be parsed or when it is laid out in a way Woden refuses
// to edit; exists is false when the project has no ols.json.
func Check(root string, collections map[string]string) (consistent bool, exists bool, err error) {
	path, err := configPath(root)
	if err != nil {
		return false, false, err
	}
	data, exists, err := readConfig(path)
	if err != nil || !exists {
		return false, exists, err
	}
	_, changed, err := merge(data, collections)
	if err != nil {
		return false, true, errs.WrapUser(err, "cannot check %s", path)
	}
	return !changed, true, nil
}

// configPath returns the path of the project's ols.json file.
func configPath(root string) (string, error) {
	if strings.TrimSpace(root) == "" {
		return "", errs.Userf("cannot locate ols.json: the project root is empty")
	}
	abs, err := fsx.AbsClean(root)
	if err != nil {
		return "", errs.WrapUser(err, "cannot resolve the project root")
	}
	return filepath.Join(abs, FileName), nil
}

// readConfig reads path, reporting a missing file as exists=false.
func readConfig(path string) (data []byte, exists bool, err error) {
	data, err = os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, false, nil
		}
		return nil, false, errs.WrapUser(err, "cannot read %s", path)
	}
	return data, true, nil
}

// spanEdit replaces the bytes of a span with a byte slice.
type spanEdit struct {
	span        span
	replacement []byte
}

// merge computes the new content of an ols.json file. It reports whether the
// content differs from data. The content is returned even when unchanged so
// callers can compare or write it directly.
func merge(data []byte, collections map[string]string) ([]byte, bool, error) {
	doc, err := parseDocument(data)
	if err != nil {
		return nil, false, err
	}
	if doc.collections == nil {
		if len(collections) == 0 {
			return data, false, nil
		}
		return insertCollections(doc, collections)
	}

	arr := doc.collections
	unit := indentUnit(data, doc.keyIndent, arr)
	var rawEntries [][]byte
	var pathEdits []spanEdit
	present := make(map[string]bool)
	removed := 0
	for _, e := range arr.entries {
		if !e.hasName {
			// An entry without a name cannot be matched against the manifest;
			// keep it exactly as it is.
			rawEntries = append(rawEntries, data[e.value.span.start:e.value.span.end])
			continue
		}
		present[e.name] = true
		target, managed := "", false
		if e.name == depsCollection {
			if path, ok := collections[depsCollection]; ok {
				target, managed = path, true
			} else {
				removed++
				continue // "deps" unmapped in woden.toml: Woden owns it, drop it
			}
		} else if path, ok := collections[e.name]; ok {
			target, managed = path, true
		}
		raw := data[e.value.span.start:e.value.span.end]
		if managed && e.path != target {
			if !e.hasPath {
				return nil, false, errs.Userf("collection %q has no \"path\" to update", e.name)
			}
			replacement := []byte(quoteJSON(target))
			pathEdits = append(pathEdits, spanEdit{span: e.pathSpan, replacement: replacement})
			offset := e.value.span.start
			raw = splice(raw, span{
				start: e.pathSpan.start - offset,
				end:   e.pathSpan.end - offset,
			}, replacement)
		}
		rawEntries = append(rawEntries, raw)
	}

	var additions []string
	for _, name := range sortedKeys(collections) {
		if present[name] {
			continue
		}
		additions = append(additions, canonicalEntry(name, collections[name]))
	}

	changed := removed > 0 || len(additions) > 0 || len(pathEdits) > 0
	if !changed {
		return data, false, nil
	}
	if removed > 0 || len(additions) > 0 {
		// Re-rendering the array would move bytes around; refuse to do that
		// when comments live inside it, because they cannot be kept safely.
		if arr.hasComments {
			return nil, false, errs.Userf(`the "collections" array contains comments; refusing to rewrite it (remove the comments from that array first)`)
		}
		entries := make([]string, 0, len(rawEntries)+len(additions))
		for _, raw := range rawEntries {
			entries = append(entries, string(raw))
		}
		entries = append(entries, additions...)
		rendered := renderArray(entries, doc.keyIndent, unit, doc.lineEnding)
		return splice(data, arr.span, rendered), true, nil
	}
	// Only paths of existing entries changed: replace the string literals in
	// place so that every other byte (including comments) is preserved.
	return applyEdits(data, pathEdits)
}

// insertCollections adds a "collections" key to a file that has none.
func insertCollections(doc *document, collections map[string]string) ([]byte, bool, error) {
	indent := dominantIndent(doc.data)
	if indent == "" {
		indent = "  "
	}
	// One indentation unit is enough for a new key; entries are nested one
	// level deeper, which is what array entries conventionally use.
	unit := indent
	names := sortedKeys(collections)
	entries := make([]string, 0, len(names))
	for _, name := range names {
		entries = append(entries, canonicalEntry(name, collections[name]))
	}
	array := renderArray(entries, indent, unit, doc.lineEnding)
	key := []byte(`"collections": `)
	key = append(key, array...)

	closePos := doc.top.end - 1
	empty := len(doc.data) == 0 || doc.lastValueEnd == 0
	if !empty {
		// Insert right after the last field's value: the existing trailing
		// trivia (newlines, comments, one trailing comma) stays where it is.
		ins := []byte(",\n")
		ins = append(ins, indent...)
		ins = append(ins, key...)
		return insertAt(doc.data, doc.lastValueEnd, ins), true, nil
	}
	// The object has no fields, though it may contain whitespace or comments.
	ins := []byte(indent)
	ins = append(ins, key...)
	ins = append(ins, doc.lineEnding...)
	if !bytes.HasSuffix(doc.data[:closePos], []byte("\n")) {
		ins = append([]byte(doc.lineEnding), ins...)
	}
	return insertAt(doc.data, closePos, ins), true, nil
}

// canonicalEntry renders a new collections entry in Woden's canonical inline
// form. Existing entries keep their own formatting instead.
func canonicalEntry(name, path string) string {
	return `{ "name": ` + quoteJSON(name) + `, "path": ` + quoteJSON(path) + ` }`
}

// renderArray renders a collections array whose entries are the given texts.
// A single single-line entry is rendered inline, mirroring the conventional
// ols.json layout; anything else is rendered on multiple lines with the
// detected indentation.
func renderArray(entries []string, keyIndent, unit, lineEnding string) []byte {
	if len(entries) == 0 {
		return []byte("[]")
	}
	if len(entries) == 1 && !strings.ContainsAny(entries[0], "\r\n") {
		return []byte("[" + entries[0] + "]")
	}
	var b bytes.Buffer
	b.WriteByte('[')
	b.WriteString(lineEnding)
	for i, e := range entries {
		b.WriteString(keyIndent)
		b.WriteString(unit)
		b.WriteString(e)
		if i < len(entries)-1 {
			b.WriteByte(',')
		}
		b.WriteString(lineEnding)
	}
	b.WriteString(keyIndent)
	b.WriteByte(']')
	return b.Bytes()
}

// splice replaces one span of data.
func splice(data []byte, sp span, replacement []byte) []byte {
	out := make([]byte, 0, len(data)-(sp.end-sp.start)+len(replacement))
	out = append(out, data[:sp.start]...)
	out = append(out, replacement...)
	out = append(out, data[sp.end:]...)
	return out
}

// insertAt inserts replacement at offset pos.
func insertAt(data []byte, pos int, replacement []byte) []byte {
	return splice(data, span{start: pos, end: pos}, replacement)
}

// applyEdits applies non-overlapping span edits. The edits must be sorted by
// their position; they are ordered here to be independent of the caller.
func applyEdits(data []byte, edits []spanEdit) ([]byte, bool, error) {
	if len(edits) == 0 {
		return data, false, nil
	}
	sorted := make([]spanEdit, len(edits))
	copy(sorted, edits)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].span.start < sorted[j].span.start })
	for i := 1; i < len(sorted); i++ {
		if sorted[i].span.start < sorted[i-1].span.end {
			return nil, false, errs.Userf("internal error: overlapping text edits")
		}
	}
	var out bytes.Buffer
	pos := 0
	for _, e := range sorted {
		if e.span.start < pos || e.span.end > len(data) {
			return nil, false, errs.Userf("internal error: text edit outside the file")
		}
		out.Write(data[pos:e.span.start])
		out.Write(e.replacement)
		pos = e.span.end
	}
	out.Write(data[pos:])
	return out.Bytes(), true, nil
}

// sortedKeys returns the keys of m sorted alphabetically.
func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
