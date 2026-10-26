# Decisions

This document records the ambiguities found while implementing Woden and the
conservative, Cargo-like decisions taken for them. `README.md` is the sole
requirements source; `docs/BRIEF.md` is implementation guidance and
`docs/FIXTURES.md` describes the test resources. Fixture drift discovered during
the M0 audit is tracked with stable IDs (`AUD-###`) in
`docs/FIXTURES_AUDIT.md`; that document also contains the raw evidence for the
real-Odin verification summarised in §13.

The document uses the following conventions:

- "The toolchain" means the pinned versions in `mise.toml`:
  Go 1.27.x, Odin `dev-2026-10` (`dev-2026-10-nightly:84bc3fc`) and OLS/odinfmt
  `dev-2026-10`.
- A decision is normative for the implementation and the tests. Decisions that
  are still waiting for a real-world observation are listed in §14 and are
  isolated behind interfaces so that they can be adjusted with a contract test.
- Changes to a decision must be appended to the change log in §15.

## 1. Sources of truth and fixture drift

### D-001 README.md is the only requirements source

`README.md` wins over `docs/BRIEF.md`, `docs/FIXTURES.md` and the audit whenever
they disagree; `BRIEF.md` is guidance and `FIXTURES.md` is a test-resource
description. Where the README is silent, Cargo's behaviour is the tie-breaker.
The README is not modified (BRIEF §7); this document is the place for the
remaining ambiguities.

### D-002 Fixture drift is recorded, never papered over

Existing `example/` and `testdata/` files are not modified. Missing non-Odin
fixtures that have a full spec in `docs/FIXTURES.md` were created in M0
(`testdata/toml/valid-minimal.toml`, `testdata/toml/valid-full.toml`,
`testdata/lock/valid.lock`; AUD-001..AUD-003). Remaining drift is handled by
branching or skipping the affected assertion and citing the AUD ID in the test,
so that a later fixture fix is a visible test change (BRIEF §1.1). §12 is the
complete mapping.

### D-003 Golden files beat the README fences (AUD-010)

The fenced blocks in README's "初始化生成的文件" section contain a blank line
before the closing fence (73/83/121 bytes for `woden.toml`, `src/main.odin`,
`tests/main_test.odin`) while `testdata/golden/new/` has 72/82/120 bytes — one
final LF and no blank line. The blank line is a Markdown artifact of the README
fences; the golden files are the intended byte output. `woden new`/`woden init`
must reproduce the golden bytes exactly, and the golden comparison is
byte-for-byte (no EOL or trailing-whitespace normalisation). The same rule
applies to `testdata/golden/new/.gitignore`, which has no final LF (AUD-009):
the generated `.gitignore` matches the golden file byte-for-byte, so it also has
no final LF until the fixture is fixed. If AUD-009 is fixed by appending an LF,
the template and the test input change together in the same commit.

### D-004 Root editor/CI files are left as they are (AUD-006/AUD-007/AUD-011)

BRIEF §1 declares the repository-root `.gitattributes`, `.editorconfig` and
`.vscode/settings.json` ready and not to be modified. Their bytes differ from
`docs/FIXTURES.md` §3.1 (missing final LF in the first two; the `.vscode` file is
a personal 58-line settings file, not the 7-line snippet, and is not strict JSON
because it contains `//` comments). No test parses `.vscode/settings.json` and
Woden never reads it, so the drift is harmless; the files stay untouched.

### D-005 odinfmt rewrites LF to CRLF on Windows (AUD-014)

`odinfmt <dir> -w` from OLS `dev-2026-10` rewrites line endings to CRLF on
Windows even when the source is already formatted: the default `Newline_Style`
is `.CRLF` when `ODIN_N_OS == .Windows` (observed, see
`docs/FIXTURES_AUDIT.md` §7.2). The README explicitly says Woden does not
generate or manage `odinfmt.json`, so the product must not write such a file to
fix this. Consequences:

- `woden fmt` on Windows may change file hashes only through line endings when
  the project does not force LF itself. This is odinfmt's behaviour, not a Woden
  bug; `woden` only reports odinfmt's exit status.
- The end-to-end harness writes an `odinfmt.json` containing
  `{"newline_style": "LF"}` into the temporary `example/` copy before running
  `woden fmt`, which makes the hashes stable (verified: all four files unchanged
  with the config, all four rewritten without it). The file is test-only and
  never part of the repository or of a generated project.
- On Linux/macOS odinfmt's default is expected to be LF (upstream source
  default), so the workaround is not expected to be necessary there; the CI e2e
  job on Ubuntu confirms it (still listed as unverified in §14 until CI runs).

## 2. CLI shape, output and exit codes

### D-010 Cobra layout

Commands are the 13 commands of README (`new`, `init`, `fmt`, `check`, `lint`,
`test`, `build`, `run`, `clean`, `add`, `remove`, `sync`, `update`) plus the
generated `completion`/`help` and a `version` command. They are grouped as
`project` (`new`, `init`), `build` (`fmt`, `check`, `lint`, `test`, `build`,
`run`, `clean`) and `dependency` (`add`, `remove`, `sync`, `update`) for help
output only (BRIEF M1). Command `Use`, `Short`, `Long`, `Example` and `Args`
follow the README examples.

