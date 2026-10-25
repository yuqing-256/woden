# 给 DSH 的 Woden 实现指令（修订版）

> 使用方式：将本文保存为仓库内 `docs/BRIEF.md`，然后向 DSH 发送一句话：“阅读 docs/BRIEF.md、README.md 和 docs/FIXTURES.md，并按其中要求实现完整的 Woden。”

## 0. 任务与边界

你是资深 Go CLI 工程师，任务是根据 README.md 实现 Woden（用 Go + Cobra 编写的 Odin 项目管理工具）第一版。

- README.md 是唯一需求来源。README 标明“第一版不支持”的内容一律不做；不得增加 README 之外的命令、选项、配置键，不得透传任意 odin / odinfmt 参数。
- README 有歧义或遗漏时：选择最保守、最接近 Cargo 的实现，把决定和理由记录到 `docs/DECISIONS.md`，然后继续，不要停下来提问（除非完全无法继续）。
- 先阅读 README、本文和 `docs/FIXTURES.md`，完成第 1.1 节的测试资源核对并提交核对结果，再输出分阶段实施计划（见第 6 节），然后逐步实现。

## 1. 仓库的初始状态（已就绪，不要重做）

- 已完成并提交基线：`go mod init`、`cobra-cli init`，以及 13 个命令的 `cobra-cli add`：new、init、fmt、check、lint、test、build、run、clean、add、remove、sync、update（文件为 `cmd/<name>.go`，变量为 `<name>Cmd`）。
- **不要再运行 cobra-cli，也不要重新生成这些文件**；只在现有文件上演进：把占位的 `Run` 改为 `RunE`，删除根命令里的示例 `toggle` 标志和占位文本，补全 `Use/Short/Long/Example/Args/flags`。
- 已提供测试资源：`example/`（示例项目）、`testdata/`（fixture）、`docs/FIXTURES.md`（内容规格）。不要修改 `example/` 中的 Odin 源码；可以按 FIXTURES.md 的规格补充缺失的非 Odin fixture。仓库根的 `.gitattributes`、`.editorconfig`、`.vscode/settings.json` 已就绪，不要改动；所有文本文件一律使用 LF。

### 1.1 开工前：核对测试资源（M0）

`docs/FIXTURES.md` 是规格，`example/` 和 `testdata/` 是实际文件，二者可能因为人工编辑而不一致。在写任何实现代码之前，先核对并留下记录：

1. **核对文件清单**：递归列出 `example/`、`testdata/`，加上仓库根的 `.gitattributes`、`.editorconfig`、`.vscode/settings.json`，与 FIXTURES.md 的文件树逐项比对，找出缺失和多余的文件。
2. **核对内容**：FIXTURES.md 给出全文的文件，逐字节比对（含换行符、结尾换行、缩进）；只有行为规格的 Odin 源码（如 `example/src/main.odin`），静态阅读并核对是否满足规格，有真实 Odin 时再运行 FIXTURES.md 里的验证清单。`testdata/golden/new/` 还要与 README “初始化生成的文件”一节逐字比对。
3. **核对换行与编码**：运行 `git ls-files --eol`，所有文本文件应为 `i/lf` 且 `w/lf`；检查有无 BOM。发现 CRLF 或 BOM，只记录，不要自行批量转换人工编写的文件。
4. **核对文档之间的矛盾**：README、本文、FIXTURES.md 之间有矛盾的地方一并列出。
5. **处理规则**：缺失的非 Odin fixture，且 FIXTURES.md 有全文或明确规格，按规格创建；不一致的一律**不要**自行修改 `example/` 与已存在的 `testdata/` 文件，也不要改测试的期望值去迁就它们，而是记录到“需要人工处理”，依赖它的测试用 `t.Skip` 并写明审计项编号，其余工作继续。“待实测”项不算差异。
6. **产出**：`docs/FIXTURES_AUDIT.md`，包含核对所用的提交、已补建的文件、需要人工处理的差异（编号、文件、期望、实际）、换行与 BOM 检查结果、未能核对的项（例如没有真实 Odin）。先提交该文件和补建的 fixture，再开始 M1。

