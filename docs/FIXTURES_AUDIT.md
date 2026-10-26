# Fixtures Audit (M0)

- Audit date: 2026-10-26 (Asia/Shanghai)
- Repository commit: `864dc7d1ebb08c8effe350d4f1fe25557bec8c6e` (`chore: scaffold cobra-cli skeleton`, branch `main`)
- Scope: M0 test-resource reconciliation required by `docs/BRIEF.md` §1.1, against the spec in `docs/FIXTURES.md` and `README.md`.
- Auditor: fixture-auditor role; report only, no implementation changes.
- Toolchain on the machine: Go 1.27.3, Odin `dev-2026-10-nightly:84bc3fc`, OLS/`odinfmt` `dev-2026-10`, git; Windows (pwsh 7).
  - `odin`: `C:\Users\akina\AppData\Local\mise\installs\odin\dev-2026-10\odin.exe`
  - `odinfmt`: `C:\Users\akina\AppData\Local\mise\installs\ols\dev-2026-10\odinfmt.exe`

## 1. Method

1. Enumerated `example/` and `testdata/` recursively and added the repo-root files `.gitattributes`, `.editorconfig`, `.vscode/settings.json`.
2. Extracted every fenced full-text block from `docs/FIXTURES.md` and every golden block from the README "初始化生成的文件" section with a one-off Go program into a TEMP directory (never into the repo), expanded them into an expected tree (`LF`-terminated fenced content), then compared item by item with `git diff --no-index`. Every file was also measured with size, SHA-256, BOM flag and CR and CRLF byte counts.
3. Odin sources described only behaviorally were read statically; the FIXTURES verification checklist was then executed against a TEMP copy of `example/` only (never in place).
4. `git ls-files --eol` plus a raw byte scan of all fixture files for BOM and CRLF.
5. All paths and commands in this audit are Windows paths run from the repository root unless stated otherwise. No repo file was modified by the audit except the three new `testdata/` fixtures listed in §3 and this document.

## 2. File inventory

Spec tree (FIXTURES): 56 files. Before the audit 58 files were present; after creating the three missing fixtures, 61 files are present.

| Group | Expected | Before audit | After audit | Status |
|---|---|---|---|---|
| repo root configs (`.gitattributes`, `.editorconfig`, `.vscode/settings.json`) | 3 | 3 | 3 | present, drifts AUD-006, AUD-007, AUD-011 |
| `example/` | 9 | 10 | 10 | present, extra `example/src.exe` (AUD-004), drift AUD-008 |
| `testdata/README.md` | 1 | 1 | 1 | byte-exact |
| `testdata/golden/new/` | 5 | 5 | 5 | present, drifts AUD-009, AUD-010 |
| `testdata/toml/` (top level) | 15 | 15 | 17 | 3 expected names missing before, 3 misspelled extras; 3 created (AUD-001–003), extras remain (AUD-002/003), drift AUD-012 is in `edit/` |
| `testdata/toml/edit/` | 5 | 5 | 5 | present, content drift AUD-012 |
| `testdata/lock/` | 6 | 6 | 7 | `valid.lock` created (AUD-003), misspelled `vaild.lock` remains (AUD-003) |
| `testdata/ols/` | 8 | 8 | 8 | present, drift AUD-013 |
| `testdata/deplib/rev1..rev4` | 4 | 4 | 4 | byte-exact |
| Total | 56 | 58 | 61 | — |

Missing expected files (before audit): `testdata/toml/valid-minimal.toml`, `testdata/toml/valid-full.toml`, `testdata/lock/valid.lock`.

Extra files (not in the FIXTURES tree):

- `example/src.exe` (600576 bytes, tracked binary)
- `testdata/.gitattributes` (0 bytes, tracked)
- `testdata/toml/vaild-full.toml`, `testdata/toml/vaild-minimal.toml`, `testdata/lock/vaild.lock` (misspelled "vaild")

