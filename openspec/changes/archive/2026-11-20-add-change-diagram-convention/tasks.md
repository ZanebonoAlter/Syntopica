## 1. 样板

- [x] 1.1 在 `openspec/changes/archive/2026-11-20-fix-disaster-recovery-channel/` 新增 `design-diagram.md`，含三类图（核心路径 flowchart / 状态流转 stateDiagram-v2 / 顺序图 sequenceDiagram），验证：文件存在且每个代码块标注 `mermaid`、块前有问题行、块后有约束行
- [x] 1.2 图的事实核对：三条根因（`schema_migrations` 排除、`vector(4096)` 迁移、扩展未预装）与恢复链路（新建库 → 预装扩展 → TOC 过滤 → restore → count 校验 → 迁移对账）逐条对照原 `design.md` / `proposal.md`，验证：`grep -c "schema_migrations\|vector(4096)\|pg_trgm" design-diagram.md` 非零且与原文表述一致

## 2. 规范与推广

- [x] 2.1 `docs/reference/开发执行规范.md` 新增 §13「change 图表达（design-diagram.md）」：性质、三类图判据、写作约束、样板路径、可复制骨架，验证：`grep -n "## 13. change 图表达" docs/reference/开发执行规范.md` 命中
- [x] 2.2 同文档 §0.6 步骤 1 与 §9 产出物清单各加一句指引，验证：`grep -c "design-diagram" docs/reference/开发执行规范.md` ≥3
- [x] 2.3 §13 内三段 mermaid 骨架片段语法有效，验证：三段骨架一并过 mermaid 解析（与样板同一脚本）

## 3. 测试

- [x] 3.1 `openspec validate add-change-diagram-convention --type change` 期望通过
- [x] 3.2 `bash scripts/harness/check-tasks-tail.sh openspec/changes/add-change-diagram-convention` 期望退出码 0
- [x] 3.3 mermaid 语法抽查：对 `design-diagram.md` 三个代码块做渲染验证（前端页面或 `npx @mermaid-js/mermaid-cli`），期望无语法错误
- [x] 3.4 归档目录只新增文件：`git status --porcelain openspec/changes/archive/2026-11-20-fix-disaster-recovery-channel/` 期望只有 `?? design-diagram.md` 一条

## 4. 文档

<!-- doc-impact: standard(只改 doc-authoring 图表达规范与开发执行规范指引，不涉 flow/api/database/architecture) -->

> **无 flow 影响**：本 change 只动图表达规范（`docs/reference/standard/**` + 开发执行规范 §13），不改任何业务域流程语义，故不补 flow 变更溯源行（§12.2 豁免）。

- [x] 4.1 §13 的样板路径与契约源引用可解析：`grep -n "archive/2026-11-20-fix-disaster-recovery-channel/design-diagram.md\|change-diagram-convention/spec.md" docs/reference/开发执行规范.md` 两条均命中

## 5. 验证

- [x] 5.1 `bash scripts/harness/doc-impact.sh verify openspec/changes/add-change-diagram-convention/` 期望 PASS
- [x] 5.2 `bash scripts/harness/archive-readiness.sh add-change-diagram-convention` 期望七项全绿
- [x] 5.3 `grep -rn "design-diagram.md" openspec/specs docs/reference | wc -l` 期望 ≥3（主 spec 同步后 + 两处规范）
