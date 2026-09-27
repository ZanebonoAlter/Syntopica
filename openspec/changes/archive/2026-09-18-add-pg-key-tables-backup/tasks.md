<!-- doc-impact: deployment（docs/reference/deployment.md 备份节更新） -->

## 1. 备份脚本

- [x] 1.1 新增 `scripts/db/backup-key-tables.sh`：set -euo pipefail；flock 重入保护；容器与 `pg_isready` 前置校验（失败非零退出、无产物）；`docker exec` 容器内 `pg_dump -U postgres -d syntopica -n public` + 排除清单（proposal 15 表，`--exclude-table=public.<name>` 形式）；输出先写 `backups/.pg-key-<ts>.dump.part`。验证：`bash -n` 语法通过
- [x] 1.2 实现校验改名与轮转：dump 成功后 `pg_restore -l` 校验 → 改名 `pg-key-<ts>.dump` → 仅成功后清理旧档保留最近 7 份；脚本头注释写明「新加日志/缓存表须补排除清单」。验证：`shellcheck`（若可用）或逐段 review，日志步骤行含时间戳

## 2. 定时安装

- [x] 2.1 幂等安装 crontab 条目（`0 4 * * * cd <repoRoot> && bash scripts/db/backup-key-tables.sh >> logs/db-backup.log 2>&1`，grep 判重不重复写入）。验证：`crontab -l` 含且仅含一条该条目

## 3. 端到端验证

- [x] 3.1 手动触发备份全链路：产出 `backups/pg-key-<ts>.dump`、`pg_restore -l` 列表含关键表（articles/semantic_labels/topic_tag_embeddings/daily_report_threads 等）且不含排除表（ai_call_logs/otel_spans/ai_embedding_cache/scheduler_tasks/reading_behaviors 等）。验证：spec「备份范围」两 Scenario 逐条核对
- [x] 3.2 失败路径验证：临时停容器跑脚本 → 非零退出、无新档、旧档不动；重启容器恢复。验证：退出码 `echo $?` 非 0，`ls backups/` 数量不变
- [x] 3.3 轮转验证：手动放 7 份假档（`touch backups/pg-key-20200101-000000.dump` 等）再跑一次 → 跑完后恰 7 份且含新档。验证：`ls -1t backups/pg-key-*.dump | wc -l` 输出 7

## 4. 测试

- 纯脚本 change，无代码包测试欠账；验证以 3.1-3.3 端到端 Scenario 代替（备份行为无法用单测有效覆盖，spec Scenario 即验收用例）

## 5. 文档

<!-- 无 flow 影响：系统 crontab 运维设施，独立于应用栈（design D4），不触及任何业务 flow 域 -->

- [x] 4.1 更新 `docs/reference/deployment.md` 备份节：定时备份机制（脚本路径/排除法口径/保留 7 份/crontab 条目/日志位置）、恢复命令（`pg_restore` 完整示例，含只恢复单表）、重装 crontab 的方法；`doc-impact.sh verify` 对账通过

## 6. 验证

- [x] 6.1 `bash -n scripts/db/backup-key-tables.sh` → 无输出（语法通过）
- [x] 6.2 `bash scripts/db/backup-key-tables.sh` → 退出码 0，stdout 含「backup ok」与档名大小，`backups/pg-key-*.dump` 新增 1 份
- [x] 6.3 `docker exec -i syntopica-postgres pg_restore -l < backups/pg-key-<ts>.dump` → 含 articles/semantic_labels，不含 ai_call_logs/otel_spans；补充实测：排除表 TABLE/TABLE DATA 零命中，但其 serial 序列作为空壳进档（pg_dump 18 无序列级排除），恢复时 PG 复用同名序列、AutoMigrate 建表不受影响（已实测），边界已写入脚本注释与 deployment.md
- [x] 6.4 `crontab -l | grep backup-key-tables` → 恰一条 `0 4 * * *` 条目
- [x] 6.5 `bash scripts/harness/doc-impact.sh verify openspec/changes/add-pg-key-tables-backup` → 对账通过（deployment.md 已更新）
- [x] 6.6 `openspec validate add-pg-key-tables-backup` → 通过

| Scenario | 测试文件 |
| --- | --- |
| 排除清单内的表不进备份档 | 人工：`pg_restore -l < backups/pg-key-20260918-143344.dump` 输出核对（6.3）——15 张排除表零命中 |
| 新建业务表自动被备份 | 人工：排除法口径，6.3 输出含 articles/semantic_labels/topic_tag_embeddings/daily_report_threads 等未排除表 |
| 正常备份产出 | 人工：6.2 手动跑退出码 0，产出 `backups/pg-key-20260918-143344.dump`（约 581MB），`pg_restore -l` 校验通过 |
| 容器不可用时失败而非空档 | 人工：3.2 停容器跑脚本 → 非零退出、无新档、旧档不动 |
| 超出保留数的旧档被清理 | 人工：3.3 放 7 份假档再跑 → 跑完后恰 7 份且含新档 |
| 备份失败不删旧档 | 人工：3.2 失败路径 `ls backups/` 数量不变；轮转仅成功分支执行 |
| crontab 条目存在且格式正确 | 人工：6.4 `crontab -l` 含且仅含一条 `0 4 * * * cd <repoRoot> && bash scripts/db/backup-key-tables.sh >> logs/db-backup.log 2>&1` |
| 失败写入日志并可察觉 | 人工：cron 条目重定向 `>> logs/db-backup.log 2>&1`；脚本失败路径 stderr 输出带时间戳 ERROR 行（3.2 实测终端可见） |
