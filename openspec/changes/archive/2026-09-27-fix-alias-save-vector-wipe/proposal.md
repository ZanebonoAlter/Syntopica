<!-- complexity: simple -->
<!-- ui-impact: none -->
<!-- constraint-domains: semantic-board -->

## Why

巡检发现日报泳道偏离版块的根因之一是 aux 标签向量被静默清零：`loadActiveAuxiliaryLabels` 的 `Select` 为缓存性能刻意排除 embedding/merge_embedding 两列，而 L2 命中路径 `addAlias` 拿着这个**不含向量的结构体**执行 GORM `Save(label)` 全量保存，nil 指针字段被写成 NULL——标签向量丢失且 status 仍 active，无任何报错。现状全库 1505/17525（8.6%）active aux 标签死向量，11/12 版块构成被截肢（死的恰是各版块最高 ref 核心标签：习近平/特朗普/OpenAI/高市早苗/日本央行…），直接后果是版块匹配面残缺、错版块收留 + 真版块拒收（真实案例：日本首相 tag 挂进「中国国内新闻」而「日本新闻」拒收）。清零速率随打标量增长（9/21 后 60-300 个/天），不修会持续恶化。

证据链：死向量标签 100% 带 aliases（addAlias 痕迹）、活向量标签 100% 无 aliases；受害行 updated_at 聚集在午夜打标批处理；引入于 optimize-pg-storage 时代的缓存 Select 改造（2026-08-31~09-04）。详见 `docs/research/board-matching-leak-patrol/explore-findings.md`。

## What Changes

- **修复 addAlias 清向量 bug**：`AuxiliaryLabelService.addAlias` 从 `Save(label)` 全量保存改为列级 `Update("aliases", ...)`（只动 aliases 列），不再触碰 embedding/merge_embedding 及其他列。
- **审计并加固同型写路径**：排查 `semantic_labels` 表上所有「结构体不含向量却全量 Save」的写点（如 `MergeAuxLabels` 的 `tx.Save(&target)`——target 若为全列加载则安全，但统一改为列级更新以绝后患）。
- **死向量回填**：为存量 active 且 embedding/merge_embedding 为 NULL 的 aux 标签重建向量（复用 L3 的 LLM embedding 生成逻辑：storage=「label: description」、merge=label），提供手动触发的回填端点（POST 异步，遵循仓库既有 backfill 端点模式）。
- **回归防护**：单测覆盖 addAlias/merge 路径不触碰向量列（建行带向量 → addAlias → 向量原样保留）。

不动 weighted 规则后门与方向校验阈值——那是巡检发现的第二/三层根因，属后续独立 change 的范围。

## Capabilities

### New Capabilities

（无）

### Modified Capabilities

- `auxiliary-label`: 新增「alias 变更不得触碰向量列」需求（addAlias/alias 合并路径的向量保持不变量，含回归 scenario）；新增「死向量回填」需求（active+NULL 向量 aux 标签的手动回填端点行为）。

## Impact

- **后端代码**：`backend-go/internal/tagmanagement/service/auxlabel/auxiliary_label_service.go`（addAlias、MergeAuxLabels 写路径改造）、回填端点（handler + service，挂 `/api/` 下 tagmanagement 路由组）。
- **数据**：回填完成后 `semantic_labels` 中 active aux 死向量行恢复向量；版块构成标签（board_composition 引用）匹配面恢复，预期日报泳道锚定显著改善（错挂 tag 会在下次全量重算时被真版块重新竞争）。
- **LLM 调用**：回填一次性消耗约 1500×2 次 embedding 调用（走既有 airouter，operation 复用 `auxiliary_label_storage_embedding`/`auxiliary_label_merge_embedding`）。
- **不涉及前端**：无 UI 变化（ui-impact: none）。
- **部署后影响**：bug 修复即止血（新标签不再被清向量）；回填端点需用户手动触发一次；回填后建议跑一轮 `rematch-all` 让版块挂载重新竞争（将在 tasks 中列为可选操作）。
