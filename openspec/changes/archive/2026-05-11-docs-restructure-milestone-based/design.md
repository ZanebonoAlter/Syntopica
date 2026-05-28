## Context

当前 `docs/` 按功能分目录（architecture/、guides/、api/、operations/、plans/、releases/），但代码规范和架构文档散落在根目录 AGENTS.md、CLAUDE.md、.cursorrules、子目录 AGENTS.md、子目录 ARCHITECTURE.md 等多处，无单一真相源。plans/ 堆积 67 个文件无状态标记。两份 800+ 行 ARCHITECTURE.md 与 docs/architecture/ 重叠且部分过时。

## Goals / Non-Goals

**Goals:**

- docs/ 以里程碑为主轴，按 `v{version}-{semantic-name}/` 组织
- 每个里程碑内按 design/、user-guide/、changes/、debug/ 四类分组
- 跨里程碑活文档收入 `docs/reference/`，作为唯一权威源
- AI 代理指南精简为层级引用结构（根 ~60 行，子目录 ~20 行）
- 清理全部冗余文档（删除 front/ARCHITECTURE.md、backend-go/ARCHITECTURE.md 等）
- plans/ 的 67 个文件归类到对应里程碑

**Non-Goals:**

- 不修改 CLAUDE.md（兜底文件）
- 不修改前后端代码
- 不引入新的文档工具或流程（不做 CI 文档检查、不引入 docusaurus 等）
- 不重写文档内容本身（只做组织结构调整，内容准确性不在本次范围）
- 不处理 docs/ 以外的散落文档（wiki/、llm-cost-analysis/ 等）

## Decisions

### D1: 文档组织主轴 = 里程碑

**选择**: 里程碑（版本）为主轴，功能类型为二级分类
**替代方案**: 纯功能分目录（现状）、纯时间线（按年月）
**理由**: 项目开发节奏以大版本推进（v1.1-bugfixes、v1.2-tag-intelligence），团队成员（和 AI 代理）按"最近做了什么"来回溯，里程碑主轴匹配心智模型。

**目录结构**:
```
docs/
├── README.md                    ← 总索引
├── getting-started.md           ← 快速开始
├── reference/                   ← 跨里程碑活文档
│   ├── architecture/            ← 唯一架构真相源
│   ├── api/                     ← API 参考
│   ├── database/                ← 数据库参考
│   ├── development.md           ← 开发规范
│   ├── configuration.md
│   ├── deployment.md
│   ├── testing.md
│   ├── content-processing.md
│   ├── reading-preferences.md
│   └── frontend-features.md
├── v{version}-{name}/           ← 各里程碑
│   ├── SUMMARY.md               ← 里程碑总结
│   ├── design/                  ← 设计文档
│   ├── user-guide/              ← 用户文档
│   ├── changes/                 ← 变更/实施记录
│   └── debug/                   ← 调试记录
├── experience/                  ← 经验沉淀（跨里程碑）
```

### D2: reference/ 代表"当前系统真实状态"

**选择**: reference/ 下的文档是活文档，随系统演进持续更新
**规则**: 
- 架构文档只在 reference/architecture/ 维护，不出现第二份拷贝
- 每个里程碑完成时更新对应的 reference 文档
- 里程碑文件夹内的 design/ 是历史快照，不回删

### D3: 里程碑命名规则

**选择**: `v{major}.{minor}-{semantic-kebab-name}`
**示例**: `v1.1-bugfixes/`、`v1.2-tag-intelligence/`、`v1.3-active/`
**理由**: 版本号 + 语义名兼顾排序和可读性。当前活跃里程碑用 `v1.3-active/`，完成后重命名为语义名。

### D4: plans/ 归类粒度

**选择**: 粗粒度 — 只分 design/ 和 changes/，不进一步按功能域子分组
**理由**: 67 个文件如果再按功能域分（tags/、ai-router/、scheduler/...）会导致目录过深且边界模糊。粗粒度归类够用。

**归类规则**:
- 带 `-design` 或 `-redesign` 后缀的 → design/
- 带 `-implementation`、`-fix`、`-enhancement` 或无后缀的 → changes/
- 带日期前缀的按日期范围归类到里程碑
- 无日期前缀的 3 个文件（tag-hierarchy-cleanup-flow、topic-aggregation-plan、topics-ui-adjustment-implementation-plan）归入 v1.2

### D5: plans/ 到里程碑的映射表

