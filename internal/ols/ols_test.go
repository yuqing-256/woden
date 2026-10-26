package ols_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"woden/internal/errs"
	"woden/internal/ols"
	"woden/internal/testutil"
)

// fixture returns the bytes of a file in testdata/ols/.
func fixture(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(testutil.Testdata(t, filepath.Join("ols", name)))
	if err != nil {
		t.Fatalf("cannot read fixture %s: %v", name, err)
	}
	return data
}

// projectWithConfig creates a temporary project root whose ols.json contains
// data and returns the root.
func projectWithConfig(t *testing.T, data []byte) string {
	t.Helper()
	root := t.TempDir()
	writeConfig(t, root, data)
	return root
}

func writeConfig(t *testing.T, root string, data []byte) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(root, ols.FileName), data, 0o644); err != nil {
		t.Fatalf("cannot write ols.json: %v", err)
	}
}

func readConfig(t *testing.T, root string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(root, ols.FileName))
	if err != nil {
		t.Fatalf("cannot read ols.json: %v", err)
	}
	return data
}

func assertBytes(t *testing.T, got, want []byte) {
	t.Helper()
	if bytes.Equal(got, want) {
		return
	}
	t.Fatalf("unexpected ols.json content:\n--- got ---\n%s\n--- want ---\n%s", got, want)
}

// TestSyncFixtures checks the byte-exact merge targets documented in
// docs/FIXTURES.md.
func TestSyncFixtures(t *testing.T) {
	cases := []struct {
		name        string
		in          string
		collections map[string]string
		want        string
	}{
		{
			name:        "add deps collection",
			in:          "base.json",
			collections: map[string]string{"app": "src", "deps": "vendor"},
			want:        "with-deps.json",
		},
		{
			name:        "remove deps collection",
			in:          "with-deps.json",
			collections: map[string]string{"app": "src"},
			want:        "base.json",
		},
		{
			name:        "update stale path",
			in:          "stale-app.json",
			collections: map[string]string{"app": "src"},
			want:        "base.json",
		},
		{
			name:        "custom file keeps its settings",
			in:          "custom.json",
			collections: map[string]string{"app": "src", "libs": "libs"},
			want:        "custom.add-libs.want.json",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := projectWithConfig(t, fixture(t, tc.in))
			before := readConfig(t, root)
			changed, exists, err := ols.Sync(root, tc.collections)
			if err != nil {
				t.Fatalf("Sync: %v", err)
			}
			if !exists {
				t.Fatal("Sync: exists = false, want true")
			}
			if !changed {
				t.Fatal("Sync: changed = false, want true")
			}
			assertBytes(t, readConfig(t, root), fixture(t, tc.want))

			// A second sync must be a no-op on the already merged content.
			changed, exists, err = ols.Sync(root, tc.collections)
			if err != nil || !exists || changed {
				t.Fatalf("second Sync = (%v, %v, %v), want (false, true, nil)", changed, exists, err)
			}
			assertBytes(t, readConfig(t, root), fixture(t, tc.want))
			if bytes.Equal(before, fixture(t, tc.want)) {
				t.Fatal("test fixture is not a change; the case does not test a merge")
			}
		})
	}
}

// TestSyncUnchanged checks that a file already holding the managed mappings is
// never touched.
func TestSyncUnchanged(t *testing.T) {
	cases := []struct {
		name        string
		in          string
		collections map[string]string
	}{
		{"base is consistent", "base.json", map[string]string{"app": "src"}},
		{"custom is consistent", "custom.json", map[string]string{"app": "src"}},
		{"comments do not block a no-op", "with-comments.json", map[string]string{"app": "src"}},
		{"deps entry absent and unmapped", "base.json", nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := projectWithConfig(t, fixture(t, tc.in))
			changed, exists, err := ols.Sync(root, tc.collections)
			if err != nil {
				t.Fatalf("Sync: %v", err)
			}
			if changed {
				t.Fatal("Sync: changed = true, want false")
			}
			if !exists {
				t.Fatal("Sync: exists = false, want true")
			}
			assertBytes(t, readConfig(t, root), fixture(t, tc.in))
		})
	}
}

// TestSyncCommentsOutsideArray checks that a comment outside the managed array
// never blocks a merge and is never lost.
func TestSyncCommentsOutsideArray(t *testing.T) {
	root := projectWithConfig(t, fixture(t, "with-comments.json"))
	changed, exists, err := ols.Sync(root, map[string]string{"app": "src", "deps": "vendor"})
	if err != nil || !exists || !changed {
		t.Fatalf("Sync = (%v, %v, %v), want (true, true, nil)", changed, exists, err)
	}
	want := withComment(fixture(t, "with-deps.json"), "  // note")
	assertBytes(t, readConfig(t, root), want)
}

