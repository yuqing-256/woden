package scaffold

import (
	"bytes"
	_ "embed"
)

// The project templates are embedded so a Woden binary can create projects from
// any directory without depending on the repository's testdata. The golden
// fixtures under testdata/golden/new are maintained separately on purpose: the
// tests compare generated files against them, so an accidental template change
// is caught.
//
// The templates use LF line endings. Sources are normalized with toLF when they
// are rendered, so a checkout that converted them to CRLF cannot produce
// generated files with CRLF either.

//go:embed templates/gitignore
var gitignoreTemplate []byte

//go:embed templates/woden.toml.tmpl
var wodenTOMLTemplate string

//go:embed templates/ols.json
var olsJSONTemplate []byte

//go:embed templates/src/main.odin
var mainOdinTemplate []byte

//go:embed templates/tests/main_test.odin
var mainTestOdinTemplate []byte

const (
	// manifestFileName is the name of the project configuration file. It is
	// duplicated here instead of importing internal/config so that the
	// scaffolding code stays independent of the configuration loader, but the
	// value must match config.FileName.
	manifestFileName = "woden.toml"

	// olsFileName is the name of the OLS configuration file generated at the
	// project root.
	olsFileName = "ols.json"

	// gitignoreName is the name of the ignore file generated at the project
	// root.
	gitignoreName = ".gitignore"

	// sourceDirName and testsDirName are the conventional source directories.
	sourceDirName = "src"
	testsDirName  = "tests"

	// mainOdinName and mainTestOdinName are the sample files created inside
	// sourceDirName and testsDirName.
	mainOdinName     = "main.odin"
	mainTestOdinName = "main_test.odin"

	// projectNamePlaceholder is replaced by the project name in
	// woden.toml.tmpl. Project names are validated before they reach this
	// function, so the placeholder can never be re-introduced by the name.
	projectNamePlaceholder = "{{project-name}}"
)

// gitignore returns the contents of a fresh .gitignore. The file deliberately
// has no trailing newline, matching testdata/golden/new/.gitignore.
func gitignore() []byte {
	return toLF(gitignoreTemplate)
}

// wodenTOML returns the project manifest for a project called name.
func wodenTOML(name string) []byte {
	template := toLF([]byte(wodenTOMLTemplate))
	return bytes.ReplaceAll(template, []byte(projectNamePlaceholder), []byte(name))
}

// olsJSON returns the contents of a fresh ols.json.
func olsJSON() []byte {
	return toLF(olsJSONTemplate)
}

// mainOdin returns the contents of the sample src/main.odin.
func mainOdin() []byte {
	return toLF(mainOdinTemplate)
}

// mainTestOdin returns the contents of the sample tests/main_test.odin.
func mainTestOdin() []byte {
	return toLF(mainTestOdinTemplate)
}

// toLF converts CRLF line endings to LF and returns a copy of data.
func toLF(data []byte) []byte {
	if !bytes.Contains(data, []byte("\r\n")) {
		return data
	}
	return bytes.ReplaceAll(data, []byte("\r\n"), []byte("\n"))
}
