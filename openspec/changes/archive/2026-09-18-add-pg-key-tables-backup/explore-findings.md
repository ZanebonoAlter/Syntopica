
## 备份脚本实现要点（含用户决策）

见 docs/research/pg-backup-script/explore-findings.md（表分类+环境事实）与 docs/research/explore-findings.md（用户决策）。核心：排除法 15 表清单见 proposal；容器 syntopica-postgres 内 pg_dump -Fc（版本匹配 pg18）；flock 重入保护；临时名 .part → pg_restore -l 校验 → 改名 → 轮转留 7；cron 0 4 * * *（宿主机 Asia/Shanghai 已确认）>> logs/db-backup.log 2>&1；schema_migrations 不备（AutoMigrate 重跑）。

<!-- pinned 2026-09-18T06:10:46Z -->