### D-011 Exit codes

- `0` on success.
- `101` for every failure of a non-`run` command and for every Woden-internal
  error, including a non-zero exit status of a child process (`odin check`,
  `odinfmt`, `git`, ...), even when that status is `1` or not otherwise
  meaningful.
- `woden run` is the only exception: it returns `odin run`'s exit code, which is
  the exit code of the executed program, and for an Odin compile failure the
  non-zero code Odin returns there. No wrapping or re-mapping happens.

This is implemented by having the root command return a typed error
(`internal/errs`) and `main` mapping it to the process exit code.

### D-012 Error output

Errors are written to stderr in English, prefixed with `Error: `, and are
actionable (`... run `woden sync` to restore it`). Usage is printed for
flag/argument errors, not for runtime failures (`SilenceUsage` is enabled once
the command starts executing). No stack traces, no debug output and no ANSI
colour are written. Internal errors carry a generic message; the underlying
error is included after `: `. Warnings (unknown `woden.toml` keys) are written
to stderr as `Warning: ...` before the command result.

### D-013 Successful output is minimal

Non-`run` commands print nothing on success, except where the README requires
output: `clean --dry-run` lists the entries under `target/` and `test`/`build`/
`run` pass through the child process' output. `check`, `lint`, `fmt`, `sync`,
`add`, `remove`, `update` and `clean` are silent on success so that scripts can
rely on stdout. This is the Cargo-like conservative choice given the README's
silence.

### D-014 Argument validation

`woden new PATH` requires exactly one path; `woden init [PATH]` takes zero or
one; `woden add NAME --git URL [--branch B | --tag T | --rev R]` requires the
name and exactly one source option, with at most one reference; `woden run`
accepts only arguments after `--` and rejects other positional arguments;
the remaining commands take no positional arguments. Unknown flags are errors;
there is no passthrough of arbitrary Odin or odinfmt flags (README line 199).

### D-015 Help and version

`woden help`, `-h`/`--help` and `completion` are Cobra defaults. `woden version`
uses the installed module version from `debug.ReadBuildInfo` when available; a
linker override such as `-ldflags "-X github.com/yuqing-256/woden/cmd.version=v1.2.3"`
takes precedence. Local development builds print `dev`. Help text is English.

## 3. Configuration (`woden.toml`)

### D-020 TOML library

`github.com/BurntSushi/toml v1.6.0` is used because it provides strict decoding
with the raw key list (`MetaData.Undecoded`), parse error positions including
line/column, and stable decoding of the TOML subset used by `woden.toml`. It is
a single, widely used dependency. No other runtime dependency is added beyond
Cobra and this library (BRIEF §7).

### D-021 Strict decoding and unknown keys

Decoding uses `toml.DecodeFile` into typed structs. Every undecoded key produces
one `Warning: unknown key ...` line on stderr (sorted, deduplicated) and is
otherwise ignored; the command continues. A key with an invalid type or value
is a hard error (`101`) with the file position from the TOML library. Duplicate
keys and duplicate tables are hard errors. Empty files, missing `[package]` and
missing `name`/`version` are hard errors.

### D-022 Validation rules

From README, plus the following conservative interpretations:

- `package.name`: starts with an ASCII letter or `_`; remaining characters are
  ASCII letters, digits, `-` or `_`; not a Windows reserved device name
  (case-insensitive, with or without an extension: `con`, `prn`, `aux`, `nul`,
  `com1`..`com9`, `lpt1`..`lpt9`). The name comes from the directory name for
  `new`/`init` and from `woden.toml` afterwards.
- `package.version`: required non-empty string. It is metadata only; Woden does
  not SemVer-validate it beyond being a non-empty string.
- `[collections]`: names are non-empty and must not be `core`, `base` or
  `vendor` (Odin built-ins). Paths are relative to the project root, must not
  be absolute and must resolve inside the project root (symlink escape is
  rejected). Duplicate paths are allowed.
- `[dependencies.<name>]`: the name follows the same rule as `package.name`;
  `git` is required and must be a valid URL that does not start with `-` and
  does not use git's `ext::` transport; at most one of `branch`, `tag`, `rev`
  may be present; `rev` must be a full or abbreviated (>= 7 hex characters)
  commit id. `file://` URLs are only accepted in builds with the `woden_testing`
  tag (D-092).
- If no Git dependency is declared, a `[dependencies]` table with no members is
  allowed; it is treated as empty.

### D-023 Profiles

