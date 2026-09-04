# 贡献指南

本 fork 刻意保持小范围，只维护 OpenAI-compatible API。

打开或更新 PR 前运行：

```bash
./scripts/lint.sh
./tests/scripts/check-refactor-line-gate.sh
./tests/scripts/run-unit-all.sh
```

要求：

- 变更保持小而聚焦。
- 修改 Go 文件后运行 `gofmt -w`。
- 不要忽略 `Close`、`Flush`、`Sync` 等 cleanup error。
- 行为变化要同步更新文档。
- 未经明确项目决策，不要重新引入已删除的协议 adapter、UI 或 runtime bridge。
