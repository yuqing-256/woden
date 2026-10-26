package ols

import (
	"bytes"
	"encoding/json"
	"regexp"
	"strings"

	"github.com/yuqing-256/woden/internal/errs"
)

// span is a half-open byte range [start,end) inside an ols.json file.
type span struct {
	start int
	end   int
}

// valueKind classifies a scanned JSON value.
type valueKind int

const (
	kindObject valueKind = iota
	kindArray
	kindString
	kindLiteral
)

// jsonValue is a scanned JSON value together with the byte range it occupies.
type jsonValue struct {
	span span
	kind valueKind
}

// field is one member of a JSON object.
type field struct {
	key      string
	keyStart int
	value    jsonValue
}

// entry is one object inside the top-level "collections" array.
type entry struct {
	value    jsonValue
	name     string
	hasName  bool
	path     string
	hasPath  bool
	pathSpan span // byte range of the "path" string literal, valid when hasPath
}

// collectionArray is the top-level "collections" setting.
type collectionArray struct {
	span        span
	entries     []entry
	hasComments bool
}

// document is the parsed subset of an ols.json file that Woden edits.
//
// The rest of the file is never interpreted: it is kept as raw bytes so that
// formatting, ordering, comments and unknown settings survive a merge.
type document struct {
	data         []byte
	lineEnding   string
	top          span // top-level JSON object
	lastValueEnd int  // byte offset just past the last top-level field's value
	collections  *collectionArray
	keyIndent    string // indentation of the line holding the "collections" key
}

// parseDocument scans enough of an ols.json file (which OLS allows to contain
// comments and, here and there, trailing commas) to locate the top-level
// "collections" array and to validate the overall structure of the file. It
// deliberately does not require a full JSONC parser.
func parseDocument(data []byte) (*document, error) {
	doc := &document{data: data, lineEnding: detectLineEnding(data)}
	start := 0
	if bytes.HasPrefix(data, []byte{0xEF, 0xBB, 0xBF}) {
		start = 3
	}
	i, err := skipTrivia(data, start)
	if err != nil {
		return nil, err
	}
	if i >= len(data) {
		return nil, errs.Userf("the file is empty")
	}
	if data[i] != '{' {
		return nil, errs.Userf("the top-level JSON value must be an object")
	}
	end, fields, err := parseObject(data, i)
	if err != nil {
		return nil, err
	}
	doc.top = span{start: i, end: end}
	for _, f := range fields {
		if f.value.span.end > doc.lastValueEnd {
			doc.lastValueEnd = f.value.span.end
		}
	}
	if i, err = skipTrivia(data, end); err != nil {
		return nil, err
	}
	if i < len(data) {
		return nil, errs.Userf("unexpected content after the top-level JSON object")
	}
	if f, ok := fields["collections"]; ok {
		if f.value.kind != kindArray {
			return nil, errs.Userf(`the "collections" setting must be an array`)
		}
		arr, err := parseCollections(data, f.value)
		if err != nil {
			return nil, err
		}
		doc.collections = arr
		doc.keyIndent = lineIndent(data, f.keyStart)
	}
	return doc, nil
}

// parseObject scans the JSON object starting at start. It returns the byte
// offset just past the closing brace and the fields of the object itself. It
// validates the whole value recursively, so malformed files are rejected even
// when the problem is in a setting Woden does not manage.
func parseObject(data []byte, start int) (int, map[string]field, error) {
	fields := make(map[string]field)
	i := start + 1
	for {
		var err error
		i, err = skipTrivia(data, i)
		if err != nil {
			return 0, nil, err
		}
		if i >= len(data) {
			return 0, nil, errs.Userf("unterminated JSON object")
		}
		if data[i] == '}' {
			return i + 1, fields, nil
		}
		if data[i] != '"' {
			return 0, nil, errs.Userf("expected a JSON object key")
		}
		keySpan, next, err := scanString(data, i)
		if err != nil {
			return 0, nil, err
		}
		key, err := decodeString(data[keySpan.start:keySpan.end])
		if err != nil {
			return 0, nil, err
		}
		if _, dup := fields[key]; dup {
			return 0, nil, errs.Userf("duplicate object key %q", key)
		}
		i = next
		if i, err = skipTrivia(data, i); err != nil {
			return 0, nil, err
		}
		if i >= len(data) || data[i] != ':' {
			return 0, nil, errs.Userf(`expected ":" after object key %q`, key)
		}
		i, err = skipTrivia(data, i+1)
		if err != nil {
			return 0, nil, err
		}
		value, next, err := scanValue(data, i)
		if err != nil {
			return 0, nil, err
		}
		fields[key] = field{key: key, keyStart: keySpan.start, value: value}
		i = next
		if i, err = skipTrivia(data, i); err != nil {
			return 0, nil, err
		}
		if i >= len(data) {
			return 0, nil, errs.Userf("unterminated JSON object")
		}
		switch data[i] {
		case ',':
			i++
		case '}':
			return i + 1, fields, nil
		default:
			return 0, nil, errs.Userf(`expected "," or "}" in a JSON object`)
		}
	}
}