Only `dev` and `release` exist. Defaults are the README table:
`debug = true`, `opt-level = "minimal"` for `dev` and `debug = false`,
`opt-level = "speed"` for `release`. A `[profile.dev]`/`[profile.release]`
table may override only `debug` and `opt-level`; partial overrides keep the
other default. Any other profile name is an error. `debug` maps to `-debug`,
`opt-level` maps to `-o:` with the allowed values `none`, `minimal`, `size`,
`speed`, `aggressive` (verified against the real compiler's error text, §13).
`build` uses `dev`; `build --release` uses `release`; `run` and `test` always use
`dev`; there is no `run --release`.

### D-024 Paths and project root

Every relative path in the configuration is resolved against the project root
(the directory containing `woden.toml`), not against the invocation directory.
`woden` walks up from the current directory to the nearest `woden.toml` for all
commands except `new` and `init`, which use their path argument directly
(`init` without a path uses the current directory). Paths must stay inside the
project root after `filepath.Clean`/symlink resolution; escaping is an error.

## 4. File editing and generation

### D-030 Textual `woden.toml` editing

`add`/`remove`/`sync`-triggered changes to `woden.toml` are minimal textual
edits, not a decode/re-encode cycle: comments, key order, whitespace and the
file's line-ending style are preserved; only the `[collections].deps` line and
the `[dependencies.<name>]` table are added or removed. Unsafe forms that the
textual editor cannot preserve (for example `collections = { ... }` written as
an inline table, or a dependency table written inline) are rejected with
`Error: cannot edit woden.toml safely ...` instead of being reformatted. Every
edit is validated by re-decoding the result; on validation failure the original
bytes are left untouched. The concrete edit rules for dependency tables follow
BRIEF and FIXTURES §3.4: a new dependency table is appended at the end of the
file after one blank line; removing a dependency removes its table, the comment
block immediately above it and one separating blank line; the file always ends
with exactly one final newline after Woden's edit.

### D-031 Atomic writes and line endings

All writes use `internal/fsx.AtomicWrite`: create a temporary file in the
target directory, write, `Sync`, `Close`, `Chmod` to the original mode, then
rename over the target; on error the temporary file is removed. New files are
always LF (goldens are LF). Edits of existing files preserve the detected
line-ending style per file (LF unless the file contains CRLF; the test
`ToCRLF` helper derives the CRLF variant), and preserve a missing final newline
for files Woden does not explicitly rewrite. Repository text files themselves
are LF only, enforced by `.gitattributes`.

### D-032 `init` and `.gitignore`

`new`/`init` create a missing `.gitignore` with exactly the golden bytes. When
one exists, only the missing `/target/` and `/vendor/` lines are appended at the
end, after ensuring the file ends with a newline; all other bytes are preserved.
If the file already contains both lines (with either LF or CRLF endings) it is
not modified.

### D-033 `new` / `init` semantics

- `new PATH`: `PATH` must not exist or must be an empty directory. `init
  [PATH]`: `PATH` must exist and be a directory. A symlink as the project
  directory or as any component that Woden would create is refused.
- The project name is the directory basename and is validated with D-022 before
  any file is written. Reserved names and invalid characters are reported with
  the suggestion to rename the directory first.
- If `woden.toml` already exists in the target root, `init` fails (`101`) and
  writes nothing; there is no force flag.
- No Git repository is initialised.
- Existing files are never overwritten: existing `ols.json` is left untouched,
  `src/main.odin` is not created when `src/` already contains any `.odin` file,
  and `tests/main_test.odin` is not created when `tests/` already contains any
  `.odin` file.
- Generated files are byte-identical to `testdata/golden/new/` (D-003). The
  `woden.toml` template substitutes `package.name`.
- On failure, files and directories created by this invocation are rolled back
  in reverse creation order; files that already existed are never deleted.

## 5. External processes and the Odin invocation contract

### D-040 Process wrapper

`internal/proc` runs external programs with `exec.CommandContext` (the context
is cancelled by Ctrl-C/SIGTERM via `signal.NotifyContext`), never through a
shell. Executables are found with `exec.LookPath`; a missing `odin` or
`odinfmt` is reported as a user error naming the tool and its purpose (no
fallback download). All paths passed to a child are absolute. The child
inherits the environment, with `GIT_TERMINAL_PROMPT=0`, `GIT_ASKPASS=` and
`GIT_CONFIG_NOSYSTEM=1` added for Git (BRIEF §3); `LC_ALL` is not overridden
for Git because it is only relevant for parsing, and Woden parses Git output
only for machine formats (`-z`/`--format` where needed). Child stdout/stderr are
streamed through to Woden's stdout/stderr for `run`/`test`/`build` so errors
remain actionable; for capture-only calls they are captured and included in the
error.

### D-041 Collection arguments

Every Odin invocation receives `-collection:<name>=<absolute path>` for each
entry of `[collections]`, sorted by collection name for deterministic command
lines (golden tests in `internal/odin`). Odin's implicit collections (`core`,
`base`, `vendor:`) are never passed. `odinfmt` does not receive collection
flags.

### D-042 Odin command lines

The exact contract (all paths absolute, cwd rules from D-040):

| Command | Invocation |
| --- | --- |
| `fmt` | `odinfmt <abs src> -w` and `odinfmt <abs tests> -w`, for existing directories only; both missing is an error; `libs/` and `vendor/` are never touched |
| `check` | `odin check <abs src> -collection:...` |
| `lint` | `odin check <abs src> -vet -collection:...` |
| `test` | `odin test <abs tests> -all-packages -out:<abs target/debug/deps/<name>[.exe]> -collection:...` + dev options |
| `build` | `odin build <abs src> -out:<abs target/{debug,release}/<name>[.exe]> -collection:...` + profile options (`-debug` for `debug = true`, `-o:<opt-level>`) |
| `run` | `odin run <abs src> -out:<abs target/debug/<name>[.exe]> -keep-executable -collection:...` + dev options + `-- <args...>` when arguments are present |