## 2. 技术约束

- 优先使用 Cobra / pflag 自带能力，不自己重复造轮子：
  - 参数校验：`cobra.NoArgs / ExactArgs / MaximumNArgs`；`run` 用自定义 Args + `ArgsLenAtDash` 处理 `-- ARGS`（位置参数只能出现在 `--` 之后）。
  - 标志约束：`MarkFlagRequired`（`add --git`）、`MarkFlagsMutuallyExclusive`（`--branch/--tag/--rev`）。
  - 所有命令用 `RunE` 返回错误；根命令设置 `SilenceUsage` / `SilenceErrors`；统一由 `main` 处理错误与退出码。
  - 根 `PersistentPreRunE` + `Annotations` 标记哪些命令需要 `woden.toml`（`new`、`init` 不需要）；避免子命令覆盖根钩子（或启用 `cobra.EnableTraverseRunHooks`，视 Cobra 版本）。
  - Command Groups 组织帮助；`ValidArgsFunction` 提供补全（`remove`/`update` 的依赖名，`new`/`init` 的目录）；`cmd.OutOrStdout / ErrOrStderr` 便于测试；`main` 里用 `ExecuteContext` 传递 context；保留内置 `version`（ldflags 注入）与 `completion`。
- 帮助文本和错误信息用英文。成功退出码 0，失败 101；只有 `woden run` 返回 `odin run` 的退出码。
- 建议目录：`cmd/`、`internal/errs`、`internal/fsx`、`internal/validate`、`internal/config`、`internal/project`、`internal/scaffold`、`internal/proc`、`internal/odin`（参数构造，纯函数）、`internal/gitdep`、`internal/ols`、`internal/testutil`。`cmd/` 保持很薄，只声明参数并调用 `internal/`。

## 3. 防御性要求（每个相关任务都要落实并有测试）

- **路径**：所有输入路径 Clean + 绝对化，并校验不越出项目根（用 `filepath.Rel` 判断，处理 `/a/b` 与 `/a/bc`、Windows 卷名与大小写）。`clean` 必须处理 `..` 与符号链接 / Windows junction：用 `Lstat`，遍历不进入链接，遇到链接只删链接本身；`--dry-run` 与真实执行共用同一份删除计划。
- **外部进程**：只用 `exec.CommandContext`，参数用切片，不经 shell；Git 调用所有位置参数前加 `--`，ref 用完整形式（`refs/heads/…`、`refs/tags/…`），加 `-c protocol.ext.allow=never` 和 `LC_ALL=C`，stdin 非 TTY 时设 `GIT_TERMINAL_PROMPT=0`，带超时。启动前检查 odin / git / odinfmt 是否在 PATH 中（处理 `exec.ErrDot`），缺失时给出明确报错。通过可注入的 `Runner` 接口让命令可用 fake 测试。
- **文件写入**：`woden.toml` / `ols.json` / `woden.lock` 的修改都用“同目录临时文件 + fsync + 原子 rename”，保留权限。解析失败必须报错，绝不整份重写。`init` 创建文件用 `O_EXCL`，绝不覆盖已有文件。修改 `woden.toml` 时保留注释和格式：做文本级最小编辑，编辑后重新解码校验，失败则放弃写入；保持文件原有的换行风格（CRLF 文件改完仍是 CRLF）。`woden new` / `init` 生成的文件一律用 LF。`ols.json` 的合并同样保序、定点修改。
- **输入校验**：项目名（字母或下划线开头，其余字母、数字、`-`、`_`，且不是 Windows 保留设备名）；依赖名（拒绝空串、`.`、`..`、含 `/ \ :`、NUL/控制字符、前导 `-`、Windows 保留设备名、`.git`，并做大小写不敏感的重名检测）；collection 名（不得为 core / base / vendor）与路径（项目内相对路径）；Git URL（只允许 https / ssh / git / scp 形式，拒绝 `-` 开头、含空白或控制字符、`ext::`）；引用名与提交 ID（`--rev` 接受完整或至少 7 位的缩写，lock 记录完整 ID）。
- **错误**：用 `%w` 包装并带上下文；区分用户错误（UserError）与内部错误；处理 Ctrl+C，取消 context 并终止子进程。
- **一致性与可恢复**：`add`/`remove`/`sync`/`update` 对 vendor、lock、toml、ols.json 的写入顺序固定，每一步可重入、失败可恢复，`sync` 是统一的修复入口。vendor 工作副本先在 `vendor/.staging-*` 克隆检出，再原子 rename。`remove` 前检测工作副本是否有本地修改或本地提交、归属是否匹配（origin 与声明一致），有则拒绝。
- **跨平台**：用 `filepath` 和 `runtime.GOOS` 处理 `.exe` 后缀与路径分隔符。

