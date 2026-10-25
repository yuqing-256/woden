# Woden 测试资源规格（example/ 与 testdata/）

> 使用方式：保存为仓库内 `docs/FIXTURES.md`。`example/` 中的 Odin 源码由人在装有真实 Odin 的机器上编写并验证；`testdata/` 中的非 Odin 文件可由 DSH 按本规格生成，已列出的文件不得改动。

## 一、命名与布局

- **`testdata/` 保留此名**：Go 工具链会忽略名为 `testdata` 的目录（`go build/vet/test ./...` 不会进入），这是 Go 社区的通行做法。`mock/` 暗示“模拟对象”，而这里放的是静态 fixture；`tests/` 会与 Odin 项目里的 `tests/` 目录以及 Go 的 `_test.go` 混淆。若仍想改名，`fixtures/` 是语义最清楚的备选，但它没有 Go 的特殊对待。
- fixture 集中放在仓库根的 `testdata/`，测试通过 `internal/testutil`（沿 `go.mod` 向上找仓库根）取路径，不写 `../../` 相对路径。
- `example/` 是可直接运行的示例项目（只有一个，所以不叫 `examples/`）。

```text
.gitattributes            # 仓库根：* text=auto eol=lf
.editorconfig             # 仓库根
.vscode/settings.json     # 仓库根（是否提交由你决定）
docs/BRIEF.md
docs/FIXTURES.md
example/
  .gitignore
  README.md
  woden.toml
  ols.json
  src/main.odin
  src/greet/greet.odin
  src/greet/greet_test.odin
  tests/main_test.odin
  libs/mathx/mathx.odin
testdata/
  README.md
  golden/new/            # woden new / init 的期望输出
  toml/                  # 配置解析 fixture
  toml/edit/             # woden.toml 文本编辑 fixture
  lock/                  # woden.lock fixture
  ols/                   # ols.json 合并 fixture
  deplib/rev1..rev4/     # 依赖仓库的源码模板
```

## 二、example/

原则：初始状态下所有命令都能成功；不提交 `target/`、`vendor/`、`woden.lock` 和任何 `[dependencies]`。所有 Odin 文件用 tab 缩进，经 `odinfmt` 格式化后无差异，`-vet` 干净（没有未使用的导入和变量）。标准库 API（如参数与退出函数所在的包）随 Odin 版本可能变化，请以你使用的版本为准。

### 逐文件规格

**`example/.gitignore`**

```gitignore
/target/
/vendor/
```

**`example/README.md`**（全文）：

```markdown
# woden-example

Woden 的测试用示例项目，用来验证 `woden` 各命令的行为。

**不要在此目录内直接运行会写入 `target/`、`vendor/` 的 woden 命令**（如 `build`、`run`、`test`、`add`、`sync`）。请先把本目录复制到临时目录再操作；仓库里的测试会自动这样做。

## 内容

- `src/main.odin`：入口。打印问候和 `2 + 3 = 5`，再逐行打印收到的参数；参数中出现 `--exit N` 时以退出码 N 退出。
- `src/greet/`：子包，通过 `app:greet` 导入，含一个包内测试。
- `libs/mathx/`：本地 collection `libs` 中的包，通过 `libs:mathx` 导入。
- `tests/main_test.odin`：测试包，`woden test` 时与 `src/greet` 的测试一起运行（共 3 个测试）。

初始状态下不含 `target/`、`vendor/`、`woden.lock` 和任何依赖。
```

**`example/woden.toml`**

```toml
[package]
name = "woden-example"
version = "0.1.0"

[collections]
app = "src"
libs = "libs"
```

**`example/ols.json`**

```json
{
  "$schema": "https://raw.githubusercontent.com/DanielGavin/ols/master/misc/ols.schema.json",
  "collections": [
    { "name": "app", "path": "src" },
    { "name": "libs", "path": "libs" }
  ],
  "checker_args": "-vet",
  "profile": "default",
  "profiles": [{ "name": "default", "checker_path": ["src", "tests"] }]
}
```