On Windows every `-out:` path ends in `.exe` for `build`, `run` and `test`;
Odin rejects a path without an extension and does not create missing parent
directories (AUD-015, verified). Woden therefore `os.MkdirAll`s
`target/debug`, `target/debug/deps` and `target/release` before invoking Odin
and appends `.exe` where Windows requires it. `test` intentionally relies on
Odin deleting its artifact after the run (verified: the file is gone after a
successful test run even with `-out:`). `run` keeps the artifact because
`-keep-executable` is passed (verified both ways).

### D-043 Failure mapping and exit codes

A non-zero exit status from `odin`, `odinfmt` or `git` is mapped to a user
error (`101`) for every command except `run`. The child's stderr is included
verbatim in the error message so the compiler diagnostic is visible; Woden does
not add its own interpretation beyond the one-line prefix. For `run`, Woden
returns the child's exit code unchanged and prints no extra message on failure
or success (the program's own output is enough). A failure in the preflight
phase (missing vendor, missing tool, invalid config) fails before any child is
started.

### D-044 `run` working directory and arguments

`odin run` is started with its working directory set to the directory in which
`woden` itself was invoked (not the project root). The program therefore sees
that directory as its cwd. `--`-separated arguments are passed to the child
after a literal `--`, preserving their order, spaces and quoting; Woden does not
parse or transform them. Verified: the program receives them in `os.args[1:]`
and `--exit N` reaches `os.exit(N)`, with the shell seeing the same exit code.

### D-045 Missing tools

`odin`/`odinfmt` are required only by the commands that use them; `git` is
required only by dependency commands. The check is a `LookPath` before the
command starts, with a message like `Error: odin not found in PATH (required by
woden build)`. `woden fmt` needs `odinfmt` only.

## 6. Formatting

### D-050 What `fmt` formats

`fmt` formats `src/` and `tests/` recursively with `odinfmt -w`, and only those:
`libs/` (project-owned local code) and `vendor/` are not formatted, matching
README line 252. If neither directory exists the command fails. `odinfmt`
handles recursion into subdirectories; Woden does not walk the tree itself
(verified: `odinfmt src -w` formatted `src/main.odin`,
`src/greet/greet.odin` and `src/greet/greet_test.odin` in one call and ignored
non-`.odin` files).

### D-051 `odinfmt` invocation

One child per existing directory: `odinfmt <absolute dir> -w`, cwd = project
root, output streamed through only on failure. A non-zero status is `101`. The
Windows CRLF rewrite is odinfmt's documented default (D-005); Woden does not
pass `-config`.

### D-052 `odinfmt.json` is not managed

README line 260: Woden never generates, ships or edits `odinfmt.json`; formatting
style is the user's/OLS's configuration. The e2e workaround in D-005 lives only
in test temp copies.

## 7. `clean`

### D-053 `clean`

`clean` deletes the project's `target/` directory and nothing else (README line
256): source files, `woden.toml`, `woden.lock`, `ols.json`, `libs/` and `vendor/`
are never touched. The deletion plan is built with `os.Lstat` only, children
before parents; symbolic links, junctions and other reparse points inside
`target/` are removed as links and never traversed, so nothing outside `target/`
can be deleted through them. Every entry is containment-checked while the plan is
built and checked again before the first deletion. `--dry-run` and the real run
use the same plan and print the same project-relative paths, one per line, so the
listing is exactly what would be removed; `--dry-run` returns before any remove.
A missing `target/` is success with no output. `target/` itself is removed after
its entries; if `target/` is itself a link, only the link is removed. Paths are
printed with forward slashes on every platform.

## 8. Lock file (`woden.lock`)

### D-060 Format

TOML with `version = 1` and one `[[package]]` table per Git dependency. Field
order is fixed: `name`, `git`, then exactly one of `branch`/`tag`/`rev` when a
reference was requested, then `commit`. The requested reference is stored
verbatim as it appears in the manifest (branch name, tag string, or the revision
the user passed, including an abbreviated one); an omitted reference writes no
key. `commit` is always the resolved full 40-character lowercase hexadecimal
commit id. No other keys are written.

### D-061 Determinism

`[[package]]` entries are sorted by `name` using byte-wise string comparison.
Serialisation is deterministic (stable field order, one blank line between
tables, exactly one final LF) so that `sync`/`add`/`update` produce byte-stable
files. Writes are atomic (D-031). The lock parser rejects unknown keys, duplicate
names, a wrong `version`, and malformed commit ids, with the file position in
the error.

### D-062 Lifecycle

The lock file is created when the first Git dependency is added and is intended
to be committed (README line 41). It is updated by `add`, `update`, `sync` (only
if vendor content had to be restored without a lock change — then the lock is
not rewritten at all) and `remove`. When the last dependency is removed the
lock file is deleted, because the README documents it only as a file that
exists when Git dependencies are present. A lock file without dependencies is
still accepted on read and treated as empty.