Files that match the spec byte-for-byte (no action needed): `example/README.md` (843 B), `example/woden.toml` (92 B), `example/ols.json` (321 B), `example/src/main.odin` (497 B), `testdata/README.md` (970 B), `testdata/toml/bad-syntax.toml` (44 B), `testdata/toml/edit/bad-structure.toml` (76 B), `testdata/ols/custom.json` (442 B), `testdata/deplib/rev1..rev4` (64/64/68/64 B), plus the created fixtures in §3.

Behavioral Odin sources (static review, then compiled with real Odin): `example/src/main.odin` prints `Hello, woden-example!`, then `2 + 3 = 5`, then one `arg[<i>]=<value>` line per argument via `fmt.printfln`, and exits with N after `--exit N`; no stdin, no cwd or clock dependency. `example/src/greet/greet.odin` (`hello :: proc(name: string) -> string`, `fmt.aprintf`), `example/src/greet/greet_test.odin` (`@(test)`, `hello("") == "Hello, !"`), `example/libs/mathx/mathx.odin` (`add :: proc(a, b: int) -> int`) and `example/tests/main_test.odin` (`package tests`, imports `app:greet`, `core:testing`, `libs:mathx`; two `@(test)`; 3 tests in total with `-all-packages`) all satisfy the spec and compile/run.

## 3. Created fixtures

Created from the FIXTURES full-text specs (UTF-8, LF, no BOM, final LF; no existing file touched):

| File | Bytes | SHA-256 | Source |
|---|---|---|---|
| `testdata/toml/valid-minimal.toml` | 72 | `880a264be25896dfe95d352f2e2e0f9225d1d409334099a2ecf8d9d9cac50264` | copy of `testdata/toml/vaild-minimal.toml`; also byte-identical to `testdata/golden/new/woden.toml` |
| `testdata/toml/valid-full.toml` | 529 | `83e889b8c5505085ce906dcb548e36f3c1b78de9f7fcd1847c918cef261d8a2f` | FIXTURES §3.3 full text (includes the `# Full example` first line) |
| `testdata/lock/valid.lock` | 557 | `6a9d04e0f9a9f4f73a0354cdcfe9d2ed9c259f8b6c10b8ec8d36ddf4ebc194db` | FIXTURES §3.5 full text |

Each created file was verified with `git diff --no-index` against the spec-expanded expected tree (empty diff, exit 0).

## 4. Drift log

Status values: **resolved** = fixed by this audit per BRIEF §1.1 step 5; **manual** = needs a human/Lead decision because the audit must not modify existing fixtures or change expectations.