**v1.1-bugfixes/** (2026-03 至 2026-04-09):
- design/: firecrawl-integration-test-design, repo-reorganization-design, scheduler-status-trigger-design, postgres-pgvector-single-database-architecture-migration
- changes/: firecrawl-integration-test, repo-reorganization, scheduler-status-trigger, article-content-source-toggle, backend-doc-cleanup, backend-runtime-closure, retry-repair, database-schema-dedup, sqlite-archive-docker-design, sqlite-archive-docker, topic-graph-homepage-progress, topic-graph-homepage, topic-graph-refactor, topics-classified-analysis-implementation, pending-articles-node-design, pending-articles-node

**v1.2-tag-intelligence/** (2026-04-13 至 2026-04-28):
- design/: ai-router-design, tag-weight-convergence-design, embedding-queue-design, tag-quality-score-design, scheduler-panel-enhancement-design, narrative-feed-category-design, tag-elimination-design, cross-category-narrative-design, tag-cleanup-redesign, tree-review-phase4-redesign, persistent-firecrawl-tag-queues-design, topic-aggregation-plan, topics-ui-adjustment-implementation-plan, tag-hierarchy-cleanup-flow, tag-hierarchy-cleanup, tag-extraction-description, unclassified-tag-clustering
- changes/: ai-router-implementation, embedding-queue-implementation, tag-quality-score-implementation, scheduler-panel-enhancement, settings-queue-refactor, tag-matching-enhancement, backend-crud-performance, tag-judgment-merge-abstract, batch-tag-judgment, dual-tag-embeddings, event-tag-matching-combo, narrative-summary, abstract-tag-merge-consolidation, cotag-event-matching-hierarchy-cleanup, tagging-pipeline-fixes, topic-graph-algorithm-improvements, tree-review-phase4-implementation, phase4-tree-review-merge-and-root-protection, tag-cleanup-llm-batch-optimization, tag-llm-optimization, unified-abstract-relationship-judgment, llm-batch-optimization, narrative-two-pass, tag-cleanup-llm-budget-and-timeout, remove-auto-summary-and-digest, tag-cleanup, persistent-firecrawl-tag-queues

**v1.3-active/** (2026-05-10+):
- design/: fix-tag-hierarchy-cycle
- changes/: tag-hierarchy-templates-remaining

### D6: AI 代理指南层级

**选择**: 根 AGENTS.md 是唯一规范入口，子目录只放差异 + 链接

**层级结构**:
```
AGENTS.md (根, ~60 行)
  ├── 项目快照 (5 行)
  ├── 代码规范: "详见 docs/reference/development.md" (链接)
  ├── 架构: "详见 docs/reference/architecture/" (链接)
  ├── AI 行为规则 (20 行, 只放 AI 特有的规则)
  ├── GitNexus 工作流 (保留, 不变)
  └── 构建命令 (10 行摘要)

front/AGENTS.md (~20 行)
  ├── "遵循根 AGENTS.md"
  ├── 前端特有差异 (导入顺序、组件命名、数据映射)
  └── 链接到 docs/reference/

backend-go/AGENTS.md (~20 行)
  ├── "遵循根 AGENTS.md"
  ├── 后端特有差异 (handler 模式、JSON tag、错误包装)
  └── 链接到 docs/reference/

.cursorrules (~3 行)
  └── "遵循 AGENTS.md" + graph 工具简述
```

### D7: 需要删除的文件

| 文件 | 原因 |
|------|------|
| front/ARCHITECTURE.md (851 行) | 被 docs/reference/architecture/frontend.md 替代 |
| backend-go/ARCHITECTURE.md (825 行) | 过时 + 被 docs/reference/architecture/backend.md 替代 |
| docs/developer/frontend-architecture.md | 和 architecture/frontend.md 重复 |
| docs/operations/architecture/ (目录) | 空壳，只有一个 README |

### D8: CONTRIBUTING.md 精简

改为纯链接页，指向:
- docs/getting-started.md — 环境搭建
- docs/reference/development.md — 开发规范
- docs/reference/architecture/overview.md — 架构

## Risks / Trade-offs

**[大量文件移动导致 Git 历史断裂]** → 使用 feature 分支一次性完成，合并后在 README.md 说明追溯方式。考虑使用 `git mv` 保留追踪。

**[AI 代理在迁移期间读到旧路径]** → 迁移在单次 commit 中完成，不设中间状态。旧位置的文件要么移动要么删除，不留过时副本。

**[里程碑归类主观性]** → plans/ 的归类不可能完美，但比现在 67 个平铺好得多。README.md 中标注归类逻辑，后续可调整。

**[reference/ 文档内容过时（如 backend-go 仍写 SQLite）]** → 本次只做组织迁移，不重写内容。但会在 tasks 中标记"过时内容待更新"作为 follow-up。

## Open Questions

- v1.3-active/ 完成后的语义名是什么？（暂用 `active`，完成时再定）