### D-063 Authoritative data

For `sync` and `--check` the lock's `commit` values are authoritative; the
manifest is authoritative for the requested references used by `update`. A
manifest/lock mismatch is fixed only by an explicit `update` (or `sync` when
`vendor/` is missing); `sync` never advances a commit.

### D-064 Missing or inconsistent lock/vendor

If `[dependencies]` is non-empty and `vendor/` is missing, every command that
compiles or runs code (`check`, `lint`, `test`, `build`, `run`) fails in
preflight with a message that names `woden sync` and explains how to restore the
vendor tree. If the lock is missing while dependencies are declared, the same
preflight error suggests `sync`. `woden sync` itself is the recovery command and
never requires the vendor tree. `woden add` deliberately has no such preflight —
it creates the vendor tree — and `woden remove` reports a missing or
unrecognised worktree as an ownership problem instead (D-072).

## 9. Git dependencies

### D-070 Source validation

Only `[dependencies.<name>]` with a `git` URL is supported (no registry,
version ranges, transitive dependencies or local path dependencies). URLs must
start with `https://`, `ssh://`, `git://`, `git+https://` or the scp-like
`git@host:path` form; a leading `-` and `ext::` are rejected; `file://` is
accepted only in `woden_testing` builds (D-092). `branch`, `tag` and `rev` are
mutually exclusive. Names are validated like project names.

### D-071 Reference resolution

`--branch B` resolves the branch head on the remote; `--tag T` resolves the
tagged commit (annotated tags are peeled); `--rev R` resolves a full or
abbreviated commit id (>= 7 hex chars) and must exist in the remote; no
reference resolves the remote's default branch. Resolution happens in a fresh
temporary clone so that the remote's real values are used, not local state.

### D-072 Vendor layout and ownership

A dependency lives in `vendor/<name>/` as a detached-HEAD Git working copy at
the locked commit. Woden does not write extra metadata into the working copy:
ownership is proven by the lock entry existing, the directory being a valid Git
repository and `git status --porcelain` being empty (ignored files are allowed).
A missing `.git`, a different HEAD, or any modification (including untracked,
non-ignored files) makes the copy "not safely ours"; the dependency is skipped
with a report and the command exits `101`, without overwriting or deleting the
copy. This applies to `sync`, `update` and `remove`.

### D-073 Clone strategy

`add`/`sync`/`update` resolve into a temporary directory inside `vendor/`
(`vendor/.staging-<16 hex random>`), fetch/checkout there, and then atomically
rename it to `vendor/<name>/`. A pre-existing `vendor/<name>/` is never replaced
in place: `add` refuses it, and `sync`/`update` reuse a clean worktree instead.
A failure leaves the staging directory behind rather than half-renaming; the next
`sync` removes stale staging directories. The staged tree is Git's own checkout,
so Woden does not scan it for links; it also never traverses links when inspecting
a worktree (`git status` stays inside it), and deleting `vendor/<name>/` uses
`os.RemoveAll` on a validated project-relative path (`validate.DependencyName`).
`target/` deletion additionally uses `internal/clean`'s `Lstat` plan (D-053), and
every other Woden write uses `internal/fsx`'s atomic helpers (D-031).

### D-074 `add`

`woden add NAME --git URL [...]` fails if the dependency already exists in the
manifest. On success it: (1) validates the URL/reference; (2) clones the
dependency into a staging directory and resolves the requested ref to a full
commit; (3) edits `woden.toml` textually — creates `[dependencies.NAME]` and,
when no `deps` mapping exists, adds `deps = "vendor"` to `[collections]`
(if `deps` exists with a different path, it is a conflict error and nothing is
changed); (4) writes/extends `woden.lock`; (5) renames the staged checkout to
`vendor/NAME/`; and (6) updates the managed `deps` collection in `ols.json`
(D-082). If a step fails after an earlier one, the partial state is not rolled
back: it is repaired by the next `woden sync`, which is reentrant (the staging
directory is removed, a missing vendor worktree is restored from the lock, and a
lock/manifest mismatch is resolved from the declared reference).

### D-075 `sync`

`sync` restores `vendor/` from the lock: it clones/checks out each missing or
wrong-commit dependency, and skips dependencies whose working copy is modified
(report + `101`, others continue). It never advances a commit, never changes
`woden.toml` dependency sources, and only touches `ols.json` through the OLS
merge in §10. `sync --check` is strictly read-only: no network, no clone, no
write; it checks the manifest/lock/vendor consistency and the managed OLS
mappings, and fails (`101`) on any mismatch.

### D-076 `update`

`update [NAME...]` moves `branch`, `tag` and reference-less dependencies to
their current remote value and rewrites the lock and vendor copy. The lock is
written before the new commit is checked out, so an interrupted `update` is
repaired by the next `sync`. Dependencies pinned with `rev` are skipped by a
no-name `update` and are an error when requested by name. Dirty/foreign vendor
copies are refused (D-072). A dependency whose manifest entry does not exist is
an error when named.

