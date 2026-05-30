## 1. 创建 reference/ 骨架（不破坏现有结构）

- [x] 1.1 创建 `docs/reference/`、`docs/reference/architecture/`、`docs/reference/api/`、`docs/reference/database/` 目录
- [x] 1.2 移动 `docs/architecture/overview.md` → `docs/reference/architecture/overview.md`
- [x] 1.3 移动 `docs/architecture/backend-go.md` → `docs/reference/architecture/backend.md`
- [x] 1.4 移动 `docs/architecture/backend-runtime.md` → `docs/reference/architecture/runtime.md`
- [x] 1.5 移动 `docs/architecture/frontend.md` → `docs/reference/architecture/frontend.md`
- [x] 1.6 移动 `docs/architecture/frontend-components.md` → `docs/reference/architecture/frontend-components.md`
- [x] 1.7 移动 `docs/architecture/data-flow.md` → `docs/reference/architecture/data-flow.md`
- [x] 1.8 移动 `docs/architecture/tracing.md` → `docs/reference/architecture/tracing.md`
- [x] 1.9 移动 `docs/architecture/tag-cleanup-redesign.md` → `docs/v1.2-tag-intelligence/design/tag-cleanup-redesign.md`（属 v1.2 设计）
- [x] 1.10 移动 `docs/api/*`（全部 15 个文件）→ `docs/reference/api/*`
- [x] 1.11 移动 `docs/database/*` → `docs/reference/database/*`
- [x] 1.12 移动 `docs/operations/development.md` → `docs/reference/development.md`
- [x] 1.13 移动 `docs/guides/configuration.md` → `docs/reference/configuration.md`
- [x] 1.14 移动 `docs/guides/deployment.md` → `docs/reference/deployment.md`
- [x] 1.15 移动 `docs/guides/testing.md` → `docs/reference/testing.md`
- [x] 1.16 移动 `docs/guides/content-processing.md` → `docs/reference/content-processing.md`
- [x] 1.17 移动 `docs/guides/reading-preferences.md` → `docs/reference/reading-preferences.md`
- [x] 1.18 移动 `docs/guides/frontend-features.md` → `docs/reference/frontend-features.md`
- [x] 1.19 移动 `docs/guides/getting-started.md` → `docs/getting-started.md`
- [x] 1.20 移动 `docs/guides/tagging-flow.md` → `docs/v1.2-tag-intelligence/user-guide/tagging-flow.md`（属 v1.2 用户文档）
- [x] 1.21 移动 `docs/guides/topic-graph.md` → `docs/v1.2-tag-intelligence/user-guide/topic-graph.md`（属 v1.2 用户文档）

## 2. 创建里程碑目录结构

- [x] 2.1 创建 `docs/v1.1-bugfixes/` 及子目录 `design/`、`user-guide/`、`changes/`、`debug/`
- [x] 2.2 创建 `docs/v1.2-tag-intelligence/` 及子目录 `design/`、`user-guide/`、`changes/`、`debug/`
- [x] 2.3 创建 `docs/v1.3-narrative-overhaul/` 及子目录 `design/`、`user-guide/`、`changes/`、`debug/`
- [x] 2.4 移动 `docs/releases/MILESTONE_v1.1_SUMMARY.md` → `docs/v1.1-bugfixes/SUMMARY.md`
- [x] 2.5 移动 `docs/releases/MILESTONE_v1.2_SUMMARY.md` → `docs/v1.2-tag-intelligence/SUMMARY.md`
- [x] 2.6 创建 `docs/v1.3-narrative-overhaul/SUMMARY.md`（简要说明当前活跃里程碑）

## 3. 归类 plans/ 到里程碑（v1.1）