**`example/src/main.odin`**（参考实现；标准库 API 以你的 Odin 版本为准，关键是输出格式必须与下面一致）：

```odin
package main

import "core:fmt"
import "core:os"
import "core:strconv"

import "app:greet"
import "libs:mathx"

main :: proc() {
	fmt.println(greet.hello("woden-example"))
	fmt.printfln("2 + 3 = %d", mathx.add(2, 3))

	args := os.args[1:]
	exit_code := 0
	for arg, i in args {
		fmt.printfln("arg[%d]=%s", i, arg)
		if arg == "--exit" && i + 1 < len(args) {
			if code, ok := strconv.parse_int(args[i + 1], 10); ok {
				exit_code = code
			}
		}
	}
	if exit_code != 0 {
		os.exit(exit_code)
	}
}
```

行为规格：

- `package main`，导入 `app:greet` 与 `libs:mathx` 及所需的标准库。
- 第 1 行输出 `Hello, woden-example!`（来自 `greet.hello("woden-example")`）。
- 第 2 行输出 `2 + 3 = 5`（来自 `mathx.add(2, 3)`）。
- 之后对程序收到的每个参数（不含程序名）按顺序输出一行 `arg[<i>]=<value>`，`i` 从 0 开始。
- 若某个参数是 `--exit` 且后面跟一个整数 N，则在输出完所有 `arg` 行之后以退出码 N 退出；否则退出码为 0。
- 不读 stdin，不依赖当前工作目录，不输出时间等不确定内容。
- 每一行都要用格式化函数拼好再输出（例如 `fmt.printfln("arg[%d]=%s", i, arg)`；若所用 Odin 版本没有 `printfln`，用 `fmt.printf` 并自带换行）。`fmt.println` 会在多个参数之间自动加空格，会得到 `2 + 3 =  5`、`arg[ 0 ]= --exit` 这样的输出，不符合规格。

**`example/src/greet/greet.odin`**：`package greet`；导出 `hello :: proc(name: string) -> string`，返回 `Hello, <name>!`（可以使用临时分配器，调用方在同一语句内使用）。

**`example/src/greet/greet_test.odin`**：`package greet`；一个 `@(test)`：断言 `hello("") == "Hello, !"`。作用：验证 `-all-packages` 会运行被导入包里的测试。

**`example/libs/mathx/mathx.odin`**：`package mathx`；导出 `add :: proc(a, b: int) -> int`。

**`example/tests/main_test.odin`**：`package tests`；导入 `core:testing`、`app:greet`、`libs:mathx`；两个 `@(test)`：`hello("x") == "Hello, x!"`，`add(2, 3) == 5`。预期在 `odin test tests -all-packages` 下共运行 3 个测试并全部通过。

### 写完后的人工验证清单

```powershell
cd example
odinfmt src -w; odinfmt tests -w          # 之后 git diff 应为空
odin check src -vet -collection:app=src -collection:libs=libs
odin run src -collection:app=src -collection:libs=libs -- a "b c"
odin run src -collection:app=src -collection:libs=libs -- --exit 3; $LASTEXITCODE   # 应为 3
odin test tests -all-packages -collection:app=src -collection:libs=libs
```

验证点：① 输出符合上述规格。PowerShell 里 `$?` 只表示成功或失败（失败时为 `False`），退出码要看 `$LASTEXITCODE`；bash 里用 `$?`。② `--exit 3` 的退出码是否被 `odin run` 透传（`$LASTEXITCODE` 是否为 3）。③ 测试数量和输出格式。④ `tests/` 导入 `app:greet` 是否可用。结果写入 `docs/DECISIONS.md` 的“待实测”清单。已验证：`odin run` 会把 `--` 之后的参数原样传给程序，程序里用 `os.args[1:]` 即可去掉程序名。