### D-077 `remove`

`remove NAME` first proves ownership and cleanliness (D-072): a missing or
foreign worktree, a dirty worktree, a disagreeing origin/lock URL or HEAD not at
the locked commit is refused before anything is deleted. Otherwise it removes
`vendor/NAME/`, removes the lock entry, removes the `[dependencies.NAME]` table
textually (including the comment block directly above it and the separating
blank line, per FIXTURES §3.4), and — when this was the last dependency — removes
the `deps` collection mapping from `woden.toml`, deletes the now-empty `vendor/`
directory, removes the `deps` collection from `ols.json` (through the OLS merge)
and deletes `woden.lock`. Other dependencies are not affected. The ordering is
safe against interruption because `sync` can repair any intermediate state.

### D-078 Network and failure behaviour

No network access outside Git operations; no interactive prompts
(`GIT_TERMINAL_PROMPT=0`); HTTP(S)/SSH authentication is whatever Git provides.
Network, authentication, missing-ref and clone errors are user errors (`101`)
with the Git message included. Offline recovery is `sync` from the lock when
`vendor/` is present, or a Git fetch when it must be restored.

## 10. OLS (`ols.json`)

### D-080 Merge semantics

`woden.toml` is the authority for collection mappings. `sync` (and the managed
part of `add`/`remove`) updates `ols.json` incrementally: the `deps` collection
is fully managed (added/removed with the dependency set), all other collections
managed by `woden.toml` are added or updated but never removed, and collections
that exist only in `ols.json` as well as unrelated settings are preserved
byte-for-byte where possible. Same-named collections take the `woden.toml`
path. If `ols.json` does not exist, `sync` does not create it (README line 258).

### D-081 Lossless, span-based editing

`internal/ols` scans `ols.json` into a lossless document of byte spans rather
than decoding/re-encoding it. Only the byte ranges of managed array entries are
edited, so key order, comments and unrelated formatting are preserved. If the
file cannot be safely parsed or a managed region cannot be edited without
losing comments/formatting that Woden would have to rewrite (for example a
malformed structure or a managed entry written in an unsupported inline form),
the command fails with an explanatory error and leaves the file untouched
(README line 258: "不得重写整份文件"). `sync --check` performs the same merge
on an in-memory copy and compares the result without writing.

### D-082 When `ols.json` is synced

`sync` always reconciles. `add` and `remove` update the managed `deps` entry
immediately as part of their transaction, so a project with Git dependencies is
consistent even if the user never runs `sync`; the full merge (all collections)
is still left to `sync`.

### D-083 Indentation of managed entries

Managed entries keep the indentation style of the surrounding file: Woden
detects the indent of the collections array (or of the neighbouring entries) and
inserts entries with the same indentation and compactness. Honouring truly
tab-indented files cannot be exercised because `testdata/ols/tabs.json` is a
byte copy of `base.json` (AUD-013); the affected test branches/skips on that
fixture and cites AUD-013 until the fixture is regenerated with tabs.

## 11. Tests and CI

### D-090 Three test layers

Unit tests are table-driven on `testdata/` fixtures. Integration tests use
temporary directories and a fake `odin`/`odinfmt` helper process
(`internal/testutil/faketool`) that records argv and returns configured exit
codes; they verify command lines, preflight, exit-code mapping and output
without a real toolchain. End-to-end tests (`e2e/`, build tag `e2e`) build the
real binary and use the real `odin`, `odinfmt` and `git` (BRIEF §5). Tests never
run Woden inside the repository's `example/` or `testdata/`; they always copy
into temporary directories.

### D-091 End-to-end harness

`e2e/TestMain` builds two binaries once: `woden` (default tags) and
`woden-testing` (`-tags woden_testing`). Dependency tests use the latter so that
`file://` URLs work; all other tests use the released-style binary. Tests skip
with `t.Skip("real odin/odinfmt not found in PATH")` (or `git`) when the tool
is absent, so a developer without Odin can still run `go test -tags e2e ./...`
partially. Dependency tests additionally probe `git ls-remote <file://...>` and
skip when the host cannot use `file://` transport (D-096). Each test starts from
`testutil.CopyExample(t)` or a fresh
`t.TempDir()`. The `fmt` tests write `odinfmt.json` with
`{"newline_style": "LF"}` into the temporary copy (D-005); this file is
created by the harness only. Assertions follow `docs/FIXTURES.md` §4 including
the mutation table.

### D-092 `woden_testing` build tag

Only binaries built with the `woden_testing` tag accept `file://` Git URLs
(`internal/validate/allowfile_default.go` vs `allowfile_testing.go`). Tests that
need local dependency repositories are written under both `e2e` and
`woden_testing` tags and skip (with a reason) in default builds. CI runs
`go test ./...`, `go test -tags woden_testing ./...` and
`go test -tags "e2e woden_testing" ./...` (last one in the e2e job).

### D-093 CI

