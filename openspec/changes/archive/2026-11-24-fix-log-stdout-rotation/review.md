# fix-log-stdout-rotation Review

审查人：主 session（自审 + 运行验证）
审查范围：日志输出通道重构、Gin writer 注入、zap bridge 通道对齐、日志级别配置、启动脚本与文档同步。

- 审毕无发现（审查范围：`backend-go/internal/platform/logging/logging.go`、`zap_bridge.go`、`logging_stdout_test.go`、`backend-go/cmd/server/main.go`、`backend-go/configs/config.yaml`、`scripts/dev/start-dev.sh`、`docs/reference/configuration.md`、`docs/reference/development.md`）

核对项与证据：

1. 文件日志启用时不再向 stdout 扇出（nohup.out 无界增长的根因）——运行期 30 秒零字节增长，`TestInitWithFileDoesNotWriteStdout` 覆盖。
2. `LOG_CONSOLE=true` 能显式恢复 stdout，`SetWriters` / `ResetWriters` 语义未变——`TestInitWithLogConsoleWritesStdout`、`TestInitWithoutFileKeepsStdout`、既有 3 个单测全绿。
3. Gin 访问/恢复日志落到同一 lumberjack 实例——`grep -c '^\[GIN\]' backend-go/logs/app.log` 有增长，nohup.out 无新增 GIN 行。
4. zap bridge 不再硬编码 `os.Stdout`——`activeWriteSyncer()` 复用 `FileWriter()`，运行期无 zap 格式行进入 nohup.out。
5. 轮转参数未被改动（50MB × 30 份 × 30 天 × gzip），历史 `.gz` 不受影响。
6. 无 API 契约、数据库、前端行为变化；`golangci-lint run ./...` 0 issues、`go vet ./...` 0、`go build ./...` 通过。
7. 启动横幅打印日志路径，`start-dev.sh` 注释与文档同步，避免「以为日志丢了」的误判。