## 三、testdata/

### 3.1 换行、编辑器与根部文件

以下三个文件放在**仓库根**，不再单独放 `testdata/.gitattributes`：根部的规则覆盖整个仓库，包括 `example/` 里的 Odin 文件。

**`.gitattributes`**

```gitattributes
# 仓库内的文本文件一律使用 LF，检出时也是 LF。
# 否则 Windows 的 autocrlf 会改写 fixture 与 golden 文件，字节级比对会失败。
* text=auto eol=lf
```

**`.editorconfig`**

```ini
root = true

[*]
charset = utf-8
end_of_line = lf
```

**`.vscode/settings.json`**

```json
{
  "files.eol": "\n",
  "files.associations": {
    "**/testdata/**": "plaintext"
  }
}
```

- `files.eol` 让 VS Code 新建的文件使用 LF（编辑器默认是 CRLF 时必须设置）。`odinfmt` 和 `woden new` 的输出都是 LF，混入 CRLF 会让“格式化后无差异”和 golden 比对失败。
- `files.associations` 把 `testdata/` 下的所有文件当作纯文本。fixture 里有故意写错的 TOML / JSON（如 `bad-syntax.toml`、`bad-syntax.json`），TOML 和 JSON 插件的校验会报错；保存时格式化还会改写它们（例如压缩注释前的多余空格），破坏 fixture。这样不用修改文件后缀。用别的编辑器时，需要自行关闭对 `testdata/` 的校验与格式化。
- 已有文件的迁移：加入 `.gitattributes` 后执行 `git add --renormalize .` 并提交；工作区干净（没有未提交改动）时，再执行 `git rm --cached -r .` 和 `git reset --hard`，让所有文件按 LF 重新检出。

**`testdata/README.md`**（全文）：

```markdown
# testdata

Woden 的测试 fixture。Go 工具链会忽略名为 `testdata` 的目录。

- 已列出的文件是测试的期望值，不要随手改动；规格见 `docs/FIXTURES.md`。
- 所有文本文件使用 LF 换行，由仓库根的 `.gitattributes` 保证。需要 CRLF 的用例在测试里由 LF 内容转换得到，不单独存放文件。
- 其中一些文件故意是非法的 TOML / JSON，编辑器的校验和保存时格式化会破坏它们；VS Code 里本目录按纯文本处理（见 `.vscode/settings.json`）。

## 子目录

- `golden/new/`：`woden new <tmp>/my-game` 的期望输出，逐字节比对。
- `toml/`：`woden.toml` 的解析与校验用例；`toml/edit/`：文本级编辑的输入与期望输出。
- `lock/`：`woden.lock` 的解析用例。
- `ols/`：`ols.json` 的合并用例。
- `deplib/rev1`–`rev4`：依赖仓库的源码模板；测试在临时目录里用 git 构造仓库，不要直接在这里运行 git。
```

### 3.2 golden/new/

五个文件：`.gitignore`、`woden.toml`、`ols.json`、`src/main.odin`、`tests/main_test.odin`。内容逐字等于 README “初始化生成的文件”一节，项目名固定为 `my-game`。测试运行 `woden new <tmp>/my-game` 后逐字节比对。它与 scaffold 的 `embed` 模板分开维护，目的是模板被误改时能被发现。

### 3.3 toml/（解析与校验，全部 UTF-8、LF）

除特别说明外，每个文件都以下面三行开头，再加下表所列的内容：

```toml
[package]
name = "my-game"
version = "0.1.0"
```