`.github/workflows/ci.yml` has two parts. Matrix jobs (Ubuntu, macOS, Windows)
run `go vet ./...`, `go test -race ./...` and `go test -tags woden_testing
./...` with `actions/setup-go` using `go-version-file: go.mod`. A separate
Ubuntu e2e job downloads the pinned Odin and OLS releases (`dev-2026-10`, the
same versions as `mise.toml`), puts `odin`/`odinfmt` on `PATH`, and runs
`go test -tags "e2e woden_testing" ./...`. Tests never require network; CI
downloads only the toolchain and the Go modules at setup time.

### D-094 No release/goreleaser configuration

The README and BRIEF require no release artifacts, checksums or packaging:
versioning uses Go module build information, with optional `-ldflags` override
as described in D-015. A `.goreleaser.yml`
is deliberately not added, because inventing release targets would be
requirements not present in the sole requirements source. It can be added later
without changing behaviour.

### D-095 `DECISIONS.md` is committed

This document is part of the repository and is updated with the implementation;
it is the place to look for "why does Woden behave this way?" after the README.

### D-096 `file://` transport in the e2e dependency tests

The committed dependency scenarios use `testutil.NewDepRepo` and therefore real
`file://` Git URLs (D-070); the harness never registers a temporary `git daemon`
or any other fallback. A working `file://` transport is a property of the host,
not of Woden:

- CI (Ubuntu, macOS and Windows runners) has working `file://` transport, and
  the e2e job runs the dependency scenarios there
  (`go test -tags "e2e woden_testing" ./...`).
