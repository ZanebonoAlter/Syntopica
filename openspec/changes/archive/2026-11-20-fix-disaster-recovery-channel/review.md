# Review — fix-disaster-recovery-channel

审查时间：2026-11-20（CST）；基线：工作树 HEAD（本 change 的实现文件均已提交）。
审查范围：`scripts/db/backup-key-tables.sh`（EXCLUDED 变更 + 同一快照基线）、`scripts/db/restore-key-tables.sh`（落库闸门 / 扩展预装 / TOC 过滤 / count 校验 / 迁移对账）、迁移 `20260403_0003`（维度无关化）、两份 smoke 脚本、`docs/reference/database/RESTORE.md` 与 `RESTORE_DRILL.md` 演练记录。
方式：逐文件读实现与脚本头部行为契约，对照 `proposal.md` / `design.md` / `specs/`（disaster-recovery、db-migration-safety、pg-scheduled-backup）；未重跑 smoke（tasks 6.1/6.2 已勾，且归档时树上存在其他 active change 的并发改动，避免重负载与交叉污染）。

### R-1 2026-11-20【Medium】`schema_migrations` 进档后，「迁移已应用但表不在档内」的交叉缺口只进报告、不阻断
- **状态**：已采纳（脚本第 5 条行为契约即为此设计：缺口只进报告不影响退出码，理由是被排除清单里的 6 张再生表由 AutoMigrate 重建。风险真实存在——恢复库会显示某迁移已应用而该迁移触及的排除表不存在于档内；但该表本就设计为可再生，且 `20260403_0003` 已改为维度无关、不会因维度失败。后续 `unify-vector-column-dimension` 收口维度 schema 时需再对账一次）

### R-2 2026-11-20【Low】落库闸门的生产库判定依赖 `RESTORE_PROD_DB` 环境变量，环境变量误置会让闸门失效
- **状态**：已采纳（缺省值为 `syntopica`，生产 cron 不调用本脚本；`--allow-prod` 仍需交互输入库名二次确认。已在脚本头部行为契约写明，风险面收敛到「手工把 RESTORE_PROD_DB 设成别的值再指定同名为目标库」这一人为路径，不再额外加代码餐）

### R-3 2026-11-20【Low】6.12 历史档演练的 `/health` 200 未取到
- **状态**：已采纳（如实保留在 `RESTORE_DRILL.md` 演练② 与 tasks 6.12：迁移链关键点「维度冲突不再失败」已实测通过（2560 维存量 ALTER 131s 成功），卡点在 `20260417_0002` 的 search_vector 全表回填耗时（小时级）；该遗留已登记进 RESTORE_DRILL「遗留」节，移交后续 change，不在本 change 内伪造完成）

### R-4 2026-11-20【Low】count 基线必须与 dump 同一 MVCC 快照，否则 10 分钟窗口内的写入会让逐表比对误报
- **状态**：已采纳（脚本用 `pg_export_snapshot` → `pg_dump --snapshot` + `SET TRANSACTION SNAPSHOT` 同一快照，脚本头部注释写明理由；2026-11-05 演练① 实测 64 张表 0 差异）

## 结论

无 High 阻塞项、无 Medium 需在归档前修复项（R-1 已由脚本的报告与对账机制覆盖并留痕）。实现与 delta spec 一致：备份语义修复（L1）、迁移维度无关化（L2）、恢复脚本与文档（L3）三线齐全，smoke 与演练证据在 `RESTORE.md` / `RESTORE_DRILL.md` 可复核。