| 文件                           | 附加内容                                                                   | 期望                                                                                                          |
| ------------------------------ | -------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------- |
| `valid-minimal.toml`           | `[collections]` + `app = "src"`                                            | 解析成功，无警告                                                                                              |
| `valid-full.toml`              | 见下方完整内容                                                             | 解析成功；dev 的 opt-level 为 none，release 的 debug 为 true；四个依赖的引用类型依次为 branch、tag、rev、默认 |
| `unknown-key.toml`             | `[package]` 下额外 `edition = "2026"`，并有顶层表 `[foo]`                  | 解析成功，警告 2 条（`package.edition`、`foo`）                                                               |
| `bad-syntax.toml`              | **不用上面三行开头**，整个文件见下方                                       | 错误，定位到第 2 行                                                                                           |
| `bad-profile-name.toml`        | `[profile.custom]` + `opt-level = "none"`                                  | 错误：未知 profile                                                                                            |
| `bad-opt-level.toml`           | `[profile.dev]` + `opt-level = "warp"`                                     | 错误：非法值（允许值以对 `-o:` 的实测为准）                                                                   |
| `bad-debug-type.toml`          | `[profile.dev]` + `debug = "yes"`                                          | 错误：类型不符                                                                                                |
| `bad-multi-ref.toml`           | `[dependencies.alpha]`，`git` 加 `branch` 和 `tag`                         | 错误：引用互斥                                                                                                |
| `bad-dep-no-git.toml`          | `[dependencies.alpha]` + `branch = "main"`                                 | 错误：缺少 git                                                                                                |
| `bad-dep-name.toml`            | `[dependencies."../evil"]` + `git = "https://github.com/example/evil.git"` | 错误：非法依赖名                                                                                              |
| `bad-collection-reserved.toml` | `[collections]` + `core = "src"`                                           | 错误：保留名                                                                                                  |
| `bad-collection-escape.toml`   | `[collections]` + `app = "../outside"`                                     | 错误：越出项目根                                                                                              |
| `bad-collection-abs.toml`      | `[collections]` + `app = "/etc"`                                           | 错误：需要相对路径                                                                                            |
| `bad-name.toml`                | **不用上面三行开头**：`[package]`、`name = "1game"`、`version = "0.1.0"`   | 错误：项目名不合法                                                                                            |
| `missing-package.toml`         | **不用上面三行开头**，只有 `[collections]` 与 `app = "src"`                | 错误：缺少 `[package]`                                                                                        |

`bad-syntax.toml` 全文（第 2 行字符串未闭合）：

```toml
[package]
name = "my-game
version = "0.1.0"
```

`valid-full.toml` 全文：

```toml
# Full example
[package]
name = "my-game"
version = "0.1.0"

[collections]
app = "src"
libs = "libs"
deps = "vendor"

[profile.dev]
opt-level = "none"

[profile.release]
debug = true

[dependencies.alpha]
git = "https://github.com/example/alpha.git"
branch = "main"

[dependencies.beta]
git = "https://github.com/example/beta.git"
tag = "v1.0.0"

[dependencies.gamma]
git = "https://github.com/example/gamma.git"
rev = "0123456789abcdef0123456789abcdef01234567"

[dependencies.delta]
git = "https://github.com/example/delta.git"
```

### 3.4 toml/edit/（文本级编辑，保留注释与格式）

编辑器操作：`AddDependency`、`RemoveDependency`、`SetCollection`、`RemoveCollection`。规则：新增的依赖段追加在文件末尾，前面空一行；删除依赖段时，紧邻其上的注释行和该段前的一个空行一并删除；文件以单个换行结尾。

**`base.toml`**

```toml
# my project
[package]
name = "my-game"   # keep this comment
version = "0.1.0"

[collections]
app = "src"
```

**`add-first-dep.want.toml`**：对 `base.toml` 执行 `SetCollection("deps", "vendor")` 与 `AddDependency("example-library", git, branch="main")` 的期望结果：

```toml
# my project
[package]
name = "my-game"   # keep this comment
version = "0.1.0"

[collections]
app = "src"
deps = "vendor"

[dependencies.example-library]
git = "https://github.com/example/example-library.git"
branch = "main"
```

**`two-deps.toml`**

