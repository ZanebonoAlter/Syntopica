# tasks — 雷达总览降级诚实性 + 端点性能治本

> 排期：T1.1 / T1.6（后端协议+测试）与 T1.3–T1.5 / T1.7（前端）可并行；T1.2（后端性能）依赖 design D4 的实测方案。
> 协议契约见 design D1/D2（前端按该契约实现，后端同契约落地）。

## 1. 实现

- [x] T1.1 后端协议：未取到 ≠ 0（`backend-go/internal/topicgraph/repository/board_radar_repository.go`）
  - `RadarBoardRow` 的 `attention` / `today_clues` / `last3_clues` / `prev3_clues` / `report_article_count` / `report_event_count` 改为可空（JSON `null` = 未知）；正常行仍输出数字。
  - 新增行级 `data_unavailable`（bool）：超时未完成该版块、或该版块泳道动态计算失败时为 `true`；真实空态（近 14 天无日报/无泳道）不标。
  - 超时分支与 `buildRadarRow` 的降级分支都走同一套「未取到」表达；`finalizeRadar` 的全局 `date` 在全部行未取到时不得冒充今天（保持未知）。
- [x] T1.2 后端性能治本：消掉 5 秒预算被击穿的触发器（方案见 design D4；只改性能，不改任何统计口径与数字）
- [x] T1.3 前端归一化与类型可空化（`front/app/api/normalizers/boardRadar.ts`、`front/app/api/semanticBoards.ts`）
  - `RadarBoard` 增 `dataUnavailable: boolean`；`attention` / `reportArticleCount` / `reportEventCount` 改 `number | null`（缺字段与未取到都归一为 null，不造 0）。
- [x] T1.4 前端状态派生（`front/app/features/tags/composables/useBoardRadarOverview.ts`、`front/app/features/tags/components/console/boardRadarHelpers.ts`）
  - 未取到的版块不参与关注度分档与「N 个板块值得关注」计数；`todayClueTotal` 与热点数在存在未取到版块时为未知（null）。
  - `subKind` 增 `partial`；`headline`：全部未取到 → 「本次数据未取全，请重试」；部分未取到 → 「N 个板块值得关注（M 个版块未取到）」。
- [x] T1.5 前端呈现（`front/app/features/tags/components/console/BoardRadarMasthead.vue`、`BoardRadarStory.vue`、`BoardRadarView.vue`、`BoardRadarCold.vue`）
  - 报头：未取全时热点/今日线索显示「—」，副注说明本次数据不完整。
  - 版块行：未取到 → 「数据未取到」+「—」，不显示「0 篇文章 · 0 个事件」「关注强度 0.0」「无日报日期」。
  - 视图：未取全时复用既有 `.rd-error` 错误条（带既有「重试」按钮）；全部未取到时**不得**渲染「今天没有需要看的版块」业务空态。
- [x] T1.8 前端：滞后版块沉底（用户 2026-11-19 决定）
  - 滞后判定 = 该版块自己的锚点日 ≠ 全局锚点日（`anchorDate` 为空**不算**滞后）；滞后版块不进 hot/warm/cold 任何分档、不占前排名次。
  - 主列表分段：关注版块（今日数据）→ 常温折叠段 → 滞后版块逐行（保留「数据至 MM/DD」）→ 未取到版块逐行；序号连续不重号。
  - 滞后行不挂「关注强度 · 热点/关注/常温」分档徽标、不染热点/关注色。
- [x] T1.6 后端测试（`backend-go/internal/topicgraph/`）
  - 超时降级行：标 `data_unavailable=true` 且计数类字段为 `null`（不是 0）。
  - 计算失败降级行：同样标 `data_unavailable=true`。
  - 真实零值行：仍为 0 且不标 `data_unavailable`。
  - 全部行未取到时全局 `date` 不冒充今天。