// parseCollections scans the entries of the top-level "collections" array.
func parseCollections(data []byte, arr jsonValue) (*collectionArray, error) {
	out := &collectionArray{span: arr.span}
	hasComments, err := containsComment(data, arr.span)
	if err != nil {
		return nil, err
	}
	out.hasComments = hasComments
	seen := make(map[string]bool)
	i := arr.span.start + 1
	for {
		i, err = skipTrivia(data, i)
		if err != nil {
			return nil, err
		}
		if i >= len(data) {
			return nil, errs.Userf(`unterminated "collections" array`)
		}
		if data[i] == ']' {
			return out, nil
		}
		if data[i] != '{' {
			return nil, errs.Userf(`every "collections" entry must be a JSON object`)
		}
		end, fields, err := parseObject(data, i)
		if err != nil {
			return nil, err
		}
		e := entry{value: jsonValue{span: span{start: i, end: end}, kind: kindObject}}
		if name, ok := fields["name"]; ok {
			if name.value.kind != kindString {
				return nil, errs.Userf(`"name" in a "collections" entry must be a string`)
			}
			value, err := decodeString(data[name.value.span.start:name.value.span.end])
			if err != nil {
				return nil, err
			}
			if seen[value] {
				return nil, errs.Userf("duplicate collection name %q in the \"collections\" array", value)
			}
			seen[value] = true
			e.name, e.hasName = value, true
		}
		if path, ok := fields["path"]; ok {
			if path.value.kind != kindString {
				return nil, errs.Userf(`"path" in a "collections" entry must be a string`)
			}
			value, err := decodeString(data[path.value.span.start:path.value.span.end])
			if err != nil {
				return nil, err
			}
			e.path, e.hasPath, e.pathSpan = value, true, path.value.span
		}
		out.entries = append(out.entries, e)
		i = end
		i, err = skipTrivia(data, i)
		if err != nil {
			return nil, err
		}
		if i >= len(data) {
			return nil, errs.Userf(`unterminated "collections" array`)
		}
		switch data[i] {
		case ',':
			i++
		case ']':
			return out, nil
		default:
			return nil, errs.Userf(`expected "," or "]" after a "collections" entry`)
		}
	}
}

// scanValue scans one JSON value and returns it together with the byte offset
// just past it.
func scanValue(data []byte, i int) (jsonValue, int, error) {
	if i >= len(data) {
		return jsonValue{}, 0, errs.Userf("unexpected end of file")
	}
	switch data[i] {
	case '{':
		end, _, err := parseObject(data, i)
		if err != nil {
			return jsonValue{}, 0, err
		}
		return jsonValue{span: span{start: i, end: end}, kind: kindObject}, end, nil
	case '[':
		end, err := scanArray(data, i)
		if err != nil {
			return jsonValue{}, 0, err
		}
		return jsonValue{span: span{start: i, end: end}, kind: kindArray}, end, nil
	case '"':
		sp, next, err := scanString(data, i)
		if err != nil {
			return jsonValue{}, 0, err
		}
		return jsonValue{span: sp, kind: kindString}, next, nil
	default:
		return scanLiteral(data, i)
	}
}

// scanArray scans a JSON array whose top-level structure is not otherwise
// interesting to Woden (for example "profiles").
func scanArray(data []byte, start int) (int, error) {
	i := start + 1
	for {
		var err error
		i, err = skipTrivia(data, i)
		if err != nil {
			return 0, err
		}
		if i >= len(data) {
			return 0, errs.Userf("unterminated JSON array")
		}
		if data[i] == ']' {
			return i + 1, nil
		}
		if _, next, err := scanValue(data, i); err != nil {
			return 0, err
		} else {
			i = next
		}
		i, err = skipTrivia(data, i)
		if err != nil {
			return 0, err
		}
		if i >= len(data) {
			return 0, errs.Userf("unterminated JSON array")
		}
		switch data[i] {
		case ',':
			i++
		case ']':
			return i + 1, nil
		default:
			return 0, errs.Userf(`expected "," or "]" in a JSON array`)
		}
	}
}

var numberPattern = regexp.MustCompile(`^-?(0|[1-9][0-9]*)(\.[0-9]+)?([eE][+-]?[0-9]+)?$`)

// scanLiteral scans a JSON literal: true, false, null or a number.
func scanLiteral(data []byte, start int) (jsonValue, int, error) {
	i := start
	for i < len(data) && !isValueDelimiter(data[i]) {
		i++
	}
	if i == start {
		return jsonValue{}, 0, errs.Userf("unexpected character %q", data[start])
	}
	token := string(data[start:i])
	if token != "true" && token != "false" && token != "null" && !numberPattern.MatchString(token) {
		return jsonValue{}, 0, errs.Userf("invalid JSON value %q", token)
	}
	return jsonValue{span: span{start: start, end: i}, kind: kindLiteral}, i, nil
}

func isValueDelimiter(b byte) bool {
	switch b {
	case ' ', '\t', '\r', '\n', ',', ']', '}':
		return true
	}
	return false
}

