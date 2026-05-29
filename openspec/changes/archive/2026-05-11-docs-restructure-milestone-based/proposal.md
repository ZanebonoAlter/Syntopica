## Why

项目文档和 AI 代理规范文件经过持续迭代已严重散落和重复：同一条代码规范出现在 AGENTS.md、CLAUDE.md、CONTRIBUTING.md、.cursorrules、子目录 AGENTS.md、docs/operations/development.md 等 6+ 个位置；两份 800+ 行的 ARCHITECTURE.md（front/ 和 backend-go/）与 docs/architecture/ 下的对应文档大量重叠且部分过时（仍在描述 SQLite）；plans/ 目录堆积 67 个文件无索引无状态。没有单一真相源，改一处忘其他，矛盾已经出现。

## What Changes

- **建立里程碑主轴文档结构** — docs/ 下按 `v{version}-{name}/` 组织里程碑文件夹，每个里程碑内按 `design/`、`user-guide/`、`changes/`、`debug/` 四类分组
- **建立 reference/ 活文档层** — 跨里程碑的架构、API、数据库、开发规范等文档统一收入 `docs/reference/`，作为当前系统真实状态的唯一权威源
- **清理冗余架构文档** — 删除 `front/ARCHITECTURE.md`（851 行）和 `backend-go/ARCHITECTURE.md`（825 行，过时），由 `docs/reference/architecture/` 替代
- **归类 plans/ 到里程碑** — 67 个计划文件按粗粒度归类到 v1.1-bugfixes/、v1.2-tag-intelligence/、v1.3-active/ 对应的 design/ 或 changes/ 子目录
- **精简 AI 代理指南** — AGENTS.md（根）缩减为 ~60 行（只保留 AI 行为规则 + 链接），子目录 AGENTS.md 各缩减到 ~20 行，.cursorrules 缩减到 ~3 行
- **精简 CONTRIBUTING.md** — 改为纯链接页，指向 docs/ 下的权威文档
- **CLAUDE.md 不动** — 兜底文件，不修改

## Capabilities

### New Capabilities

- `docs-milestone-structure`: 里程碑主轴文档目录规范 — 定义里程碑文件夹命名规则、内部四类分组（design/user-guide/changes/debug）的结构要求、SUMMARY.md 格式
- `docs-reference-layer`: reference/ 活文档层规范 — 定义哪些文档属于跨里程碑活文档、归档规则、与里程碑文档的关系
- `agent-guide-slim`: AI 代理指南精简规范 — 定义 AGENTS.md 层级（根 → 子目录）的职责划分、内容范围限制、链接规则

### Modified Capabilities

（无 — 本次不涉及功能需求变更，仅涉及文档和规范组织方式）

## Impact

- **文档文件**: ~90 个文件需要移动、重写或删除（67 个 plans + ~15 个现有文档迁移 + ~8 个代理指南/规范文件重写）
- **无代码变更**: 纯文档重组，不影响前后端代码和构建流程
- **AI 代理行为**: 代理读取的指南文件内容变化，但指向的权威文档不变
- **Git 历史**: 大量文件移动，建议在 feature 分支一次性完成，合并时用 `git log --follow` 追溯
- **现有 CI/工具**: 无 CI 流水线受影响
