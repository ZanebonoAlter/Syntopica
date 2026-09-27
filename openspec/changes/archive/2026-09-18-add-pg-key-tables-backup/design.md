<!-- design: add-pg-key-tables-backup -->

## Context

PG 跑在 Docker 容器 `syntopica-postgres`（pgvector/pgvector:pg18-trixie，compose：`docker-compose.pg.yml`），数据挂载 `./data`。现状：唯一备份脚本 `scripts/db/backup-postgres.ps1` 是 Windows 时代停库冷备，现环境（树莓派 arm64 / Debian 13）不可用；宿主机 crontab 为空。表分类与排除清单经活库实测与用户确认，见 proposal。可复用先例：`scripts/db/db-cleanup-2026-08/backup.sh`（容器内 `pg_dump -Fc` 模式）。

## Goals / Non-Goals

**Goals**
- 无人值守每日备份，产出可恢复的单文件档
- 备份体积可控（排除 ~1GB 可再生数据）
- 失败可察觉（退出码 + 日志）

**Non-Goals**
- 不做恢复脚本/自动恢复（恢复命令写进 deployment.md 人工执行）
- 不做异地/远端备份（本机磁盘冗余即可，用户未要求）
- 不备 `backend-go/data/icons/`（用户确认接受 favicon 重新抓取）
- 不接入 Go scheduler（见决策 4）

## Decisions

1. **排除法而非白名单**：`pg_dump --exclude-table` 显式列可再生表，其余自动纳入。新增业务表默认被备（白名单会漏）；新加日志表需手动补排除，此维护成本写在脚本头注释里。
2. **在线 `pg_dump -Fc` 而非停库冷备**：pg_dump 基于 MVCC 一致性快照，不中断服务；`-Fc` 自定义格式压缩率高（关键表 1.2GB → 预计几百 MB）且支持 `pg_restore -l` 校验与选择性恢复。停库冷备（ps1 先例）只在全库物理迁移场景有意义，每日定时不可接受停机。
3. **容器内 `pg_dump` 而非宿主机**：镜像内 pg_dump 与 server 版本严格一致（pg18），宿主机不保证装了客户端且版本可能不匹配（pg_dump 拒绝跨大版本）。执行方式：`docker exec syntopica-postgres pg_dump -U postgres -d syntopica -Fc ...`，输出经 stdout 流出宿主机落盘（Linux 本机 SD 卡 IO 无先例脚本中 Windows /mnt/d 的慢 IO 问题，无需 /tmp 中转）。
4. **crontab 而非 systemd timer / Go scheduler**：crontab 一行搞定、树莓派 Debian 原生支持、与项目无代码耦合。systemd timer 能力过剩且配置分散；Go scheduler 依赖应用进程活着——数据库/机器异常时恰恰是备份最该跑的时候，备份必须独立于应用栈。
5. **临时名落盘 + 成功后改名**：dump 先写 `backups/.pg-key-<ts>.dump.part`，`pg_restore -l` 校验通过后改名 `pg-key-<ts>.dump` 再轮转。半途失败的残档不占正式名，轮转也永不处理残档。
6. **重入保护用 `flock`**：`exec 9>/tmp/pg-key-backup.lock; flock -n 9 || exit 0`。cron 与手动跑可能重叠；备份耗时分钟级，冲突概率低但 flock 一行成本为零。拿不到锁静默退出（上一次还在跑，非错误）。
7. **日志由 cron 重定向落盘**：脚本只写 stdout/stderr（含时间戳步骤行），crontab 条目 `>> logs/db-backup.log 2>&1`。手动跑直接看终端，不产生额外文件。
8. **时区**：宿主机已确认 `Asia/Shanghai`（CST），cron `0 4 * * *` 即北京时间 04:00，无需 TZ 处理。

## Risks / Trade-offs

- [排除清单靠表名硬编码，新日志表默认被备进档] → 单份体积增长可从日志察觉；脚本头注释与 deployment.md 注明"新加日志/缓存表须补排除"；备份档内多一张日志表不影响恢复。
- [pg_dump 与写入并发，长事务极端情况下快照建立慢] → 单用户系统凌晨 4 点几乎无写入，实际无影响。
- [`-Fc` 档依赖 pg_restore 恢复，版本需 ≥ dump 版本] → 恢复用同一容器镜像执行（deployment.md 恢复命令写明），不存在版本漂移。
- [SD 卡是单点，备份也在同一张卡上] → 用户当前只要求本机备份；异地备份留作后续 change。
- [crontab 是用户级配置，重装系统/换机会丢] → crontab 条目写入 tasks 由脚本幂等安装（grep 判重），并记录在 deployment.md 可随时重装。

## Migration Plan

1. 新增脚本 + 幂等安装 crontab 条目（`crontab -l` 合并去重后写回）
2. 手动触发一次验证端到端（档产出、`pg_restore -l` 校验、轮转）
3. 回滚：删除 crontab 条目 + 删脚本即可，无数据/Schema 变更，无服务影响

## Open Questions

（无）