```toml
# my project
[package]
name = "my-game"   # keep this comment
version = "0.1.0"

[collections]
app = "src"
deps = "vendor"

# first dependency
[dependencies.alpha]
git = "https://github.com/example/alpha.git"
branch = "main"

# second dependency
[dependencies.beta]
git = "https://github.com/example/beta.git"
tag = "v1.0.0" # pinned
```

**`remove-alpha.want.toml`**：对 `two-deps.toml` 执行 `RemoveDependency("alpha")` 的期望结果：

```toml
# my project
[package]
name = "my-game"   # keep this comment
version = "0.1.0"

[collections]
app = "src"
deps = "vendor"

# second dependency
[dependencies.beta]
git = "https://github.com/example/beta.git"
tag = "v1.0.0" # pinned
```

**CRLF 用例**：不单独存放 CRLF 文件。测试运行时用 `internal/testutil.ToCRLF`，把 `base.toml` 与 `add-first-dep.want.toml` 的内容里的 `\n` 全部换成 `\r\n`，分别作为输入和期望；验证编辑器保持文件原有的换行风格（CRLF 文件改完仍是 CRLF，新增的行也是 CRLF）。这样 git 和编辑器的换行转换不会影响这个用例。

**`bad-structure.toml`**：`collections` 用内联表写法，编辑器无法安全修改，期望拒绝编辑并报错、文件内容不变：

```toml
[package]
name = "my-game"
version = "0.1.0"

collections = { app = "src" }
```

### 3.5 lock/

**`valid.lock`**（按 name 排序；`rev` 保存用户请求的（可缩写的）引用，`commit` 始终是完整 ID）：

```toml
version = 1

[[package]]
name = "alpha"
git = "https://github.com/example/alpha.git"
branch = "main"
commit = "0123456789abcdef0123456789abcdef01234567"

[[package]]
name = "beta"
git = "https://github.com/example/beta.git"
tag = "v1.0.0"
commit = "89abcdef0123456789abcdef0123456789abcdef"

[[package]]
name = "delta"
git = "https://github.com/example/delta.git"
rev = "fedcba9"
commit = "fedcba9876543210fedcba9876543210fedcba98"

[[package]]
name = "gamma"
git = "https://github.com/example/gamma.git"
commit = "1111111111111111111111111111111111111111"
```

其他文件（都以 `version = 1` 开头，除非特别说明）：

| 文件                   | 内容要点                                | 期望                       |
| ---------------------- | --------------------------------------- | -------------------------- |
| `unknown-version.lock` | `version = 2` + 一个合法 `[[package]]`  | 报错：不支持的版本         |
| `bad-syntax.lock`      | `[[package]]` 下字符串未闭合            | 报错并定位行号，不重写     |
| `missing-commit.lock`  | `[[package]]` 有 name、git，没有 commit | 报错                       |
| `dup-name.lock`        | 两个 `name = "alpha"` 的 `[[package]]`  | 报错：重复依赖             |
| `short-commit.lock`    | `commit` 只有 7 位                      | 报错：lock 中必须是完整 ID |

### 3.6 ols/

**`base.json`**：与 README 的 `ols.json` 相同（仅 `app` 一个 collection）。

**`with-deps.json`**：同 `base.json`，但 `collections` 数组中在 `app` 之后增加 `{ "name": "deps", "path": "vendor" }`。

**`stale-app.json`**：同 `base.json`，但 `app` 的 `path` 为 `old-src`。

**`custom.json`**（4 空格缩进，带 OLS 独有的设置）：

```json
{
  "$schema": "https://raw.githubusercontent.com/DanielGavin/ols/master/misc/ols.schema.json",
  "collections": [
    { "name": "app", "path": "src" },
    { "name": "ols-only", "path": "/opt/ols-only" }
  ],
  "checker_args": "-vet -strict-style",
  "enable_document_symbols": true,
  "profile": "default",
  "profiles": [
    { "name": "default", "checker_path": ["src", "tests"] },
    { "name": "extra", "checker_path": ["libs"] }
  ]
}
```

