<!-- complexity: simple -->
<!-- ui-impact: none -->
<!-- constraint-domains: 无（纯运维脚本 change，不涉业务域代码） -->

## Why

当前没有适配 Linux 树莓派环境的自动化数据库备份：唯一的备份脚本 `scripts/db/backup-postgres.ps1` 是 Windows 时代的全库冷备（需停容器→7z 打包→起容器），在现环境不可用也跑不了定时；全库 dump 又把 ~1GB 的日志/缓存/队列表（ai_embedding_cache 723MB、otel_spans 日均 82 万行读写、ai_call_logs 11 万行）一并拖进来。articles、语义板块标签体系（semantic_labels 12.7 万行）、日报、AI 配置等业务数据每日持续写入且部分依赖付费 AI 生成（embed/摘要），磁盘故障时无恢复点。

## What Changes

- 新增 `scripts/db/backup-key-tables.sh`：基于 pg_dump 排除法（`--exclude-table` 显式列出日志/缓存/队列/运行时状态表），`-Fc` 自定义格式，输出 `backups/pg-key-<timestamp>.dump`
  - 排除清单（9 类，边界表经用户确认全部排除）：`ai_call_logs`、`otel_spans`、`ai_embedding_cache`、`firecrawl_jobs`、`tag_jobs`、`embedding_queues`、`merge_reembedding_queues`、`notifications`、`discovery_runs`、`discovery_run_items`、`cross_board_relation_runs`、`topic_watch_hits`、`scheduler_tasks`（运行时调度状态非配置）、`reading_behaviors`、`schema_migrations`（恢复时 AutoMigrate 重跑）
  - 其余全部业务表自动纳入（排除法：新加业务表默认被备）
- 新增保留策略：每次备份后清理旧档，仅保留最近 7 份
- 安装 crontab 条目：每天 04:00 执行（低峰期），日志落 `logs/db-backup.log`
- **不做**：不停库（pg_dump 在线一致性快照）、不备 `backend-go/data/icons/`（用户确认接受重新抓取）、不做恢复脚本（恢复命令写入 deployment.md 文档即可）

## Capabilities

### New Capabilities

- `pg-scheduled-backup`: 定时备份 PostgreSQL 关键业务表的行为契约——备份范围（排除法清单）、产物格式与命名、保留策略（7 份）、定时执行（每日 04:00）、失败可观测（退出码 + 日志）

### Modified Capabilities

（无——现有 spec 均不涉及）

## Impact

- **新增脚本**：`scripts/db/backup-key-tables.sh`（排除清单 + dump + 轮转清理，复用 `db-cleanup-2026-08/backup.sh` 的容器内 exec 模式）
- **宿主机 crontab**：新增一条定时条目（当前 crontab 为空）
- **文档**：`docs/reference/deployment.md` 备份节更新（现只有一句手工 pg_dump 示例，补定时备份说明与恢复命令）
- **磁盘**：关键表约 1.2GB，`-Fc` 压缩后单份预计几百 MB，7 份约 2-3GB（/ 剩余 87G，充裕）
- **风险与边界**：pg_dump 与在线写入并发安全（MVCC 一致性快照）；容器未运行时脚本应明确报错退出而非产出空档；排除清单以表名硬编码，未来新增日志表需手动补排除（已在脚本注释与文档中注明）