// scanString scans a JSON string literal and validates its escape sequences.
func scanString(data []byte, start int) (span, int, error) {
	i := start + 1
	for i < len(data) {
		switch data[i] {
		case '\\':
			if i+1 >= len(data) {
				return span{}, 0, errs.Userf("unterminated escape sequence in a string")
			}
			switch data[i+1] {
			case '"', '\\', '/', 'b', 'f', 'n', 'r', 't':
				i += 2
			case 'u':
				if i+6 > len(data) {
					return span{}, 0, errs.Userf("unterminated \\u escape sequence in a string")
				}
				for k := i + 2; k < i+6; k++ {
					if !isHexDigit(data[k]) {
						return span{}, 0, errs.Userf("invalid \\u escape sequence in a string")
					}
				}
				i += 6
			default:
				return span{}, 0, errs.Userf("invalid escape sequence \\%c in a string", data[i+1])
			}
		case '"':
			return span{start: start, end: i + 1}, i + 1, nil
		default:
			if data[i] < 0x20 {
				return span{}, 0, errs.Userf("unescaped control character in a string")
			}
			i++
		}
	}
	return span{}, 0, errs.Userf("unterminated string")
}

func isHexDigit(b byte) bool {
	switch {
	case b >= '0' && b <= '9':
		return true
	case b >= 'a' && b <= 'f':
		return true
	case b >= 'A' && b <= 'F':
		return true
	}
	return false
}

// skipTrivia skips whitespace and comments.
func skipTrivia(data []byte, i int) (int, error) {
	for i < len(data) {
		switch data[i] {
		case ' ', '\t', '\r', '\n':
			i++
		case '/':
			if i+1 >= len(data) {
				return i, errs.Userf("the file ends inside a comment")
			}
			switch data[i+1] {
			case '/':
				i += 2
				for i < len(data) && data[i] != '\n' {
					i++
				}
			case '*':
				offset := bytes.Index(data[i+2:], []byte("*/"))
				if offset < 0 {
					return i, errs.Userf("the file ends inside a block comment")
				}
				i += 2 + offset + 2
			default:
				return i, errs.Userf("unexpected character %q", data[i])
			}
		default:
			return i, nil
		}
	}
	return i, nil
}

// containsComment reports whether sp contains a comment token outside strings.
func containsComment(data []byte, sp span) (bool, error) {
	for i := sp.start; i < sp.end; {
		if data[i] == '"' {
			_, next, err := scanString(data, i)
			if err != nil {
				return false, err
			}
			i = next
			continue
		}
		if data[i] == '/' && i+1 < sp.end && (data[i+1] == '/' || data[i+1] == '*') {
			return true, nil
		}
		i++
	}
	return false, nil
}

// decodeString decodes a JSON string literal, including its surrounding quotes.
func decodeString(raw []byte) (string, error) {
	var value string
	if err := json.Unmarshal(raw, &value); err != nil {
		return "", errs.Userf("invalid JSON string literal: %v", err)
	}
	return value, nil
}

// quoteJSON encodes s as a JSON string literal without HTML escaping.
func quoteJSON(s string) string {
	var b strings.Builder
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(s); err != nil {
		return "null"
	}
	return strings.TrimSuffix(b.String(), "\n")
}

// detectLineEnding returns the line ending style of the file.
func detectLineEnding(data []byte) string {
	if i := bytes.IndexByte(data, '\n'); i > 0 && data[i-1] == '\r' {
		return "\r\n"
	}
	return "\n"
}

// lineIndent returns the whitespace between the start of the line holding pos
// and pos itself, or "" when pos is not the first non-whitespace content of its
// line.
func lineIndent(data []byte, pos int) string {
	start := bytes.LastIndexByte(data[:pos], '\n') + 1
	indent := data[start:pos]
	for _, b := range indent {
		if b != ' ' && b != '\t' {
			return ""
		}
	}
	return string(indent)
}

// dominantIndent returns the shortest indentation of a non-empty line in the
// file, i.e. one indentation unit of the file's style. It returns "" when the
// file has no indented lines.
func dominantIndent(data []byte) string {
	best := ""
	for _, line := range bytes.Split(data, []byte("\n")) {
		line = bytes.TrimSuffix(line, []byte("\r"))
		n := 0
		for n < len(line) && (line[n] == ' ' || line[n] == '\t') {
			n++
		}
		if n == 0 || n == len(line) {
			continue
		}
		indent := string(line[:n])
		if best == "" || len(indent) < len(best) {
			best = indent
		}
	}
	return best
}

// indentUnit returns one indentation unit of the file. Existing entries of the
// collections array are the best source; otherwise the file's dominant style
// is used. The default is two spaces.
func indentUnit(data []byte, keyIndent string, arr *collectionArray) string {
	if arr != nil {
		for _, e := range arr.entries {
			indent := lineIndent(data, e.value.span.start)
			if indent == "" {
				continue // the entry does not start its own line
			}
			if keyIndent != "" && !strings.HasPrefix(indent, keyIndent) {
				continue
			}
			if unit := indent[len(keyIndent):]; unit != "" {
				return unit
			}
		}
	}
	if indent := dominantIndent(data); indent != "" {
		return indent
	}
	return "  "
}