| ID | File(s) | Expected (spec) | Actual | Status |
|---|---|---|---|---|
| AUD-001 | `testdata/toml/valid-minimal.toml` | present, 72 B | missing | resolved — created (§3) |
| AUD-002 | `testdata/toml/valid-full.toml`, `testdata/toml/vaild-full.toml` | `valid-full.toml` 529 B, first line `# Full example` | `valid-full.toml` missing; `vaild-full.toml` (514 B) lacks the first line | resolved (created) + manual (extra `vaild-full.toml` remains; tests must use `valid-full.toml`) |
| AUD-003 | `testdata/lock/valid.lock`, `testdata/lock/vaild.lock` | `valid.lock` 557 B | `valid.lock` missing; `vaild.lock` 557 B is byte-identical to the spec but has the wrong name | resolved (created) + manual (remove or ignore `vaild.lock`) |
| AUD-004 | `example/src.exe` | not in the FIXTURES tree | tracked 600576-byte Windows binary | manual — remove from the repo (not needed by any test) |
| AUD-005 | `testdata/.gitattributes` | FIXTURES §3.1: "不再单独放 `testdata/.gitattributes`" | 0-byte tracked file | manual — harmless but contradicts the spec; remove |
| AUD-006 | `.gitattributes` | 181 B, FIXTURES §3.1 full text | 180 B; last line `* text=auto eol=lf` has no final LF | manual — append a trailing LF |
| AUD-007 | `.editorconfig` | 50 B | 49 B; `end_of_line = lf` has no final LF | manual — append a trailing LF |
| AUD-008 | `example/.gitignore` | 18 B | 17 B; `/vendor/` has no final LF | manual — append a trailing LF |
| AUD-009 | `testdata/golden/new/.gitignore` | 18 B | 17 B; no final LF | manual — append a trailing LF (it is byte-compared by `woden new` tests) |
| AUD-010 | `testdata/golden/new/woden.toml`, `src/main.odin`, `tests/main_test.odin` | README fenced blocks imply 73/83/121 B (blank line before the closing fence); FIXTURES §3.2 requires golden to be byte-equal to README | 72/82/120 B (single final LF, no blank line) | manual — contradiction README vs golden. Recommendation: the golden files are the intended generated output (single final LF) and the blank line is a Markdown artifact of the README fences; keep the golden files as-is, compare templates against them, and record the decision in `docs/DECISIONS.md` |
| AUD-011 | `.vscode/settings.json` | 89 B / 7 lines per FIXTURES §3.1 (`files.eol`, `files.associations` only) | 2683 B / 58 lines: 52 extra lines of gopls/Go settings and `//` comments, then the two expected keys; the file is not strict JSON because of the comments. BRIEF §1 says the root files are ready and must not be modified | manual — contradiction between BRIEF "don't modify" and FIXTURES full text; leave untouched (no test may parse it) or restore it with the Lead's approval |
| AUD-012 | `testdata/toml/edit/*.toml` | FIXTURES §3.4 full texts: first line `# my project`; `name = "my-game"   # keep this comment` (3 spaces); `tag = "v1.0.0" # pinned` | all four files lack `# my project`; `name = "my-game"  # keep this comment` (2 spaces); `tag = "v1.0.0"                              # pinned` (padded comment). Sizes: `base.toml` 93/107, `add-first-dep.want.toml` 212/226, `two-deps.toml` 349/334, `remove-alpha.want.toml` 247/232 (actual/expected) | manual — decide whether to correct fixtures to the spec. The CRLF case derives from these actual files (`base.toml` + `add-first-dep.want.toml`), so tests will follow the actual text; the implementation must still preserve the actual comment spacing and final LF |
| AUD-013 | `testdata/ols/tabs.json` | FIXTURES §3.6: same content as `base.json` but all indentation with tabs | 273 B, 0 tab bytes, 26 space bytes; byte-identical to `base.json` (SHA-256 `7d8b118401621fcfce694db77ebada818b03ff0dccf8276e8e88fb142984eeab`) | manual — regenerate `tabs.json` with tab indentation; until then the tab-merge case cannot be exercised |
| AUD-014 | odinfmt on Windows | FIXTURES §4 e2e: `woden fmt` → exit 0, all file hashes unchanged; mutation case: fmt rewrites only `greet.odin`, `libs/` unchanged; `.vscode/settings.json` comment: "`odinfmt` 和 `woden new` 的输出都是 LF" | real `odinfmt <dir> -w` (OLS dev-2026-10) rewrote LF→CRLF on Windows even when the formatting was already clean; hashes changed for all formatted files (see §7.2) | manual — this breaks the fmt e2e expectations on Windows. Verified workaround: an `odinfmt.json` containing `{"newline_style":"LF"}` at the project root keeps all four files byte-identical (hashes unchanged, 0 CRLF). README line 260 says Woden does not generate or manage `odinfmt.json`, so the e2e harness (not Woden) should place that file in the temp `example/` copy, or compare hashes modulo EOL; record the choice in `docs/DECISIONS.md` |
| AUD-015 | `-out:` paths on Windows | README and BRIEF §4 table use `target/debug/deps/<name>` (test) and `target/debug/<name>` (run) without an extension | real Odin rejects an `-out:` path without `.exe` on Windows: `Output path ... must have an appropriate extension.` (exit 1), and does not create missing parent directories: `Error(013): don't have access to write to ...` (exit 13) | manual — implementation constraint: append `.exe` for build/run/test outputs on Windows and `os.MkdirAll` `target/debug`, `target/debug/deps`, `target/release` before invoking Odin; note it in `docs/DECISIONS.md` (docs are self-consistent here only for `build`) |