- [x] T1.7 前端测试（`front/app/features/tags/`）
  - `useBoardRadarOverview`：全部未取到 → headline 不含「0 个板块值得关注」；部分未取到 → 统计为 null；未取到版块不进 hot/warm/cold 分档。
  - 滞后版块（锚点日 ≠ 全局锚点日）不进 hot/warm/cold，出现在末尾滞后段；`anchorDate` 为空的行仍按常温处理。
  - 行渲染：未取到版块显示「数据未取到」，不显示 0.0 / 0 篇文章；滞后行带「数据至 MM/DD」且不带分档徽标。

## 2. Review

- [x] T2.1 派子线程聚焦 review（审查制品 + diff），意见按 High/Medium/Low 处置后 review.md 建账（两轮审查：`review-brief.md` + `review.md` 建账 7 条 R-1~R-7，全部已处置并复审通过；`review-trace.sh` 退出码 0）

## 3. 测试

- [x] T3.1 后端：`cd backend-go && go test -p 1 ./internal/topicgraph/... -count=1`（包级测试会各起一个 Postgres testcontainer，树莓派上必须 `-p 1` 串行）
- [x] T3.2 前端：`cd front && pnpm test:unit app/features/tags/composables/useBoardRadarOverview.test.ts app/features/tags/components/console/BoardRadarView.test.ts app/features/tags/components/console/BoardRadarStory.test.ts app/features/tags/components/console/BoardRadarMasthead.test.ts app/api/normalizers/boardRadar.test.ts --maxWorkers=2`

## 4. 文档

<!-- doc-impact: api, flow, configuration -->
<!-- doc-impact-excuse: standard=theming.md 与 main.css 的 `.scroll-thin` 滚动条规范改动属 unify-inner-scrollbar-style（已归档），本 change 未碰标准域文档 -->

- [x] T4.1 `docs/reference/api/daily-reports.md` — GET `/semantic-boards/overview` 契约段：补 `data_unavailable` 与「计数类字段可空 = 未知」语义（现状写的「单版块查询超时返回零值并标 `data_delayed=true`」要改成未取到语义）。
- [x] T4.2 `docs/reference/flow/semantic-board.md` — L0 雷达段补「未取到 ≠ 0」与「滞后版块沉底」的呈现契约；「变更溯源」行按 §12.2 在**归档后**补（archive 目录名生成后再写）。
- [x] T4.3 `docs/reference/configuration.md` — 连接池调参（`max_open_conns` 40 / `max_idle_conns` 25）与原因说明（随 D7 同步更新）。

## 5. 验证

- [x] T5.1 `cd backend-go && go test -p 1 ./internal/topicgraph/... -count=1` → 全绿（含新增的超时/计算失败降级用例与批量口径回归用例）。
- [x] T5.2 `cd front && pnpm exec nuxi typecheck` → 无类型错误（可空化后的 `attention` 等字段全链路 null-safe）。
- [x] T5.3 `cd front && pnpm test:unit app/features/tags/composables/useBoardRadarOverview.test.ts app/features/tags/components/console/BoardRadarView.test.ts app/features/tags/components/console/BoardRadarStory.test.ts app/features/tags/components/console/BoardRadarMasthead.test.ts app/api/normalizers/boardRadar.test.ts --maxWorkers=2` → 全绿（**minor 档 UI 验收映射指名的组件测试文件路径**）。
- [x] T5.4 `curl -s --noproxy '*' localhost:5100/api/semantic-boards/overview` 连续 10 次 → 正常路径下 `data_unavailable` 全为 `false`、数值与改动前一致（关注度最高 16.5、当日线索合计 66）、墙钟 p95 ≪ 5s 预算；人为制造降级时未取到的行计数为 `null` 且标 `data_unavailable=true`。
- [x] T5.5 `bash scripts/harness/check-standards.sh` 与 `bash scripts/harness/doc-impact.sh verify openspec/changes/fix-radar-overview-false-zero` → 退出码 0。
- [x] T5.6 `bash scripts/dev/deploy-frontend.sh` → 构建静态产物并铺到 `backend-go/frontend/`、重启后端、健康检查通过（日常访问走静态托管，不留 dev server）。