// withComment inserts a comment line after the first two lines of a pretty
// ols.json fixture, the way with-comments.json relates to base.json.
func withComment(data []byte, comment string) []byte {
	lines := strings.SplitAfter(string(data), "\n")
	if len(lines) < 3 {
		return data
	}
	var b strings.Builder
	b.WriteString(lines[0])
	b.WriteString(lines[1])
	b.WriteString(comment)
	b.WriteString("\n")
	b.WriteString(strings.Join(lines[2:], ""))
	return []byte(b.String())
}

// TestSyncMissingFile checks that a missing ols.json is never created.
func TestSyncMissingFile(t *testing.T) {
	root := t.TempDir()
	collections := map[string]string{"app": "src", "deps": "vendor"}
	changed, exists, err := ols.Sync(root, collections)
	if err != nil || changed || exists {
		t.Fatalf("Sync = (%v, %v, %v), want (false, false, nil)", changed, exists, err)
	}
	if _, err := os.Stat(filepath.Join(root, ols.FileName)); !os.IsNotExist(err) {
		t.Fatalf("Sync created ols.json, stat error = %v", err)
	}

	consistent, exists, err := ols.Check(root, collections)
	if err != nil || consistent || exists {
		t.Fatalf("Check = (%v, %v, %v), want (false, false, nil)", consistent, exists, err)
	}
}

// TestSyncEmptyProjectRoot checks argument validation.
func TestSyncEmptyProjectRoot(t *testing.T) {
	if _, _, err := ols.Sync("", map[string]string{"app": "src"}); err == nil || !errs.IsUser(err) {
		t.Fatalf("Sync with empty root: err = %v, want user error", err)
	}
}

// TestSyncBadSyntax checks that a malformed ols.json is reported and left
// byte-for-byte untouched.
func TestSyncBadSyntax(t *testing.T) {
	root := projectWithConfig(t, fixture(t, "bad-syntax.json"))
	before := readConfig(t, root)
	changed, exists, err := ols.Sync(root, map[string]string{"app": "src", "deps": "vendor"})
	if err == nil {
		t.Fatal("Sync: err = nil, want an error")
	}
	if !errs.IsUser(err) {
		t.Fatalf("Sync: err = %v, want a user error", err)
	}
	if changed || !exists {
		t.Fatalf("Sync = (%v, %v), want (false, true)", changed, exists)
	}
	assertBytes(t, readConfig(t, root), before)
}

// TestSyncTabsIndentation checks that a file indented with tabs gets a tab
// indented entry. testdata/ols/tabs.json currently contains spaces, so the
// tab-indented file is built here.
func TestSyncTabsIndentation(t *testing.T) {
	const input = "{\n" +
		"\t\"$schema\": \"https://raw.githubusercontent.com/DanielGavin/ols/master/misc/ols.schema.json\",\n" +
		"\t\"collections\": [\n" +
		"\t\t{ \"name\": \"app\", \"path\": \"src\" }\n" +
		"\t],\n" +
		"\t\"checker_args\": \"-vet\"\n" +
		"}\n"
	const want = "{\n" +
		"\t\"$schema\": \"https://raw.githubusercontent.com/DanielGavin/ols/master/misc/ols.schema.json\",\n" +
		"\t\"collections\": [\n" +
		"\t\t{ \"name\": \"app\", \"path\": \"src\" },\n" +
		"\t\t{ \"name\": \"libs\", \"path\": \"libs\" }\n" +
		"\t],\n" +
		"\t\"checker_args\": \"-vet\"\n" +
		"}\n"
	root := projectWithConfig(t, []byte(input))
	changed, exists, err := ols.Sync(root, map[string]string{"app": "src", "libs": "libs"})
	if err != nil || !exists || !changed {
		t.Fatalf("Sync = (%v, %v, %v), want (true, true, nil)", changed, exists, err)
	}
	assertBytes(t, readConfig(t, root), []byte(want))
}

// TestSyncCRLFPreserved checks that the line ending style of the file is
// preserved.
func TestSyncCRLFPreserved(t *testing.T) {
	input := testutil.ToCRLF(string(fixture(t, "base.json")))
	root := projectWithConfig(t, []byte(input))
	changed, _, err := ols.Sync(root, map[string]string{"app": "src", "deps": "vendor"})
	if err != nil || !changed {
		t.Fatalf("Sync = (%v, %v), want (true, nil)", changed, err)
	}
	want := testutil.ToCRLF(string(fixture(t, "with-deps.json")))
	assertBytes(t, readConfig(t, root), []byte(want))
}

