<!-- ui-impact: none -->
<!-- ui-approval: not-required -->
<!-- ui-prototype: none -->

## N/A Reason

本 change 只改后端体检产出与触发：LLM 分诊清单的覆盖校验（漏判补齐）、体检日志、三条触发路径与两级超时预算。不改任何页面、组件、路由、交互或信息架构。

## 复用契约

前端已有两条相关分支，本 change 不改它们、只让降级态更少出现：

- 降级态：`front/app/features/tags/components/console/board-health/boardHealthWorkspace.ts:281` 判 `degraded` → 标题「本轮未生成评分与分诊」；`RailHealthBrief.vue:62` 徽章「待评分」。
- ⚪ 证据不足：`front/app/features/tags/components/console/board-health/boardHealthTriage.ts:131` 已按 category 分支渲染，补齐项走同一分支（不新增展示逻辑）。

## Acceptance

不做视觉验收。数据面验收见 test-cases.md §3（真库一轮实测：`llm_score` 非空且 `triage 长度 == labels 长度`）。若部署后发现前端对新增补齐项有渲染异常，另开 minor change 处理。