## 5. Line endings and encoding

- `git ls-files --eol`: every tracked text file is `i/lf` + `w/lf`. The only exceptions are `example/src.exe` (`i/-text w/-text`, tracked binary) and `testdata/.gitattributes` (`i/none w/none`, empty file).
- Raw byte scan of all 61 files under `example/`, `testdata/` and the three root config files: no BOM anywhere; no CRLF anywhere; no lone CR.
- Files without a final LF (drifts AUD-006 to AUD-009): `.gitattributes`, `.editorconfig`, `example/.gitignore`, `testdata/golden/new/.gitignore`; `testdata/.gitattributes` is empty (0 B, AUD-005).
- The three fixtures created by this audit are UTF-8, LF-only, no BOM, final LF (verified by byte scan and `git diff`).
- Recorded only; no human-written file was converted.

## 6. Contradictions and ambiguities between documents and files

| ID | Contradiction |
|---|---|
| C-01 | README's golden fences contain a trailing blank line (woden.toml, src/main.odin, tests/main_test.odin) but `testdata/golden/new/` ends with a single LF; FIXTURES §3.2 says golden is byte-equal to README (AUD-010). |
| C-02 | FIXTURES §3.1 and README show `.gitignore` content ending at `/vendor/`; the actual `example/.gitignore` and `golden/new/.gitignore` have no final LF (AUD-008, AUD-009). |
| C-03 | BRIEF §1 says the root `.vscode/settings.json` is ready and must not be modified, but FIXTURES §3.1 specifies a 7-line file and the real file is a 58-line editor config with `//` comments (AUD-011). |
| C-04 | FIXTURES §3.1 explicitly says there is no separate `testdata/.gitattributes`, yet one exists (AUD-005). |
| C-05 | FIXTURES names the fixtures `valid-minimal.toml`, `valid-full.toml`, `valid.lock`; the repository shipped `vaild-*` variants (AUD-001 to AUD-003). |
| C-06 | FIXTURES §3.4 full texts of `testdata/toml/edit/*.toml` disagree with the actual files (missing comment line, comment spacing) — AUD-012. |
| C-07 | FIXTURES §3.6 requires tab indentation in `tabs.json`; the actual file is identical to `base.json` (spaces) — AUD-013. |
| C-08 | The `.vscode/settings.json` comment and the e2e expectation claim odinfmt output is LF, but odinfmt emits CRLF on Windows by default — AUD-014. |
| C-09 | BRIEF §4 test command `-out:target/debug/deps/<name>` omits the `.exe` suffix that Odin requires on Windows (AUD-015). |
| C-10 | README line 260: Woden does not generate or manage `odinfmt.json`, yet on Windows the fmt e2e "hashes unchanged" expectation can only hold if that file exists (AUD-014) — a test-harness decision is needed. |
| C-11 | FIXTURES §3.7 prose gives the import order of `example/tests/main_test.odin` as `core:testing`, `app:greet`, `libs:mathx`; the actual (odinfmt-sorted) order is `app:greet`, `core:testing`, `libs:mathx`. Behaviorally equivalent; no action needed. |
| C-12 | FIXTURES §3.6 `with-comments.json` expects OLS behavior explicitly marked "待实测" (OLS tolerance of comments/trailing commas); it remains unverified (§8). Tests must be able to skip or branch on the chosen behavior. |

"待实测" items that were unverified at spec-writing time are not counted as drift; they were resolved where possible in §7 and the rest are in §8.

## 7. Real Odin / odinfmt verification

All commands were run against TEMP copies of `example/` (`...\Temp\dsh-pEsowr\woden-audit\verify*`), never against the repository copies. Exit codes were read with `$LASTEXITCODE`.

### 7.1 Odin compile/run/test behavior