**`custom.add-libs.want.json`**：对 `custom.json` 合并 `app=src, libs=libs` 的期望结果——除 `collections` 数组末尾追加一项 `{ "name": "libs", "path": "libs" }`（格式与同级项一致，4 空格缩进）外，其余字节与 `custom.json` 相同。

**`tabs.json`**：内容同 `base.json`，但缩进全部用 tab。

**`bad-syntax.json`**：截断的 JSON（缺少结尾 `}`）。

**`with-comments.json`**：同 `base.json`，并在 `collections` 前有一行 `// note` 注释。

合并用例：

| 输入                 | woden.toml 的 collections | 期望                                                                             |
| -------------------- | ------------------------- | -------------------------------------------------------------------------------- |
| `base.json`          | `app=src`、`deps=vendor`  | 等于 `with-deps.json`                                                            |
| `with-deps.json`     | `app=src`（没有 `deps`）  | 等于 `base.json`（`deps` 完全托管，会被删除）                                    |
| `stale-app.json`     | `app=src`                 | 等于 `base.json`（同名以 toml 为准）                                             |
| `custom.json`        | `app=src`、`libs=libs`    | 等于 `custom.add-libs.want.json`（`ols-only` 与其他键保留）                      |
| `custom.json`        | `app=src`                 | 与 `custom.json` 完全相同                                                        |
| `tabs.json`          | `app=src`、`libs=libs`    | 追加 `libs`，其余 tab 缩进保持                                                   |
| `bad-syntax.json`    | 任意                      | 报错，文件内容不变                                                               |
| `with-comments.json` | 任意                      | 待实测（OLS 对注释的容忍度）：要么保留注释并正常合并，要么明确拒绝；不得丢失注释 |
| （文件不存在）       | 任意                      | 不创建文件，返回提示                                                             |

### 3.7 deplib/ 与依赖仓库 helper

`testdata/deplib/rev1` 到 `rev4` 各有一个 `deplib.odin`，四个文件只有返回值不同。构造依赖仓库时，helper 把对应版本的文件复制到仓库根目录，命名为 `deplib.odin`；依赖加入后是 `vendor/deplib/deplib.odin`，可以用 `deps:deplib` 导入。

**`rev1/deplib.odin`**

```odin
package deplib

version :: proc() -> string {
	return "0.1.0"
}
```

**`rev2/deplib.odin`**

```odin
package deplib

version :: proc() -> string {
	return "0.2.0"
}
```

**`rev3/deplib.odin`**

```odin
package deplib

version :: proc() -> string {
	return "0.3.0-dev"
}
```

**`rev4/deplib.odin`**

```odin
package deplib

version :: proc() -> string {
	return "0.2.1"
}
```

写完后在有 Odin 的机器上逐个验证（对 `rev2` 到 `rev4` 同理）：

```powershell
odin check testdata/deplib/rev1 -no-entry-point
```

`internal/testutil.NewDepRepo(t)` 在临时目录里用真实 git 构造如下仓库（仅在带 `woden_testing` 标签时可用）：

- 默认分支名为 `trunk`（不是 `main`）。
- C1：内容 `rev1`，打 **annotated** tag `v0.1.0`。
- C2：内容 `rev2`，打 lightweight tag `v0.2.0`。
- 分支 `dev` 自 C2，其上提交 C3（内容 `rev3`）。
- 最后回到 `trunk`（指向 C2）。

返回 `DepRepo`：`URL`（`file://` 开头）、`Dir`、`C1`、`C2`、`C3`、`DefaultBranch`。方法：`AdvanceTrunk(t)`（在 `trunk` 上提交内容 `rev4` 并返回新提交 ID，用于测试 `update`）、`MoveTag(t, tag, commit)`（强制移动 tag，用于测试 tag 依赖的 `update`）。