## 4. Odin 调用契约

所有 odin 调用都带 `-collection:name=<绝对路径>`（由 `woden.toml` 的 `[collections]` 生成），路径全部绝对化。

| 命令  | 调用                                                                                                                                 |
| ----- | ------------------------------------------------------------------------------------------------------------------------------------ |
| fmt   | 对存在的 `src/`、`tests/` 各运行 `odinfmt <dir> -w`；都不存在则报错；不碰 `libs/`、`vendor/`                                         |
| check | `odin check src`                                                                                                                     |
| lint  | `odin check src -vet`                                                                                                                |
| test  | `odin test tests -all-packages -out:target/debug/deps/<name>` + dev 编译选项；Odin 运行后删除产物                                    |
| build | `odin build src -out:target/{debug,release}/<name>`（Windows 带 `.exe`）+ profile 选项（`debug`→`-debug`，`opt-level`→`-o:`）        |
| run   | `odin run src -out:target/debug/<name> -keep-executable` + dev 选项 + `-- ARGS`；odin 的 cwd 为调用 `woden` 时的目录，路径用绝对路径 |

`check`、`lint`、`test`、`build`、`run` 执行前，若 `woden.toml` 配置了 `deps` 映射而 `vendor/` 不存在，先报错并提示运行 `woden sync`。非 `run` 命令中，子进程返回非 0 一律转为退出码 101。

以下 Odin 行为未经验证，请在有真实 Odin 的环境里实测；没有则写入 `docs/DECISIONS.md` 的“待实测”清单，并通过 `Runner` 接口隔离，之后补契约测试：

1. `odin run` / `odin test` 是否遵守 `-out:` 与 `-keep-executable`；
2. `odin run` 是否透传程序退出码（在 PowerShell 里看 `$LASTEXITCODE`，`$?` 只表示成功或失败）。已验证：`--` 之后的参数会原样传给程序，程序里用 `os.args[1:]` 即可取得；
3. `-vet` 是否会作用到从 `deps:` 导入的第三方包；
4. `odinfmt <dir> -w` 对目录的处理；
5. Windows 下 `-collection:name=C:\path` 的写法；
6. `-o:` 的允许值；
7. `tests/` 里 `import "app:greet"` 这类指向 `src/` 子包的导入是否可用；
8. OLS 对 `ols.json` 注释和尾逗号的容忍度。

## 5. 测试策略与测试资源

- `internal/testutil` 提供：`RepoRoot(t)`（沿 `go.mod` 向上找仓库根）、`Testdata(t, rel)`、`CopyExample(t)`（把 `example/` 复制到 `t.TempDir()` 并返回路径）、`NewDepRepo(t)`（见 FIXTURES.md）、`ToCRLF(s)`（把内容里的 LF 全部换成 CRLF，用于换行风格的用例；仓库里不存放 CRLF 文件）。**测试永远不要在 `example/` 或 `testdata/` 原地运行 woden，一律操作临时副本。**
- 三层测试：
  - 单元：表驱动，使用 `testdata/` 里的 fixture。
  - 集成：临时目录 + 假 odin / odinfmt（用 Go 写 helper-process，记录收到的参数并按需返回退出码），验证命令行与退出码。
  - 端到端：构建 woden 二进制，使用真实 odin、odinfmt、git，对 `example/` 的临时副本逐条跑命令，断言依据见 FIXTURES.md；构建标签 `e2e`。
