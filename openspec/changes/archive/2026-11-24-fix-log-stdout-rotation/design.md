## Context

见 proposal.md（Why）。补充实现侧现状：

- `backend-go/internal/platform/logging/logging.go` 已实现 lumberjack 文件 handler，`FileConfig` 字段齐全；`Init` 当前把 console handler 与 file handler 一起塞进 `fanoutHandler`，**无论文件是否启用都会写 stdout**。
- Gin 未做任何 writer 定制，`gin.Default()` 创建的 Logger/Recovery 中间件默认写 `os.Stdout`/`os.Stderr`；访问日志因此完全绕过 slog 与轮转。
- `start-dev.sh` 用 `spawn_detached` 把后端 stdout/stderr 重定向到 `backend-go/nohup.out`（脚本注释也这么写）。
- `logging.SetWriters` / `ResetWriters` 被单测用于重定向输出，改动不能破坏它们。
- `configs/config.yaml` 当前 `log.level: debug`；配置层没有 env 覆盖（无 `AutomaticEnv`/`BindEnv`），级别只能通过配置或代码改。

## Goals / Non-Goals

**Goals:**

- 运行期所有日志（slog + zap bridge + Gin 访问/恢复）落入同一个已轮转的 `logs/app.log`。
- `backend-go/nohup.out` 只保留启动横幅与进程级 stderr 兜底（应为 KB 级）。
- 保留可显式打开的 stdout 通道，供脚本/单测/临时调试使用。

**Non-Goals:**

- 不引入 systemd journal、logrotate、外部日志采集。
- 不改日志格式、字段、文案；不动 `logs/app.log` 的轮转参数。
- 不改配置层（不加环境变量覆盖）。

## Decisions

### D1. 统一写入同一个 lumberjack 实例，而不是给 Gin 另配轮转

- **结论**：`logging` 包暴露 `FileWriter() io.Writer`（返回内部 lumberjack 实例，未启用文件日志时返回 `nil`），`cmd/server` 用它设置 `gin.DefaultWriter` 与 `gin.DefaultErrorWriter`。
- **落点**：backend-go/internal/platform/logging/logging.go
- **取舍**：外部 logrotate + `copytruncate` 管 nohup.out 被否——需要 root 配置、旋转瞬间会丢行，且与既有 lumberjack 形成两套机制；给 Gin 单开第二个 lumberjack 也被否——同一小时内会产生两份交错文件，排查时要对时间戳。

### D2. 文件日志启用时默认关闭 stdout 扇出，用显式开关保留

- **结论**：`Init` 在 `fileCfg.Enabled && Path != ""` 时只装文件 handler；`LOG_CONSOLE=true`（进程环境变量，仅此一处解析）时额外装 console handler。文件未启用时行为不变（纯 stdout），保证脚本与单测语义。
- **落点**：backend-go/internal/platform/logging/logging.go
- **取舍**：永久双写被否——正是 nohup.out 膨胀的另一半来源；彻底删掉 console handler 也被否——`SetWriters`/`ResetWriters` 的单测与一次性脚本依赖 stdout 可见性。

### D3. Gin writer 在 `gin.Default()` 之前注入

- **结论**：`main.go` 中 `logging.Init(...)` 之后、`r := gin.Default()` 之前设置 `gin.DefaultWriter = logging.FileWriter()`（非 nil 时），`gin.DefaultErrorWriter` 同样处理。`gin.Default()` 内部的 Logger/Recovery 中间件在构造时读取该全局值。
- **落点**：backend-go/cmd/server/main.go
- **取舍**：自写 `gin.LoggerWithWriter` 中间件并替换 `gin.Default()` 被否——要重写 `gin.Default()` 的 Logger+Recovery 组合，改动面反而更大且易漏 Recovery。

### D4. `log.level` 默认降到 `info`

- **结论**：`configs/config.yaml` 的 `log.level` 改为 `info`。要 debug 时改该文件后重启；配置层不提供 env 覆盖（不在本 change 范围）。
- **落点**：backend-go/configs/config.yaml
- **取舍**：保持 `debug` 被否——实测 `TagMatch`/`findOrCreateTag` 每条标签 4 行、`ensureVectorDimension` 每次写入 1 行，debug 下日志量是 info 的 5-10 倍。

### D5. nohup.out 保留为启动兜底，不删重定向

- **结论**：`start-dev.sh` 继续把 stdout/stderr 重定向到 `nohup.out`（保留启动横幅、panic 栈、进程级 stderr），只更新脚本注释中的日志查看指引为 `tail -f backend-go/logs/app.log`。
- **落点**：scripts/dev/start-dev.sh
- **取舍**：改重定向到 `/dev/null` 被否——启动失败（端口占用、DSN 错）时那几行是唯一线索；改到 `/dev/stderr` 被否——在 detached 进程里同样无处可去。

### D6. 轮转参数维持现状

- **结论**：`max_size_mb=50`、`max_backups=30`、`max_age_days=30`、`compress=true` 不动；实测 9 天产生约 110MB gz，容量上限充裕。
- **落点**：backend-go/configs/config.yaml
- **取舍**：调小到 20MB/7 份被否——没有容量压力，频繁切分只增加文件数。

## Risks / Trade-offs

- [开发者习惯 `tail nohup.out`，改动后会以为日志没了] → 启动横幅里打一行指向 `logs/app.log`；`start-dev.sh` 注释与 `docs/reference/development.md` 同步更新。
- [Gin Recovery 的 panic 栈原本进 nohup.out，与 slog 分离] → `gin.DefaultErrorWriter` 也指向同一 writer，panic 进 `logs/app.log`；进程级崩溃（如 `fatal error`）的 stderr 仍落 nohup.out，两者互补。
- [lumberjack 在多 goroutine 并发写下的锁竞争] → lumberjack 自身带 mutex；Gin 访问日志当前约 1-2 行/秒，不构成热点。
- [`LOG_CONSOLE` 解析写在 logging 包内会引入「配置散落」] → 只允许这一个开关，且命名前缀 `LOG_`，在文件注释与文档中登记为唯一例外。

## Migration Plan

1. 改代码 → 本地 `go build ./...` / 针对性 `go test ./internal/platform/logging ./cmd/server`（如可测）。
2. 重启后端（`bash scripts/dev/start-dev.sh --restart back`），观察 `nohup.out` 是否停在 KB 级、`logs/app.log` 是否继续增长。
3. 回滚：还原提交后重启即可，无数据迁移；`logs/app.log` 与历史 `.gz` 不受影响。

## Open Questions

无。