确定性：设置 `GIT_CONFIG_GLOBAL` 与 `GIT_CONFIG_SYSTEM` 为空设备，固定 `GIT_AUTHOR_*` / `GIT_COMMITTER_*` 的姓名、邮箱和日期，关闭 `commit.gpgsign`。缩写 `--rev` 的用例用 `C1[:7]`。

端到端用到此仓库时，测试代码在临时副本里额外写一个 `src/dep_probe.odin`：`package main`，导入 `deps:deplib`，定义一个返回 `deplib.version()` 的过程；`add` 之前 `woden check` 应失败，`add` 之后应成功。

## 四、端到端期望与变异用例

以下都在 `example/` 的临时副本上运行（真实 Odin、odinfmt、git，构建标签 `e2e`）。

| 命令                        | 期望                                                                                             |
| --------------------------- | ------------------------------------------------------------------------------------------------ |
| `woden check`、`woden lint` | 退出码 0                                                                                         |
| `woden test`                | 退出码 0，3 个测试通过（输出格式待实测）                                                         |
| `woden build`               | 退出码 0，`target/debug/woden-example`（Windows 带 `.exe`）存在                                  |
| `woden build --release`     | 退出码 0，`target/release/woden-example` 存在                                                    |
| `woden run`                 | 退出码 0，输出前两行为 `Hello, woden-example!` 和 `2 + 3 = 5`；`target/debug/woden-example` 保留 |
| `woden run -- a "b c"`      | 额外输出 `arg[0]=a` 和 `arg[1]=b c`                                                              |
| `woden run -- --exit 3`     | 退出码 3                                                                                         |
| `woden fmt`                 | 退出码 0，所有文件内容不变（比较哈希）                                                           |
| `woden sync --check`        | 退出码 0                                                                                         |
| `woden clean --dry-run`     | 退出码 0，列出 `target/` 内条目，文件不变                                                        |
| `woden clean`               | `target/` 被删除，其余文件哈希不变                                                               |

变异用例（每个用例从全新的副本开始）：

| 变异                                                                                      | 期望                                                                          |
| ----------------------------------------------------------------------------------------- | ----------------------------------------------------------------------------- |
| 把 `src/greet/greet.odin` 和 `libs/mathx/mathx.odin` 的格式同时弄乱                       | `woden fmt` 只改写 `greet.odin`，`libs/` 不变                                 |
| 在 `greet.odin` 引入类型错误                                                              | `woden check` 退出码 101                                                      |
| 引入未使用的变量                                                                          | `woden lint` 退出码 101，`woden check` 为 0（依赖 `-vet` 的实际行为，待实测） |
| 把 `tests/main_test.odin` 的期望值改错                                                    | `woden test` 退出码 101                                                       |
| 从 `woden.toml` 删除 `libs` 映射                                                          | `woden check` 退出码 101                                                      |
| 在 `example` 副本中按 toml 里的 `deps` 映射但不创建 `vendor/`                             | `woden check` 报错并提示运行 `woden sync`                                     |
| 用 `NewDepRepo` 的 URL 依次 `add`（默认、`--branch dev`、`--tag v0.1.0`、`--rev C1[:7]`） | 都成功；lock 记录完整提交 ID；写入 `dep_probe.odin` 后 `woden check` 成功     |
| `add` 后删除 `vendor/` 再 `woden sync`                                                    | `check` 先提示运行 sync，sync 后恢复，提交与 lock 一致                        |
| `AdvanceTrunk` 后 `woden update`                                                          | 默认引用依赖前进到新提交；`rev` 依赖不变；指名 `rev` 依赖报错                 |
| 在 `vendor/<dep>` 里修改一个文件后 `sync` / `update` / `remove`                           | 都拒绝处理该依赖，文件不被覆盖或删除                                          |
| `remove` 最后一个依赖                                                                     | `deps` 映射、`vendor/`、`ols.json` 的 `deps` 条目都被移除                     |