- In the development sandbox used for this implementation (DSH on Windows)
  `git clone file://...` fails because Git's transport helper `sh.exe` needs
  named pipes, which the sandbox blocks ("couldn't create signal pipe, Win32
  error 5"). Local Git operations (init/add/commit/tag/checkout/rev-parse) work
  normally.

Each e2e dependency test therefore starts with a capability probe,
`git ls-remote <repo.URL>`; on failure it skips with
`git file:// transport unavailable in this sandbox (CI runs it)`. Probe result
observed while writing this suite: `git ls-remote file:///...` fails in the
sandbox, so the dependency e2e tests skip locally and run only in CI. The
dependency logic itself is still exercised locally by the `woden_testing`
integration tests, which use a loopback `git daemon` as a test-only fallback
(deps-dev); that fallback is not part of the committed e2e harness.

## 12. Fixture drift and test handling

The table maps every `AUD` finding to its handling. "resolved" means the fixture
was created in M0 (and committed); "manual" means the drift remains in the
repository and no test may silently rely on it.

| ID | Fixture(s) | Handling | Tests |
| --- | --- | --- | --- |
| AUD-001 | `testdata/toml/valid-minimal.toml` | resolved (created, 72 B) | config unit tests read `valid-*` names; no skip |
| AUD-002 | `testdata/toml/valid-full.toml`; extra `vaild-full.toml` | resolved (created, 529 B); extra misspelled file untouched | config tests use `valid-full.toml`; the extra file is ignored |
| AUD-003 | `testdata/lock/valid.lock`; extra `vaild.lock` | resolved (created, 557 B, byte-equal to spec) | lock tests use `valid.lock`; the extra file is ignored |
| AUD-004 | `example/src.exe` | manual (600 KB Windows binary tracked) | `CopyExample` copies it but no test asserts the example tree, so no skip; it must be removed by the Lead before release |
| AUD-005 | `testdata/.gitattributes` | manual (0-byte file contradicting FIXTURES §3.1) | ignored by tests; harmless |
| AUD-006/007 | root `.gitattributes`, `.editorconfig` missing final LF | manual (BRIEF says don't modify root files) | no test parses them; git/EOL behaviour unaffected |
| AUD-008 | `example/.gitignore` missing final LF | manual | example copies work; no test compares this file's final newline |
| AUD-009 | `testdata/golden/new/.gitignore` missing final LF | manual, golden authoritative | `new`/`init` byte-compare against golden; generated `.gitignore` has no final LF until the fixture is fixed (D-003) |
| AUD-010 | README golden fences vs golden files | manual, golden authoritative | `new`/`init` byte-compare against golden (single final LF); README blank lines ignored (D-003) |
| AUD-011 | `.vscode/settings.json` | manual (BRIEF: don't modify) | never parsed by Woden or tests |
| AUD-012 | `testdata/toml/edit/*.toml` comment spacing/content | manual, actual files are authoritative inputs | editor unit tests use the actual files; the implementation preserves the actual 2-space comment and padded `tag` comment and final newline; no skip |
| AUD-013 | `testdata/ols/tabs.json` identical to `base.json` | manual (fixture not tab-indented) | the tab-preservation case is skipped/branched with `AUD-013` in the reason; other OLS merge tests run on `base.json`/`with-deps.json`/`custom.json` |
| AUD-014 | odinfmt LF→CRLF on Windows | manual; e2e writes `odinfmt.json` LF into temp copy | `fmt` e2e tests use the workaround; Windows-only; Linux confirmed in CI |
| AUD-015 | `-out:` requires `.exe`; parent dirs not created | implemented decision | build/run/test append `.exe` on Windows and `MkdirAll` target dirs; e2e asserts platform-specific artifacts |

## 13. Verified Odin behaviour (real toolchain, Odin `dev-2026-10`)

The raw commands and outputs are in `docs/FIXTURES_AUDIT.md` §7. Summary of the
behaviours the design relies on:

| Claim | Result |
| --- | --- |
| `odin run` / `odin test` honour `-out:` and `-keep-executable` | verified: `run` without `-keep-executable` deletes the artifact; with it the artifact remains; `test` deletes its artifact after a successful run even with `-out:` |
| `odin run` passes through the program's exit code | verified: `-- --exit 3` produced exit code 3 (`$LASTEXITCODE` = 3) |
| `--` arguments reach the program | verified: `-- a "b c"` printed `arg[0]=a`, `arg[1]=b c` from `os.args[1:]` |
| `-vet` reaches packages imported through a collection (deps) | verified: an unused import/var in a `deps:` package passes `odin check` and fails only with `-vet` |
| `odinfmt <dir> -w` recurses and ignores non-`.odin` files | verified: one call formatted `src/main.odin`, `src/greet/*.odin`, `src/greet/greet_test.odin`; `.txt` ignored |
| Windows `-collection:name=C:\abs\path` form | verified: both `C:\...` and `C:/...` absolute forms accepted |
| `-o:` allowed values | verified from the compiler's error text: `minimal`, `size`, `speed`, `aggressive`, `none` |
| tests import `app:greet` (a subpackage of `src/`) | verified: example tests compile and run (3 tests) |
| `-out:` on Windows requires `.exe`, parent dirs are not created | verified (AUD-015) |
| `odin test` output format | observed: `Finished 3 tests ... All tests were successful.`; the exact wording is version-dependent, so e2e asserts the exit code and a loose `3 tests` pattern |
| cwd of the program launched by `odin run` | verified: it is the invocation directory, not the package directory |

### Local e2e verification (Windows, real Odin `dev-2026-10`)

While the e2e package was written, the suite was executed locally against the
real toolchain with `go test -tags e2e ./e2e -count=1`:

- 19 of 20 non-dependency tests passed, including `new`/`init` golden-byte
  comparison, `check`/`lint`, the three-test `test` run, `build`/`build
  --release` artifacts, `run` (first output lines, argument forwarding,
  exit-code passthrough, kept `target/debug` artifact), `fmt` hash stability and
  the mutation cases (type error, unused variable, wrong expectation, missing
  collection).
- `TestCheckPrefersSyncWhenVendorMissing` failed: `woden check` exited 0 for a
  project with a `[dependencies]` entry and no `vendor/`, while D-064 and
  FIXTURES §4 require a preflight error suggesting `woden sync`. The
  implementation currently only inspects the `deps` collection directory
  (`Project.CheckVendor`), not `[dependencies]`; the command layer needs to
  treat a non-empty `[dependencies]` without `vendor/` as a preflight error.
  The test follows the requirement and is left unchanged.
- `go test -tags "e2e woden_testing" ./e2e` compiled; the eight dependency
  scenarios skipped locally in the sandbox with the D-096 probe message
  (`git ls-remote file:///...` fails with `couldn't create signal pipe, Win32
  error 5`), as expected.

## 14. 待实测 / unverified

These items are not verified with the real toolchain yet. They are isolated
behind the process/OLS interfaces so that they can be corrected with a contract
test when observed:

1. OLS tolerance of comments and trailing commas in `ols.json` (FIXTURES
   `with-comments.json`). Woden's own span-based scanner tolerates JSONC, so
   the merge works; whether the real OLS language server accepts the same file
   is unverified (the audit did not drive the LSP).
2. `odinfmt` behaviour on files with real syntax errors (the audit only used a
   parse-valid file with a type error; formatting such a file succeeded).
3. `odinfmt` directory behaviour and default line endings on Linux/macOS
   (Windows defaults to CRLF; Linux is expected to default to LF from upstream
   source, but this is confirmed only when the CI e2e job runs).
4. Windows paths containing spaces and UNC paths, both for the project root and
   for `-collection:`/`-out:` arguments.
5. `odinfmt --stdin` behaviour (not used by Woden; only relevant if a future
   version formats a single file).
6. The exact OLS `with-comments` merge result: Woden refuses/merges according to
   D-081, but "OLS still opens the resulting file" is not tested.
7. e2e dependency scenarios that need `file://` fetch (`add`/`sync`/`update`/
   `remove` end-to-end) are skipped in the local DSH sandbox and execute only in
   CI (D-096); their local substitution is the `woden_testing` integration suite
   that uses a loopback `git daemon`.

## 15. Change log

- 2026-10-26: initial decisions recorded during the M0-to-M9 implementation
  (auditor, task-7). Full Odin verification evidence lives in
  `docs/FIXTURES_AUDIT.md`; M0 commit `58f719c` added the audit and the three
  missing valid fixtures.
- 2026-10-26: Lead integration pass aligned sections 5 and 8 with the
  shipped implementation (add/remove preflight, staging directory naming,
  add/remove/update write ordering, repair through `sync`), added section 7
  `clean`, and renumbered the later sections.
