// Package validate checks every user-provided name, path, URL and reference
// before Woden uses it.
//
// The rules implemented here are the ones documented in README.md; when in
// doubt the conservative choice (reject) is taken, because a rejected value
// gives the user a clear error while a malicious value can escape the project.
package validate

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
	"unicode"
)

var (
	commitIDPattern = regexp.MustCompile(`^[0-9a-fA-F]{40}$`)
	revPattern      = regexp.MustCompile(`^[0-9a-fA-F]{7,40}$`)
	scpURLPattern   = regexp.MustCompile(`^[A-Za-z0-9._-]+@[A-Za-z0-9._-]+:`)
	windowsDrive    = regexp.MustCompile(`^[A-Za-z]:[\\/]`)
)

// windowsReservedNames are the device names Windows refuses to use as file
// names, with or without an extension and in any case.
var windowsReservedNames = map[string]bool{
	"CON": true, "PRN": true, "AUX": true, "NUL": true,
	"COM1": true, "COM2": true, "COM3": true, "COM4": true, "COM5": true,
	"COM6": true, "COM7": true, "COM8": true, "COM9": true,
	"LPT1": true, "LPT2": true, "LPT3": true, "LPT4": true, "LPT5": true,
	"LPT6": true, "LPT7": true, "LPT8": true, "LPT9": true,
}

// reservedDeviceName reports whether name (or name.ext) is reserved on Windows.
func reservedDeviceName(name string) bool {
	base := name
	if i := strings.IndexByte(base, '.'); i >= 0 {
		base = base[:i]
	}
	base = strings.TrimRight(base, " ")
	return windowsReservedNames[strings.ToUpper(base)]
}

// ProjectName validates the name used for [package] name and the project
// directory of "woden new".
func ProjectName(name string) error {
	if name == "" {
		return errors.New("name must not be empty")
	}
	if !isLetter(rune(name[0])) && name[0] != '_' {
		return fmt.Errorf("name %q must start with a letter or an underscore", name)
	}
	for _, r := range name[1:] {
		if !isLetter(r) && !isDigit(r) && r != '-' && r != '_' {
			return fmt.Errorf("name %q may only contain letters, digits, '-' and '_'", name)
		}
	}
	if reservedDeviceName(name) {
		return fmt.Errorf("name %q is a reserved Windows device name", name)
	}
	return nil
}

// DependencyName validates the name of a Git dependency. The name becomes a
// directory inside vendor/ and a key in woden.toml/woden.lock, so it must be a
// safe single path element.
func DependencyName(name string) error {
	if name == "" {
		return errors.New("dependency name must not be empty")
	}
	if name == "." || name == ".." {
		return fmt.Errorf("dependency name %q is not a valid directory name", name)
	}
	for _, r := range name {
		switch {
		case r == 0 || unicode.IsControl(r):
			return fmt.Errorf("dependency name %q must not contain control characters", name)
		case unicode.IsSpace(r):
			return fmt.Errorf("dependency name %q must not contain whitespace", name)
		case r == '/' || r == '\\' || r == ':':
			return fmt.Errorf("dependency name %q must not contain path separators or ':'", name)
		}
	}
	if strings.HasPrefix(name, "-") {
		return fmt.Errorf("dependency name %q must not start with '-'", name)
	}
	if strings.HasSuffix(name, ".") || strings.HasSuffix(name, " ") {
		return fmt.Errorf("dependency name %q must not end with '.' or a space", name)
	}
	if reservedDeviceName(name) {
		return fmt.Errorf("dependency name %q is a reserved Windows device name", name)
	}
	if strings.EqualFold(name, ".git") {
		return fmt.Errorf("dependency name %q is reserved", name)
	}
	return nil
}

// CollectionName validates a key of the [collections] table.
func CollectionName(name string) error {
	if name == "" {
		return errors.New("collection name must not be empty")
	}
	switch name {
	case "core", "base", "vendor":
		return fmt.Errorf("collection name %q is reserved by Odin", name)
	}
	for _, r := range name {
		switch {
		case r == 0 || unicode.IsControl(r):
			return fmt.Errorf("collection name %q must not contain control characters", name)
		case unicode.IsSpace(r):
			return fmt.Errorf("collection name %q must not contain whitespace", name)
		case r == '/' || r == '\\' || r == ':' || r == '=':
			return fmt.Errorf("collection name %q must not contain %q", name, string(r))
		}
	}
	if strings.HasPrefix(name, "-") {
		return fmt.Errorf("collection name %q must not start with '-'", name)
	}
	return nil
}

