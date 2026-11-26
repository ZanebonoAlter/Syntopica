<!-- complexity: simple -->
<!-- ui-impact: none -->

# Proposal: 修 edit.map 对「部署产物」的误记账，止住 doc-impact 归属污染

## Why

`deploy-frontend.sh` 每次部署会写 `backend-go/frontend/`（静态托管产物）与 `backend-go/.frontend-before-<随机>.KtDRQM/`（备份目录），quality-gate 的 `edit.map` 把这些**构建产物**当成"本次编辑过的文件"记账。

后果是**污染后续所有 change 的归属判定**：`doc-impact.sh verify` 的启发式输入取 `edit.map` 归属集，于是归档门禁会报出 `api / database / architecture / standard` 四类"疑似遗漏"——而这些路径根本不是任何 change 写的。

实测证据（change `rework-lane-compose-swimlane-canvas` 归档时）：

```
edit.map 归一化后（该 change）：
  ["--full-page",
   ".pi/prototypes/archive-date/index.html",
   "backend-go/.frontend-before-matrix-copy.KtDRQM/200.html",
   "backend-go/.frontend-before-matrix-copy.KtDRQM/_nuxt/2fUfdcv6.js",
   … 1195 条 backend-go 路径]

该 change 实际只改了：front/app/features/tags/ 下 9 个文件 + 3 个 flow 文档
verify 结果：4 FAIL「疑似遗漏: 改了 api/database/architecture/standard 未声明」
```

同时暴露第二个缺陷：**`--full-page` 这个 CLI flag 被当成文件路径**混进了归属集——说明采集侧对 argv token 没有做「是不是路径」的判定。

危害不止误报：`doc-impact-excuse` 这种留痕豁免会被滥用成常态（本次就是靠它过的），而豁免多了就失去对账意义；`failure-classify` 的 `isForeignFailure`（本会话/他人文件判定）也吃同一份路径集，污染会让人误判"这红是不是我搞的"。

## What Changes

1. **采集侧扩展工具自管路径判定**：`isToolManagedPath`（`edit-map.ts`）从「openspec 归档产物 + change 元数据」扩到**构建/部署产物**——
   `backend-go/frontend/**`、`backend-go/.frontend-before-*/**`、`front/.output/**`、`front/.nuxt/**`、`front/dist/**`、`.pi/prototypes/**` 的内联产物形态（按实际存在形态定稿）。
2. **采集侧剔除 argv token**：形如 `--xxx` / `-x` 的条目不是文件路径，SHALL NOT 进入 `edit.map`。
3. **消费侧兜底**：`doc-impact.sh` 读 `ownership_paths` 时施加同一过滤（存量污染不清洗，但读取侧不再吃到；与现有一句「存量污染不清洗、新快照不携带」的采集侧纪律同源）。
4. **回归测试**：`edit-map` 的单测补两类用例（部署产物路径被剔除、`--flag` 被剔除）；`doc-impact` 若有单测则补 ownership 过滤用例。

## Capabilities

### New Capabilities
（无）

### Modified Capabilities
（无代码能力变更——纯 harness/工具链，spec-level 行为不变）

> 本 change 属纯工具链改动，`specs` 阶段按 schema 要求走「无变更」声明；若 `openspec validate` 因零 delta 报错，按 CLI 提示在 `.openspec.yaml` 置 `skip_specs: true`。

## Non-Goals

- **不清洗历史污染**：已入库的 `edit.map` 事件不改写（append-only 纪律）；只保证新快照干净、读取侧不再吃到。
- **不改 `deploy-frontend.sh` 的产物布局**：`backend-go/frontend/` 与备份目录是既有部署契约，不动。
- **不顺手改「3000 端口废弃」**：`start-dev.sh` 默认还起 3000 前端，与本次无关，另开。
- **不放宽 `doc-impact-excuse`**：修完之后豁免应当是例外而非常态，本 change 不调整其语法或语义。

## Impact

- `.pi/extensions/lib/edit-map.ts`（`isToolManagedPath` + payload 构造）
- `.pi/extensions/quality-gate.ts`（若需在 trigger 收集处前置过滤 argv token）
- `scripts/harness/doc-impact.sh`（`ownership_paths` 输出加同源过滤）
- 测试：`.pi/extensions/tests/`（edit-map / quality-gate 方向）、`scripts/harness` 若有自测则补
- **部署后影响**：无用户可见变化（纯记账口径）；效果是以后每次 `deploy-frontend.sh` 之后跑归档门禁不再误报 4 个域。
- **需要的操作**：无。历史已污染的快照仍会留在库里，但读取侧过滤后不再影响判定。