// TestSyncMissingCollectionsKey checks that a file without a "collections"
// setting gets one instead of failing forever.
func TestSyncMissingCollectionsKey(t *testing.T) {
	const input = "{\n  \"checker_args\": \"-vet\"\n}\n"
	const want = "{\n" +
		"  \"checker_args\": \"-vet\",\n" +
		"  \"collections\": [\n" +
		"    { \"name\": \"app\", \"path\": \"src\" },\n" +
		"    { \"name\": \"libs\", \"path\": \"libs\" }\n" +
		"  ]\n" +
		"}\n"
	root := projectWithConfig(t, []byte(input))
	changed, exists, err := ols.Sync(root, map[string]string{"app": "src", "libs": "libs"})
	if err != nil || !exists || !changed {
		t.Fatalf("Sync = (%v, %v, %v), want (true, true, nil)", changed, exists, err)
	}
	assertBytes(t, readConfig(t, root), []byte(want))
}

// TestSyncEmptyObject checks the "collections" insertion into an empty object.
func TestSyncEmptyObject(t *testing.T) {
	root := projectWithConfig(t, []byte("{}"))
	changed, _, err := ols.Sync(root, map[string]string{"app": "src"})
	if err != nil || !changed {
		t.Fatalf("Sync = (%v, %v), want (true, nil)", changed, err)
	}
	want := "{\n  \"collections\": [{ \"name\": \"app\", \"path\": \"src\" }]\n}"
	assertBytes(t, readConfig(t, root), []byte(want))
}

// TestSyncEmptyManifestRemovesDeps checks that "deps" is deleted even when the
// manifest has no collections at all.
func TestSyncEmptyManifestRemovesDeps(t *testing.T) {
	root := projectWithConfig(t, fixture(t, "base.json"))
	changed, _, err := ols.Sync(root, map[string]string{"app": "src", "deps": "vendor"})
	if err != nil || !changed {
		t.Fatalf("Sync = (%v, %v), want (true, nil)", changed, err)
	}
	// A nil map still means "woden.toml does not map deps".
	changed, _, err = ols.Sync(root, nil)
	if err != nil || !changed {
		t.Fatalf("second Sync = (%v, %v), want (true, nil)", changed, err)
	}
	assertBytes(t, readConfig(t, root), fixture(t, "base.json"))
}

// TestSyncCommentsInsideArray checks the comment policy: comments inside the
// managed array allow a minimal path update but block adding or deleting
// entries, because that requires re-rendering the array.
func TestSyncCommentsInsideArray(t *testing.T) {
	const updateInput = "{\n" +
		"  \"collections\": [\n" +
		"    { \"name\": \"app\", \"path\": \"old-src\" } // keep this comment\n" +
		"  ],\n" +
		"  \"checker_args\": \"-vet\"\n" +
		"}\n"
	t.Run("path update preserves the comment", func(t *testing.T) {
		root := projectWithConfig(t, []byte(updateInput))
		changed, _, err := ols.Sync(root, map[string]string{"app": "src"})
		if err != nil || !changed {
			t.Fatalf("Sync = (%v, %v), want (true, nil)", changed, err)
		}
		want := strings.Replace(updateInput, `"path": "old-src"`, `"path": "src"`, 1)
		assertBytes(t, readConfig(t, root), []byte(want))
	})
	t.Run("adding an entry is refused", func(t *testing.T) {
		root := projectWithConfig(t, []byte(updateInput))
		before := readConfig(t, root)
		changed, exists, err := ols.Sync(root, map[string]string{"app": "old-src", "libs": "libs"})
		if err == nil || !errs.IsUser(err) {
			t.Fatalf("Sync: err = %v, want a user error", err)
		}
		if changed || !exists {
			t.Fatalf("Sync = (%v, %v), want (false, true)", changed, exists)
		}
		assertBytes(t, readConfig(t, root), before)
	})
	t.Run("deleting deps is refused", func(t *testing.T) {
		input := "{\n" +
			"  \"collections\": [\n" +
			"    { \"name\": \"deps\", \"path\": \"vendor\" } // managed\n" +
			"  ]\n" +
			"}\n"
		root := projectWithConfig(t, []byte(input))
		before := readConfig(t, root)
		if _, _, err := ols.Sync(root, map[string]string{}); err == nil || !errs.IsUser(err) {
			t.Fatalf("Sync: err = %v, want a user error", err)
		}
		assertBytes(t, readConfig(t, root), before)
	})
}