// CollectionPath validates the path of a collection: a relative path that
// stays inside the project root.
func CollectionPath(path string) error {
	if path == "" {
		return errors.New("collection path must not be empty")
	}
	for _, r := range path {
		if r == 0 || unicode.IsControl(r) {
			return fmt.Errorf("collection path %q must not contain control characters", path)
		}
	}
	// Validate with both separators so the same configuration is accepted on
	// every platform.
	slashed := strings.ReplaceAll(path, "\\", "/")
	if strings.HasPrefix(slashed, "/") || windowsDrive.MatchString(path) {
		return fmt.Errorf("collection path %q must be relative", path)
	}
	if strings.Contains(slashed, ":") {
		return fmt.Errorf("collection path %q must not contain ':'", path)
	}
	for _, segment := range strings.Split(slashed, "/") {
		if segment == ".." {
			return fmt.Errorf("collection path %q must stay inside the project root", path)
		}
	}
	return nil
}

// GitURL validates the URL of a Git dependency. Only https, ssh and git URLs
// and scp-like addresses are accepted. The file:// scheme is only available
// when Woden is built with the woden_testing tag, so a released binary can
// never be tricked into reading a local repository.
func GitURL(raw string) error {
	if raw == "" {
		return errors.New("Git URL must not be empty")
	}
	if strings.HasPrefix(raw, "-") {
		return fmt.Errorf("Git URL %q must not start with '-'", raw)
	}
	for _, r := range raw {
		if r == 0 || r == '\n' || r == '\r' || unicode.IsControl(r) {
			return fmt.Errorf("Git URL %q must not contain control characters", raw)
		}
		if unicode.IsSpace(r) {
			return fmt.Errorf("Git URL %q must not contain whitespace", raw)
		}
	}
	if strings.HasPrefix(raw, "ext::") {
		return fmt.Errorf("Git URL %q uses the forbidden ext:: transport", raw)
	}
	if windowsDrive.MatchString(raw) || strings.HasPrefix(raw, "\\\\") {
		return fmt.Errorf("Git URL %q must not be a filesystem path", raw)
	}
	for _, scheme := range []string{"https://", "ssh://", "git://"} {
		if strings.HasPrefix(raw, scheme) {
			host := strings.TrimPrefix(raw, scheme)
			if i := strings.IndexAny(host, "/?#"); i >= 0 {
				host = host[:i]
			}
			if host == "" {
				return fmt.Errorf("Git URL %q has no host", raw)
			}
			return nil
		}
	}
	if strings.HasPrefix(raw, "file://") {
		if !fileURLAllowed() {
			return fmt.Errorf("Git URL %q uses file://, which is only available in tests", raw)
		}
		return nil
	}
	if scpURLPattern.MatchString(raw) {
		return nil
	}
	return fmt.Errorf("Git URL %q must be an https, ssh or git URL or an scp-like address", raw)
}

// RefName validates a branch or tag name. The value is passed to Git as a
// complete ref (refs/heads/… or refs/tags/…), so Git's ref name rules are
// enforced here as well.
func RefName(ref string) error {
	if ref == "" {
		return errors.New("reference name must not be empty")
	}
	if strings.HasPrefix(ref, "-") {
		return fmt.Errorf("reference name %q must not start with '-'", ref)
	}
	if ref == "@" || strings.Contains(ref, "@{") {
		return fmt.Errorf("reference name %q is not a valid Git reference", ref)
	}
	if strings.HasPrefix(ref, "/") || strings.HasPrefix(ref, ".") {
		return fmt.Errorf("reference name %q must not start with '/' or '.'", ref)
	}
	if strings.HasSuffix(ref, "/") || strings.HasSuffix(ref, ".") {
		return fmt.Errorf("reference name %q must not end with '/' or '.'", ref)
	}
	if strings.Contains(ref, "..") || strings.Contains(ref, "//") {
		return fmt.Errorf("reference name %q must not contain '..' or '//'", ref)
	}
	for _, r := range ref {
		if unicode.IsSpace(r) || unicode.IsControl(r) {
			return fmt.Errorf("reference name %q must not contain whitespace or control characters", ref)
		}
		if strings.ContainsRune("~^:?*[\\", r) {
			return fmt.Errorf("reference name %q contains the forbidden character %q", ref, string(r))
		}
	}
	if strings.HasSuffix(ref, ".lock") {
		return fmt.Errorf("reference name %q must not end with '.lock'", ref)
	}
	return nil
}

// Rev validates the value of add --rev: a full commit ID or an abbreviation of
// at least seven hexadecimal digits.
func Rev(rev string) error {
	if rev == "" {
		return errors.New("revision must not be empty")
	}
	if !revPattern.MatchString(rev) {
		return fmt.Errorf("revision %q must be a full or abbreviated (at least 7 hexadecimal digits) commit ID", rev)
	}
	return nil
}

// CommitID validates a full 40 character hexadecimal commit ID.
func CommitID(id string) error {
	if !commitIDPattern.MatchString(id) {
		return fmt.Errorf("commit ID %q must be 40 hexadecimal characters", id)
	}
	return nil
}

func isLetter(r rune) bool {
	return (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z')
}

func isDigit(r rune) bool {
	return r >= '0' && r <= '9'
}