- [x] 3.1 移动 `docs/plans/2026-03-04-firecrawl-integration-test-design.md` → `docs/v1.1-bugfixes/design/firecrawl-integration-test.md`
- [x] 3.2 移动 `docs/plans/2026-03-04-firecrawl-integration-test.md` → `docs/v1.1-bugfixes/changes/firecrawl-integration-test.md`
- [x] 3.3 移动 `docs/plans/2026-03-08-repo-reorganization-design.md` → `docs/v1.1-bugfixes/design/repo-reorganization.md`
- [x] 3.4 移动 `docs/plans/2026-03-08-repo-reorganization.md` → `docs/v1.1-bugfixes/changes/repo-reorganization.md`
- [x] 3.5 移动 `docs/plans/2026-03-08-scheduler-status-trigger-design.md` → `docs/v1.1-bugfixes/design/scheduler-status-trigger.md`
- [x] 3.6 移动 `docs/plans/2026-03-08-scheduler-status-trigger.md` → `docs/v1.1-bugfixes/changes/scheduler-status-trigger.md`
- [x] 3.7 移动 `docs/plans/2026-04-03-postgres-pgvector-single-database-architecture-migration.md` → `docs/v1.1-bugfixes/design/postgres-migration.md`
- [x] 3.8 移动 `docs/plans/2026-03-08-article-content-source-toggle.md` → `docs/v1.1-bugfixes/changes/article-content-source-toggle.md`
- [x] 3.9 移动 `docs/plans/2026-03-08-backend-doc-cleanup.md` → `docs/v1.1-bugfixes/changes/backend-doc-cleanup.md`
- [x] 3.10 移动 `docs/plans/2026-03-19-database-schema-dedup.md` → `docs/v1.1-bugfixes/changes/database-schema-dedup.md`
- [x] 3.11 移动 `docs/plans/2026-03-20-backend-runtime-closure.md` → `docs/v1.1-bugfixes/changes/backend-runtime-closure.md`
- [x] 3.12 移动 `docs/plans/2026-03-20-retry-repair.md` → `docs/v1.1-bugfixes/changes/retry-repair.md`
- [x] 3.13 移动 `docs/plans/2026-04-09-sqlite-archive-docker-design.md` → `docs/v1.1-bugfixes/design/sqlite-archive-docker.md`
- [x] 3.14 移动 `docs/plans/2026-04-09-sqlite-archive-docker.md` → `docs/v1.1-bugfixes/changes/sqlite-archive-docker.md`
- [x] 3.15 移动 `docs/plans/2026-03-11-topic-graph-homepage-progress.md` → `docs/v1.1-bugfixes/changes/topic-graph-homepage-progress.md`
- [x] 3.16 移动 `docs/plans/2026-03-11-topic-graph-homepage.md` → `docs/v1.1-bugfixes/changes/topic-graph-homepage.md`
- [x] 3.17 移动 `docs/plans/2026-03-14-topic-graph-refactor.md` → `docs/v1.1-bugfixes/changes/topic-graph-refactor.md`
- [x] 3.18 移动 `docs/plans/2026-03-14-topics-classified-analysis-implementation.md` → `docs/v1.1-bugfixes/changes/topics-classified-analysis-implementation.md`
- [x] 3.19 移动 `docs/plans/2026-03-26-pending-articles-node-design.md` → `docs/v1.1-bugfixes/design/pending-articles-node.md`
- [x] 3.20 移动 `docs/plans/2026-03-26-pending-articles-node.md` → `docs/v1.1-bugfixes/changes/pending-articles-node.md`
- [x] 3.21 移动 `docs/plans/2026-03-30-persistent-firecrawl-tag-queues-design.md` → `docs/v1.1-bugfixes/design/persistent-firecrawl-tag-queues.md`
- [x] 3.22 移动 `docs/plans/2026-03-30-persistent-firecrawl-tag-queues.md` → `docs/v1.1-bugfixes/changes/persistent-firecrawl-tag-queues.md`

## 4. 归类 plans/ 到里程碑（v1.2 — design/）

- [x] 4.1 移动 `docs/plans/2026-03-12-ai-router-design.md` → `docs/v1.2-tag-intelligence/design/ai-router.md`
- [x] 4.2 移动 `docs/plans/2026-03-23-tag-weight-convergence-design.md` → `docs/v1.2-tag-intelligence/design/tag-weight-convergence.md`
- [x] 4.3 移动 `docs/plans/2026-04-13-embedding-queue-design.md` → `docs/v1.2-tag-intelligence/design/embedding-queue.md`
- [x] 4.4 移动 `docs/plans/2026-04-15-tag-quality-score-design.md` → `docs/v1.2-tag-intelligence/design/tag-quality-score.md`
- [x] 4.5 移动 `docs/plans/2026-04-16-scheduler-panel-enhancement-design.md` → `docs/v1.2-tag-intelligence/design/scheduler-panel-enhancement.md`
- [x] 4.6 移动 `docs/plans/2026-04-18-narrative-feed-category-design.md` → `docs/v1.2-tag-intelligence/design/narrative-feed-category.md`
- [x] 4.7 移动 `docs/plans/2026-04-18-tag-elimination-design.md` → `docs/v1.2-tag-intelligence/design/tag-elimination.md`
- [x] 4.8 移动 `docs/plans/2026-04-20-cross-category-narrative-design.md` → `docs/v1.2-tag-intelligence/design/cross-category-narrative.md`
- [x] 4.9 移动 `docs/plans/2026-04-25-tree-review-phase4-redesign.md` → `docs/v1.2-tag-intelligence/design/tree-review-phase4-redesign.md`
- [x] 4.10 移动 `docs/plans/2025-01-15-tag-hierarchy-cleanup.md` → `docs/v1.2-tag-intelligence/design/tag-hierarchy-cleanup.md`
- [x] 4.11 移动 `docs/plans/2025-04-27-tag-extraction-description.md` → `docs/v1.2-tag-intelligence/design/tag-extraction-description.md`
- [x] 4.12 移动 `docs/plans/2025-04-27-unclassified-tag-clustering.md` → `docs/v1.2-tag-intelligence/design/unclassified-tag-clustering.md`
- [x] 4.13 移动 `docs/plans/tag-hierarchy-cleanup-flow.md` → `docs/v1.2-tag-intelligence/design/tag-hierarchy-cleanup-flow.md`
- [x] 4.14 移动 `docs/plans/topic-aggregation-plan.md` → `docs/v1.2-tag-intelligence/design/topic-aggregation-plan.md`
- [x] 4.15 移动 `docs/plans/topics-ui-adjustment-implementation-plan.md` → `docs/v1.2-tag-intelligence/design/topics-ui-adjustment.md`