// TestSyncUnsafeLayouts checks that layouts Woden cannot edit safely are
// rejected without touching the file.
func TestSyncUnsafeLayouts(t *testing.T) {
	cases := []struct {
		name string
		data string
	}{
		{"collections is not an array", `{"collections": {}}`},
		{"collection entry is not an object", `{"collections": [1]}`},
		{"duplicate collection name", `{"collections": [{"name": "a", "path": "x"}, {"name": "a", "path": "y"}]}`},
		{"name is not a string", `{"collections": [{"name": 1, "path": "x"}]}`},
		{"path is not a string", `{"collections": [{"name": "a", "path": 1}]}`},
		{"top level is not an object", `[]`},
		{"unterminated array", `{"collections": [`},
		{"unterminated object", `{"checker_args": "-vet"`},
		{"invalid comment", `{"checker_args": /}`},
		{"duplicate top level key", `{"collections": [], "collections": []}`},
		{"not json at all", `hello`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := projectWithConfig(t, []byte(tc.data))
			changed, exists, err := ols.Sync(root, map[string]string{"app": "src", "deps": "vendor"})
			if err == nil {
				t.Fatal("Sync: err = nil, want an error")
			}
			if !errs.IsUser(err) {
				t.Fatalf("Sync: err = %v, want a user error", err)
			}
			if changed || !exists {
				t.Fatalf("Sync = (%v, %v), want (false, true)", changed, exists)
			}
			assertBytes(t, readConfig(t, root), []byte(tc.data))
		})
	}
}

// TestSyncMinimalPathUpdate checks that only the path string literal changes:
// spacing, ordering and comments elsewhere stay untouched.
func TestSyncMinimalPathUpdate(t *testing.T) {
	input := "{\n" +
		"    \"collections\":  [ { \"name\": \"app\",   \"path\": \"old\" } ],\n" +
		"    \"checker_args\": \"-vet\"\n" +
		"}\n"
	root := projectWithConfig(t, []byte(input))
	changed, _, err := ols.Sync(root, map[string]string{"app": "src"})
	if err != nil || !changed {
		t.Fatalf("Sync = (%v, %v), want (true, nil)", changed, err)
	}
	want := strings.Replace(input, `"path": "old"`, `"path": "src"`, 1)
	assertBytes(t, readConfig(t, root), []byte(want))
}

// TestCheck checks the read-only consistency check.
func TestCheck(t *testing.T) {
	cases := []struct {
		name        string
		in          string
		collections map[string]string
		wantConsist bool
		wantExists  bool
		wantErr     bool
	}{
		{"no ols.json", "", map[string]string{"app": "src"}, false, false, false},
		{"consistent", "base.json", map[string]string{"app": "src"}, true, true, false},
		{"consistent custom", "custom.json", map[string]string{"app": "src"}, true, true, false},
		{"consistent with comments", "with-comments.json", map[string]string{"app": "src"}, true, true, false},
		{"needs add", "base.json", map[string]string{"app": "src", "deps": "vendor"}, false, true, false},
		{"needs remove", "with-deps.json", map[string]string{"app": "src"}, false, true, false},
		{"needs update", "stale-app.json", map[string]string{"app": "src"}, false, true, false},
		{"empty manifest needs remove", "with-deps.json", nil, false, true, false},
		{"bad syntax", "bad-syntax.json", map[string]string{"app": "src"}, false, true, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var root string
			if tc.in == "" {
				root = t.TempDir()
			} else {
				root = projectWithConfig(t, fixture(t, tc.in))
			}
			before, beforeErr := os.ReadFile(filepath.Join(root, ols.FileName))
			consistent, exists, err := ols.Check(root, tc.collections)
			if tc.wantErr {
				if err == nil || !errs.IsUser(err) {
					t.Fatalf("Check: err = %v, want a user error", err)
				}
			} else if err != nil {
				t.Fatalf("Check: %v", err)
			}
			if consistent != tc.wantConsist {
				t.Fatalf("Check: consistent = %v, want %v", consistent, tc.wantConsist)
			}
			if exists != tc.wantExists {
				t.Fatalf("Check: exists = %v, want %v", exists, tc.wantExists)
			}
			// Check is read-only.
			after, err := os.ReadFile(filepath.Join(root, ols.FileName))
			if beforeErr != nil {
				if !os.IsNotExist(err) || !os.IsNotExist(beforeErr) {
					t.Fatalf("Check created or removed ols.json: before=%v after=%v", beforeErr, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("Check lost ols.json: %v", err)
			}
			if !bytes.Equal(before, after) {
				t.Fatal("Check modified ols.json")
			}
		})
	}
}