| # | Command (cwd = temp `example` copy) | Exit | Observed |
|---|---|---|---|
| 1 | `odin check src -vet -collection:app=src -collection:libs=libs` | 0 | no output; `-vet` clean |
| 2 | `odin test tests -all-packages -collection:app=src -collection:libs=libs` | 0 | 3 tests run in both packages ("Finished 3 tests ... All tests were successful."); output format is `INFO ...` / `WARN ...` lines; WARN memory-leak lines (8 B from `greet.test_hello_empty`, 24 B from `tests.test_hello_x`, leaked by `builder.odin`) are printed but do not fail the run |
| 3 | `odin run src ... -- a "b c"` | 0 | prints `Hello, woden-example!`, `2 + 3 = 5`, `arg[0]=a`, `arg[1]=b c` — `--` arguments arrive verbatim in `os.args[1:]` |
| 4 | `odin run src ... -- --exit 3` | 3 | `$LASTEXITCODE` = 3 — exit code is passed through by `odin run` |
| 5 | `odin check src` after introducing a type error | 1 | compiler error on stderr |
| 6 | `odin test tests` after breaking one expectation | 1 | "1 test failed." plus an `ERROR` line; still runs all 3 tests |
| 7 | `odin build src -out:target/debug/woden-example` (no extension) | 1 | `Output path ...must have an appropriate extension.` |
| 8 | `odin build src -out:target2/debug/foo.exe` (parent dir missing) | 13 | `Error(013): don't have access to write to ...`; Odin does not create parent directories |
| 9 | `odin build src -out:target/debug/woden-example.exe` (parent exists) | 0 | binary created, 600576 B |
| 10 | `odin run src -out:...` without `-keep-executable` | 0 | executable is deleted after the run |
| 11 | `odin run src -out:... -keep-executable` | 0 | executable is kept after the run — the README design (`run` shares the `build` artifact) is valid |
| 12 | `odin test tests -out:target/debug/deps/mytest.exe` | 0 | artifact is deleted by Odin after the test run (directory left empty) — matches README line 163 |
| 13 | `odin test ... -out:...` without `.exe` / with missing parent | 1 / 13 | same extension error / same Error(013) as build |
| 14 | `odin build src -o:warp` | 1 | `Invalid optimization mode for -o:<string>, got warp`; message lists valid values: `minimal, size, speed, aggressive, none (useful for -debug builds)` |
| 15 | `odin run src -collection:app=C:\abs\path\src ...` | 0 | Windows backslash absolute collection paths are accepted; forward-slash absolute paths also work |
| 16 | cwd probe: program reads a relative marker file | 0 | the program sees the invocation directory, not the package directory — `odin run`'s child cwd equals the caller's cwd (README claim confirmed) |
| 17 | `-vet` on a dependency: temp project with `vendor/dep` (unused import + unused local var) | 0 / 1 | `odin check src` without `-vet`: 0; with `-vet`: 1 and `'strings' declared but not used` / `'x' declared but not used` — `-vet` does reach `deps:` packages |
| 18 | `odin check testdata/deplib/rev1` … `rev4 -no-entry-point` (from repo root) | 0 ×4 | all four dependency templates compile |
| 19 | `odin build src -debug -o:none` | 0 | `-debug` and `-o:` combine as expected |

### 7.2 odinfmt behavior