## 5. 归类 plans/ 到里程碑（v1.2 — changes/）

- [x] 5.1 移动 `docs/plans/2026-03-12-ai-router-implementation.md` → `docs/v1.2-tag-intelligence/changes/ai-router-implementation.md`
- [x] 5.2 移动 `docs/plans/2026-04-13-embedding-queue-implementation.md` → `docs/v1.2-tag-intelligence/changes/embedding-queue-implementation.md`
- [x] 5.3 移动 `docs/plans/2026-04-15-tag-quality-score-implementation.md` → `docs/v1.2-tag-intelligence/changes/tag-quality-score-implementation.md`
- [x] 5.4 移动 `docs/plans/2026-04-16-scheduler-panel-enhancement.md` → `docs/v1.2-tag-intelligence/changes/scheduler-panel-enhancement.md`
- [x] 5.5 移动 `docs/plans/2026-04-16-settings-queue-refactor.md` → `docs/v1.2-tag-intelligence/changes/settings-queue-refactor.md`
- [x] 5.6 移动 `docs/plans/2026-04-16-tag-matching-enhancement.md` → `docs/v1.2-tag-intelligence/changes/tag-matching-enhancement.md`
- [x] 5.7 移动 `docs/plans/2026-04-17-backend-crud-performance.md` → `docs/v1.2-tag-intelligence/changes/backend-crud-performance.md`
- [x] 5.8 移动 `docs/plans/2026-04-17-tag-judgment-merge-abstract.md` → `docs/v1.2-tag-intelligence/changes/tag-judgment-merge-abstract.md`
- [x] 5.9 移动 `docs/plans/2026-04-18-batch-tag-judgment.md` → `docs/v1.2-tag-intelligence/changes/batch-tag-judgment.md`
- [x] 5.10 移动 `docs/plans/2026-04-18-dual-tag-embeddings.md` → `docs/v1.2-tag-intelligence/changes/dual-tag-embeddings.md`
- [x] 5.11 移动 `docs/plans/2026-04-18-event-tag-matching-combo.md` → `docs/v1.2-tag-intelligence/changes/event-tag-matching-combo.md`
- [x] 5.12 移动 `docs/plans/2026-04-15-narrative-summary.md` → `docs/v1.2-tag-intelligence/changes/narrative-summary.md`
- [x] 5.13 移动 `docs/plans/2026-04-19-abstract-tag-merge-consolidation.md` → `docs/v1.2-tag-intelligence/changes/abstract-tag-merge-consolidation.md`
- [x] 5.14 移动 `docs/plans/2026-04-22-tag-cleanup-redesign.md` → `docs/v1.2-tag-intelligence/changes/tag-cleanup-redesign.md`
- [x] 5.15 移动 `docs/plans/2026-04-23-cotag-event-matching-hierarchy-cleanup.md` → `docs/v1.2-tag-intelligence/changes/cotag-event-matching-hierarchy-cleanup.md`
- [x] 5.16 移动 `docs/plans/2026-04-25-tagging-pipeline-fixes.md` → `docs/v1.2-tag-intelligence/changes/tagging-pipeline-fixes.md`
- [x] 5.17 移动 `docs/plans/2026-04-25-topic-graph-algorithm-improvements.md` → `docs/v1.2-tag-intelligence/changes/topic-graph-algorithm-improvements.md`
- [x] 5.18 移动 `docs/plans/2026-04-25-tree-review-phase4-implementation.md` → `docs/v1.2-tag-intelligence/changes/tree-review-phase4-implementation.md`
- [x] 5.19 移动 `docs/plans/2026-04-26-phase4-tree-review-merge-and-root-protection.md` → `docs/v1.2-tag-intelligence/changes/phase4-tree-review-merge-and-root-protection.md`
- [x] 5.20 移动 `docs/plans/2026-04-26-tag-cleanup-llm-batch-optimization.md` → `docs/v1.2-tag-intelligence/changes/tag-cleanup-llm-batch-optimization.md`
- [x] 5.21 移动 `docs/plans/2026-04-26-tag-llm-optimization.md` → `docs/v1.2-tag-intelligence/changes/tag-llm-optimization.md`
- [x] 5.22 移动 `docs/plans/2026-04-26-unified-abstract-relationship-judgment.md` → `docs/v1.2-tag-intelligence/changes/unified-abstract-relationship-judgment.md`
- [x] 5.23 移动 `docs/plans/2026-04-27-llm-batch-optimization.md` → `docs/v1.2-tag-intelligence/changes/llm-batch-optimization.md`
- [x] 5.24 移动 `docs/plans/2026-04-27-narrative-two-pass.md` → `docs/v1.2-tag-intelligence/changes/narrative-two-pass.md`
- [x] 5.25 移动 `docs/plans/2026-04-27-tag-cleanup-llm-budget-and-timeout.md` → `docs/v1.2-tag-intelligence/changes/tag-cleanup-llm-budget-and-timeout.md`
- [x] 5.26 移动 `docs/plans/2026-04-28-remove-auto-summary-and-digest.md` → `docs/v1.2-tag-intelligence/changes/remove-auto-summary-and-digest.md`
- [x] 5.27 移动 `docs/plans/2026-04-28-tag-cleanup.md` → `docs/v1.2-tag-intelligence/changes/tag-cleanup.md`

