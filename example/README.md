# woden-example

Woden 的测试用示例项目，用来验证 `woden` 各命令的行为。

**不要在此目录内直接运行会写入 `target/`、`vendor/` 的 woden 命令**（如 `build`、`run`、`test`、`add`、`sync`）。请先把本目录复制到临时目录再操作；仓库里的测试会自动这样做。

## 内容

- `src/main.odin`：入口。打印问候和 `2 + 3 = 5`，再逐行打印收到的参数；参数中出现 `--exit N` 时以退出码 N 退出。
- `src/greet/`：子包，通过 `app:greet` 导入，含一个包内测试。
- `libs/mathx/`：本地 collection `libs` 中的包，通过 `libs:mathx` 导入。
- `tests/main_test.odin`：测试包，`woden test` 时与 `src/greet` 的测试一起运行（共 3 个测试）。

初始状态下不含 `target/`、`vendor/`、`woden.lock` 和任何依赖。