- 构建标签 `woden_testing`：只有带该标签的构建才允许 `file://` 的 Git 依赖（拆成 `allowfile_testing.go` 与 `allowfile_default.go`，默认不允许）。需要 `file://` 的测试用该标签保护，未带标签时 skip 并说明原因。CI 同时运行 `go test ./...` 和 `go test -tags woden_testing ./...`；端到端用 `go test -tags "e2e woden_testing" ./...`，作为单独 job，固定 Odin 版本。
- 恶意与边界场景（符号链接逃逸、vendor 内符号链接、归属不匹配、中断恢复）由测试代码在临时目录里即时构造，不要提交到仓库；符号链接用例在 Windows 无权限时 skip。
- 测试不得依赖外网。

## 6. 实施顺序（每个任务小到一次提交可完成）

**M0 测试资源核对**（见第 1.1 节）：先完成并提交 `docs/FIXTURES_AUDIT.md`，再开始下面的 M1。

- **M1 骨架补全**：补全 13 个命令的 `Use/Short/Long/Example/Args/flags`，Command Groups（project / build / dependency），统一错误与退出码，signal context，`SetFlagErrorFunc`，`version`，testutil 基础，CI 基线（linux / macOS / windows，`go vet`、`go test -race`）。
- **M2 基础与配置**：fsx（Within、AtomicWrite、WriteNew）、validate、config（严格解码，未知键警告，非法值报错；profile 只允许 dev / release）、`woden.toml` 文本级编辑器（先做 spike）、项目根定位与 preflight。
- **M3 new / init**：模板内容与 README 完全一致，并与 `testdata/golden/new/` 逐字节比对；`.gitignore` 已存在只追加缺失行；`src/`、`tests/` 内已有 `.odin` 文件则不创建示例文件；目标为符号链接则拒绝；失败回滚本次创建的条目。
- **M4 外部进程**：proc 封装、odin 参数构造（纯函数，golden 测试）。
- **M5 fmt / check / lint / test / build / run**。
- **M6 clean**：安全删除原语 + `--dry-run`。
- **M7 Git 依赖**：lock 模型（TOML：`version = 1`，`[[package]]` 含 name、git、branch/tag/rev、commit）、git 封装、vendor 管理、`add`、`sync [--check]`、`remove`、`update`。`remove` 移除最后一个依赖时，同时删除 `deps` 映射、为空的 `vendor/`、ols.json 的 `deps` 条目。
- **M8 OLS**：保序合并；`deps` 完全托管（含删除），其他 collection 只新增或更新、不删除；同名以 `woden.toml` 为准；ols.json 不存在时 `sync` 不创建；`--check` 只读。
- **M9 收尾**：英文帮助、补全、端到端测试、CI 完善、发布配置。

## 7. 工作方式

- 先输出实施计划（按 M0–M9 细分任务，标出前置和可并行项），再实现。
- 每个任务一次提交；提交前运行 `go vet ./...` 和 `go test ./...`，保持通过。
- 不使用 README 之外的运行时依赖；TOML 解析库自选（需支持严格解码和错误位置），记录选择理由。
- 不改动 `README.md`，除非发现明显笔误，此时在 `docs/DECISIONS.md` 里说明。

## 8. 完成标准

- README 中每个命令的行为和每条约定都有对应实现与测试，没有 README 以外的功能。
- `go vet ./...`、`go test ./...`、`go test -race ./...` 在 linux / macOS / windows 上通过；带 `woden_testing` 标签的测试通过；有真实 Odin 的环境里 `e2e` 通过。
- `docs/DECISIONS.md` 记录所有歧义决定与“待实测”清单；`docs/FIXTURES_AUDIT.md` 记录测试资源核对结果，所有差异都已处理或在其中说明。
- 最后输出一份总结：已实现的命令、未验证的 Odin 行为、已知限制。