## 6. 归类 plans/ 到里程碑（v1.3-narrative-overhaul）

- [x] 6.1 移动 `docs/plans/2026-05-10-fix-tag-hierarchy-cycle.md` → `docs/v1.3-narrative-overhaul/design/fix-tag-hierarchy-cycle.md`
- [x] 6.2 移动 `docs/plans/2026-05-10-tag-hierarchy-templates-remaining.md` → `docs/v1.3-narrative-overhaul/changes/tag-hierarchy-templates-remaining.md`

## 7. 删除冗余文件和空目录

- [x] 7.1 删除 `front/ARCHITECTURE.md`（851 行，被 reference/architecture/frontend.md 替代）
- [x] 7.2 删除 `backend-go/ARCHITECTURE.md`（825 行，过时 + 被 reference/architecture/backend.md 替代）
- [x] 7.3 删除 `docs/developer/frontend-architecture.md`（与 architecture/frontend.md 重复）
- [x] 7.4 删除 `docs/operations/architecture/` 目录（空壳）
- [x] 7.5 删除 `docs/plans/` 目录（全部内容已归类到里程碑）
- [x] 7.6 删除 `docs/architecture/` 目录（已迁移到 reference/architecture/）
- [x] 7.7 删除 `docs/releases/` 目录（已迁移到里程碑 SUMMARY.md）
- [x] 7.8 删除空的 `docs/guides/`、`docs/api/`、`docs/database/`、`docs/operations/` 目录

## 8. 精简 AI 代理指南文件

- [x] 8.1 重写根 `AGENTS.md`：精简到 ~60 行，只保留 AI 行为规则 + 链接到 docs/reference/
- [x] 8.2 重写 `front/AGENTS.md`：精简到 ~20 行，前端差异 + 链接根 AGENTS.md
- [x] 8.3 重写 `backend-go/AGENTS.md`：修正过时内容（SQLite→PostgreSQL）+ 精简到 ~20 行
- [x] 8.4 重写 `.cursorrules`：精简到 ~3 行，指向 AGENTS.md

## 9. 精简 CONTRIBUTING.md 和重写索引

- [x] 9.1 精简 `CONTRIBUTING.md` 为纯链接页（指向 getting-started.md、development.md、overview.md）
- [x] 9.2 重写 `docs/README.md` 索引：反映新的目录结构（reference/ + 里程碑 + experience/）

## 10. 验证

- [x] 10.1 确认 `docs/plans/` 不存在
- [x] 10.2 确认 `front/ARCHITECTURE.md` 和 `backend-go/ARCHITECTURE.md` 不存在
- [x] 10.3 确认 `docs/reference/` 下包含所有预期的活文档
- [x] 10.4 确认三个里程碑目录各包含 SUMMARY.md + 四个子目录
- [x] 10.5 确认 AGENTS.md（根）不超过 80 行
- [x] 10.6 确认 front/AGENTS.md 不超过 25 行
- [x] 10.7 确认 backend-go/AGENTS.md 不超过 25 行且无 SQLite 过时描述
- [x] 10.8 确认 .cursorrules 不超过 5 行
- [x] 10.9 确认所有原 plans/ 文件已归位（66 plans + 3 other = 69 files total，v1.3 已改名 narrative-overhaul）
