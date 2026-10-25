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
