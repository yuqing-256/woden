# Woden

Woden 是一个使用 Go 和 Cobra 编写的 Odin 项目管理工具，借鉴 Cargo 的项目工作流，为 Odin 项目统一提供初始化、格式化、检查、测试、构建和 Git 依赖管理。

> **项目状态：** 开发中。本文档描述第一版的目标行为，尚不代表所有功能都已实现。

Woden 采用约定优先的设计：项目目录固定，常用操作使用默认配置；`woden.toml` 是项目配置的权威来源，`woden.lock` 固定 Git 依赖的实际提交。Woden 解析自己的参数，不直接暴露 Odin 的全部命令行选项。除非与 Odin 的要求冲突，Woden 的行为遵循 Cargo。命令行帮助和错误信息使用英文。

## 环境要求

- [Go](https://go.dev/)：构建 Woden。
- [Odin](https://odin-lang.org/)：检查、测试、构建和运行项目。
- [Git](https://git-scm.com/)：管理 Git 依赖。
- [OLS](https://github.com/DanielGavin/ols)：可选，用于编辑器语言服务；其项目同时提供 `odinfmt`。

Odin、Git 和 `odinfmt` 通过系统 `PATH` 查找。

## 安装

```sh
go install github.com/yuqing-256/woden@latest
```

安装后的二进制位于 `%GOBIN%`，或默认的 `%USERPROFILE%\go\bin`（Windows）；Linux/macOS 默认位于 `$HOME/go/bin`。请将对应目录加入 `PATH`。所需 Go 版本见 `go.mod`。

## 快速开始

创建新项目：

```sh
woden new my-game
cd my-game
woden run
```

进入已有的 Odin 项目目录并初始化：

```sh
woden init
```

## 项目与目录约定

```text
.
├── .gitignore
├── ols.json
├── woden.toml
├── woden.lock       # 存在 Git 依赖时生成并提交
├── src/
│   └── main.odin
├── tests/
│   └── main_test.odin
├── libs/            # 可选：项目自行维护的本地源码
├── vendor/          # 按需创建：Woden 管理的 Git 工作副本
└── target/          # 按需创建：构建产物
```

- `woden new PATH` 创建新项目；`PATH` 必填，目标目录必须不存在或为空。项目名默认取目录名，须以字母或下划线开头，其余字符为字母、数字、`-` 或 `_`，且不得是 Windows 保留设备名（如 `con`、`nul`）；不合法时报错，需先修改目录名。
- `woden init [PATH]` 初始化已有目录；省略路径时使用当前目录，指定路径必须已存在。项目名规则同上。
- 初始化会创建缺少的默认文件，但不会覆盖已有源码或配置。若目标根目录已有 `woden.toml`，则报错，避免重复初始化。
- Woden 不自动初始化 Git 仓库。
- 普通项目命令从当前目录向上查找最近的 `woden.toml`，因此可在项目子目录中运行。`new` 和 `init` 直接使用指定路径，不进行向上查找。
- `.gitignore` 默认忽略 `/target/` 和 `/vendor/`。`woden.lock` 应提交；`libs/` 属于项目自身源码，不默认忽略。

### 初始化生成的文件

`woden new` 与 `woden init` 生成下列文件，`my-game` 取自项目名；`init` 只创建缺失的文件：

- 已有 `.gitignore` 时，只追加缺失的 `/target/`、`/vendor/` 行，不改动其他内容。
- `src/` 内已有 `.odin` 文件时，不创建 `src/main.odin`；`tests/` 内已有 `.odin` 文件时，不创建 `tests/main_test.odin`。
- 已有 `ols.json` 时不改动。

`.gitignore`：

```gitignore
/target/
/vendor/
```

`woden.toml`：

```toml
[package]
name = "my-game"
version = "0.1.0"

[collections]
app = "src"

```

`ols.json`：

```json
{
  "$schema": "https://raw.githubusercontent.com/DanielGavin/ols/master/misc/ols.schema.json",
  "collections": [{ "name": "app", "path": "src" }],
  "checker_args": "-vet",
  "profile": "default",
  "profiles": [{ "name": "default", "checker_path": ["src", "tests"] }]
}
```

`src/main.odin`：

```odin
package main

import "core:fmt"

main :: proc() {
	fmt.println("Hello, world!")
}

```

`tests/main_test.odin`：

```odin
package tests

import "core:testing"

@(test)
test_sanity :: proc(t: ^testing.T) {
	testing.expect_value(t, 1 + 1, 2)
}

```

## 配置

项目配置文件为 `woden.toml`。最小示例：

```toml
[package]
name = "my-game"
version = "0.1.0"

[collections]
app = "src"
```

`woden new` 根据目录名生成 `package.name`。`package.version` 是项目元数据，与 Woden 工具自身的版本无关。`woden.toml` 中未知的键会给出警告并被忽略，无效的值会报错。Woden 修改 `woden.toml` 时保留原有的注释和格式。

### 构建 profile

Woden 内置 `dev` 和 `release`，目录和默认值固定：

| 项目        | `dev`           | `release`         |
| ----------- | --------------- | ----------------- |
| `debug`     | `true`          | `false`           |
| `opt-level` | `minimal`       | `speed`           |
| 输出目录    | `target/debug/` | `target/release/` |

可以只覆盖需要修改的字段：

```toml
[profile.dev]
opt-level = "none"
```

`debug` 映射到 Odin 的 `-debug`，`opt-level` 映射到 `-o:` 选项，允许值与 Odin 的 `-o:` 一致。`debug` 与优化等级相互独立。只能覆盖 `dev` 和 `release`，其他 profile 名称会报错。

- `woden build` 默认使用 `dev`。
- `woden build --release` 使用 `release`。
- `woden run` 使用 `dev`；第一版不提供 `run --release`。
- `woden test` 使用 `dev` 的编译选项。

`woden build` 的输出文件以 `package.name` 命名，放在 profile 的输出目录下，并采用目标操作系统对应的可执行文件格式（Windows 下带 `.exe`）。`woden run` 使用 `odin run`，通过 `-out:` 指定与 `woden build`（`dev`）相同的 `target/debug/<package.name>` 并加上 `-keep-executable`，因此 `build` 和 `run` 共用同一个产物，运行后不会被删除。`woden test` 使用 `odin test`，只通过 `-out:` 把可执行文件指定为 `target/debug/deps/<package.name>`，由 Odin 在运行结束后删除。第一版不支持自定义目录、输出路径或 profile 名称。

### Collections

`[collections]` 将 Odin collection 名称映射到目录；相对路径以项目根目录为基准。Woden 会在需要时统一生成 Odin 的 `-collection:name=path` 参数。

```toml
[collections]
app = "src"
libs = "libs"
```

collection 名称不得为 Odin 内置的 `core`、`base`、`vendor`；路径必须是项目根目录内的相对路径。

初始化时只写入 `app = "src"`。`libs/` 是可选的本地源码目录，不会自动创建、扫描或注册。需要通过 collection 导入本地库时，再显式配置映射。

Git 依赖统一放在 `vendor/<name>/`，并通过单一的 `deps = "vendor"` collection 访问。添加第一个 Git 依赖时，Woden 会按需增加该映射；若现有 `deps` 指向其他路径，则报告冲突。项目目录中的 `vendor/` 与 Odin 内置的 `vendor:` collection 不同。

## 命令

| 命令                       | 行为                                                |
| -------------------------- | --------------------------------------------------- |
| `woden new PATH`           | 创建新项目                                          |
| `woden init [PATH]`        | 初始化已有目录                                      |
| `woden fmt`                | 使用 `odinfmt` 格式化 `src/` 和 `tests/`            |
| `woden check`              | 检查 `src/`                                         |
| `woden lint`               | 检查 `src/` 并启用 Odin `-vet`                      |
| `woden test`               | 测试 `tests/`，启用 `-all-packages`                 |
| `woden build [--release]`  | 构建开发版或发布版                                  |
| `woden run [-- ARGS...]`   | 运行开发版；`--` 后的参数传给程序                   |
| `woden clean [--dry-run]`  | 清理或预览清理 `target/`                            |
| `woden add NAME --git URL` | 添加 Git 依赖                                       |
| `woden remove NAME`        | 移除 Git 依赖                                       |
| `woden sync [--check]`     | 同步依赖和受管理的 OLS collection，或只读检查一致性 |
| `woden update [NAME...]`   | 更新适用的 Git 依赖提交                             |

Woden 只接受上述命令定义的选项，不提供任意 Odin 或 `odinfmt` 参数透传。第一版不支持 `fmt --check`；`odin check` 不支持构建输出选项，因此 `check` 和 `lint` 不提供输出参数。

`woden run` 即 `odin run`：编译并运行开发版，程序的工作目录是调用 `woden` 时所在的目录，`--` 之后的参数原样传给程序。命令成功时退出码为 0；`build`、`check`、`lint`、`test`、`fmt` 等命令失败，或 Woden 自身出错时退出码为 101。只有 `woden run` 返回 `odin run` 的退出码，也就是被运行程序自身的退出码；编译失败时是 Odin 返回的非零退出码。

`clean` 只清理 `target/`（包括 `target/debug/deps/` 中被中断后残留的临时可执行文件），不删除源码、配置、锁文件或 `vendor/`。`--dry-run` 不修改文件；清理实现必须防止路径越界及符号链接导致的误删。

## Git 依赖

第一版只支持 Git 依赖，不提供注册表、版本范围、传递依赖或独立的本地路径依赖管理。

添加依赖时名称在前，来源选项在后：

```sh
woden add example-library --git https://github.com/example/example-library.git
woden add example-library --git https://github.com/example/example-library.git --branch main
woden add example-library --git https://github.com/example/example-library.git --tag v1.0.0
woden add example-library --git https://github.com/example/example-library.git --rev COMMIT
```

`--rev` 可以是完整或缩写的提交 ID。依赖名已存在时，`add` 报错。

manifest 示例：

```toml
[dependencies.example-library]
git = "https://github.com/example/example-library.git"
branch = "main"
```

每个依赖必须声明 Git 来源；`branch`、`tag`、`rev` 最多指定一种。省略引用时跟踪远端默认分支。URL 不得以 `-` 开头，也不使用 git 的 `ext::` 传输。

- `woden.toml` 声明依赖来源和请求的引用。
- `woden.lock` 记录解析出的完整提交 ID，并应提交到版本控制。
- `vendor/` 保存依赖工作副本，默认忽略，可按锁文件重新获取。
- `sync` 按锁文件恢复状态，不因远端分支变化自动更新提交。克隆项目后，先运行 `woden sync` 恢复 `vendor/`；`vendor/` 缺失时，需要 collection 的命令会提示运行 `woden sync`。含本地修改的工作副本不会被覆盖：该依赖被跳过并报告，其余依赖继续处理，最终以失败退出。
- `sync --check` 只检查，不联网、不下载、不检出、不写文件；检查 `woden.toml`、`woden.lock`、`vendor/` 工作副本的提交，以及 `ols.json` 中受管理的映射是否一致，不一致则失败。
- `update` 显式更新 `branch`、`tag` 和省略引用的依赖；固定 `rev` 的依赖不会被自动替换：不带名称的 `update` 跳过它们，指定名称则报错。
- `remove` 不得静默删除包含本地修改或无法确认归属的工作副本。移除最后一个依赖时，同时删除 `[collections]` 中的 `deps` 映射和为空的 `vendor/` 目录，避免 collection 指向不存在的目录。

`woden.lock` 示例：

```toml
version = 1

[[package]]
name = "example-library"
git = "https://github.com/example/example-library.git"
branch = "main"
commit = "0123456789abcdef0123456789abcdef01234567"
```

锁文件中每个依赖记录名称、Git 来源、请求的引用（`branch`、`tag` 或 `rev`，省略引用时不写）和完整提交 ID。

`libs/` 中的本地源码由项目自行维护，不写入 `[dependencies]` 或 `woden.lock`；它只放自行管理的代码，第三方依赖应使用 `woden add`，不要手工复制到 `libs/`。`woden fmt`、`woden check` 和 `woden lint` 不处理 `libs/`。Woden 不会在 `libs/` 与 `vendor/` 之间自动搜索、复制或切换。

## OLS

初始化时，Woden 会在 `ols.json` 缺失时生成基础配置（内容见「初始化生成的文件」），包括 collections、`checker_args` 和默认的 OLS profile。

`woden.toml` 是 Woden 管理的 collection 映射的权威来源，同名 collection 以它为准。`woden sync` 将这些映射增量同步到 `ols.json`：`deps` 由 Woden 完全管理，随 `woden.toml` 新增或删除；其他 collection 只新增或更新，不删除。OLS 其他设置和仅由 OLS 管理的 collection 保持不变。`ols.json` 不存在时，`sync` 不会创建它。无法安全解析或合并配置时应报错，不得重写整份文件。

OLS 的 `profile` 与 Woden 的 `dev` / `release` profile 相互独立。`odinfmt` 使用自己的 `odinfmt.json`；第一版 Woden 不生成或管理该文件。

## 开发 Woden

Woden 使用 Go 和 Cobra。建议使用 `cobra-cli` 初始化应用并生成命令：

```sh
go install github.com/spf13/cobra-cli@latest
go mod init <module-path>
cobra-cli init
cobra-cli add <command>
```

Woden 自定义的参数校验、项目根目录定位、配置合并、collection 参数生成、OLS 同步、锁文件维护、Git 工作副本安全检查和外部进程调用由 Woden 实现；通用 CLI 帮助与命令框架能力使用 Cobra 提供的功能。

## 参考资料

- [Cargo: `new`](https://doc.rust-lang.org/cargo/commands/cargo-new.html) / [Cargo: `init`](https://doc.rust-lang.org/cargo/commands/cargo-init.html)：项目创建与初始化语义。
- [Odin 文档](https://odin-lang.org/docs/) / [Odin 编译器参数](https://github.com/odin-lang/Odin/wiki/Compiler-Flags)：语言、collection 和编译选项。
- [Odin 测试文档](https://odin-lang.org/docs/testing/)：`@(test)`、`core:testing` 和 `-all-packages`。
- [OLS README](https://github.com/DanielGavin/ols/blob/master/README.md)：`ols.json`、profile 和 `odinfmt.json`。
- [Cobra CLI Generator](https://github.com/spf13/cobra-cli)：Go CLI 命令生成器。