| # | Command | Exit | Observed |
|---|---|---|---|
| 1 | `odinfmt src -w; odinfmt tests -w` (temp copy) | 0 / 0 | formats recursively: `src/main.odin`, `src/greet/greet.odin`, `src/greet/greet_test.odin` (3 files) and `tests/main_test.odin` (1 file); prints each absolute path plus `Formatted N files in ...` and `Peak memory used: 0`; `libs/` untouched; no formatting changes were needed (repo sources are already formatted) |
| 2 | same, but followed by hash comparison | — | **output was rewritten LF→CRLF**: `main.odin` 497→524 B (27 CRLF), `greet.odin` 116→123 B (7), `greet_test.odin` 138→146 B (8), `main_test.odin` 275→290 B (15); content is identical modulo EOL. This is the Windows default of OLS `printer.odin` (`newline_style = .CRLF` when `ODIN_OS == .Windows`), not a formatting difference — AUD-014 |
| 3 | `odinfmt src -w` / `odinfmt tests -w` with `odinfmt.json` = `{"newline_style":"LF"}` at the project root | 0 / 0 | all four files byte-identical to the repo (SHA-256 unchanged: `e3c67780…`, `93e37552…`, `ae6d31f4…`, `3676b929…`), 0 CRLF — workaround confirmed |
| 4 | config decoding probe (`{/* note */ ...}`, trailing comma, trailing `// note`, `newline_style` values) | 0 | with Odin's `core:encoding/json`: comments and trailing commas in the config are accepted; `"newline_style":"LF"` and the ordinal `1` select LF; `"lf"` and `"Newline_Style.LF"` are not decoded (fall back to CRLF). Use `{"newline_style":"LF"}` if a config is supplied |
| 5 | `odinfmt <dir> -w` on a directory containing a `.txt` file and a nested subdirectory | 0 | recurses into subdirectories, ignores non-`.odin` files (`- Formatted 2 files`) |
| 6 | `odinfmt <file>` without `-w` | 0 | prints the formatted source to stdout (Woden only needs `-w`) |
| 7 | `odinfmt -help` | 1 | `[Parse_Error.Missing_Flag] Unable to find any flag named -help`; the documented flags are `[path] [-config] [-stdin] [-w]` |

### 7.3 Other observations relevant to the implementation contract

- All Odin invocations require absolute collection paths in the `-collection:name=path` form; both `C:\...` and `C:/...` worked on this machine.
- Odin does not create `target/`, `target/debug/`, `target/debug/deps/` or `target/release/`; Woden must create them before `build`/`run`/`test` (AUD-015).
- `odin run`/`odin test` print their own output to stdout/stderr; the e2e tests must not assume an empty Odin stdout (also seen with the memory-leak WARN lines in test mode).
- `ols.exe --help` does not print help: it starts the LSP server on stdio and blocks, so OLS cannot be probed from a simple command line.

## 8. Unverified / not attempted

- OLS tolerance of `//` comments and trailing commas in `ols.json` (FIXTURES §3.6 `with-comments.json`): OLS only speaks LSP and would need a full initialize handshake; not attempted. The implementation must choose a behavior (preserve and merge, or reject safely) and tests must skip or branch accordingly.
- `odinfmt` behavior on a file with a real syntax error (the probe file was parse-valid with only a type error, and it was still formatted) — unknown whether `-w` writes or errors out.
- `odinfmt --stdin`, and `-config:<path>` with non-default enum spellings (only auto-discovered project-root `odinfmt.json` was verified).
- OLS protocol/initialize and the editor behavior of `ols.json`.
- Woden end-to-end behavior (implementation is in progress by the Lead): `clean` symlink safety, git dependency flows (`add`/`sync`/`update`/`remove`), and `internal/testutil.NewDepRepo` (FIXTURES §3.7) were not exercised by this audit.
- Windows paths containing spaces, UNC paths and long paths were not tested; only the program-argument quoting (`"b c"`) was verified.
- Stability of the exact `odin test` output format across Odin versions (FIXTURES marks this "待实测"); only `dev-2026-10-nightly` was observed.

## 9. Working tree after the audit

The audit added exactly these untracked files (Lead's own in-progress implementation changes are separate and kept changing while the audit ran):

```text
?? docs/FIXTURES_AUDIT.md
?? testdata/lock/valid.lock
?? testdata/toml/valid-full.toml
?? testdata/toml/valid-minimal.toml
```

No existing file was modified by this audit. While this audit was running, the Lead's in-progress work also left untracked `.probe-gitdaemon*/` directories and a file `hi` in the repository root; those are not audit artifacts and the Lead should decide whether to keep or remove them before committing. Per BRIEF §1.1 step 6, `docs/FIXTURES_AUDIT.md` and the three fixtures should be committed as the M0 commit before M1.
