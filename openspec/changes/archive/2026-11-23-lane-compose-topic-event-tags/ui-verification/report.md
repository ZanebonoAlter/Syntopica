# UI 验证 · 编排态「已确认关键线索」★ + 工具条入口隐藏

- 环境：本地静态产物（`front/.output/server/index.mjs` :3000）+ 后端 :5100（本 change 新代码），board 1980「生成式 AI 与大模型厂商」，30 天窗口 2026-10-24 — 2026-11-22，阈值「全部」。
- 视口：headless 实测 innerWidth×innerHeight = **1280×633**（agent-browser 启动固定，`viewport` 子命令不存在）。编排面顶部占 560px，画布可视高度仅 238px——星形的像素细节以 DOM 断言 + 节点卡文本 + 已批准原型 `ui-prototype/states/02-editing.svg` 为准。
- 种子数据：该 board 在窗口内有 222 个 section、21 个带 `confirmed_key_events`；画布默认只渲染候选链成员与阈值内散料，切「全部」后渲染 48 个节点，其中 1 个星形。

### A-1 工具条两个入口不存在【pass】
- `document.querySelector('[data-testid="lane-compose-show-assigned"]')` → `null`
- `document.querySelector('[data-testid="lane-compose-history-open"]')` → `null`
- 工具条只剩「取消聚焦 / 仅高相关 / 标准 / 全部」（截图 `toolbar-no-entries.png`）。

### A-2 带确认注记的节点渲染星形【pass】
- `document.querySelectorAll('[data-role="key-star"]').length === 1`（窗口内渲染 48 节点，其余带标记的 section 在 active 只读层/未渲染层）。
- 星形元素 `tagName === 'path'`（不是圆点 + 角标）；外接尺寸与同半径圆点一致、已选 ✓ 缩小不溢出由组件测试覆盖：`LaneComposeCanvas.test.ts` → `LaneComposeCanvas — 已确认关键线索星形`（5 条）。

### A-3 节点卡逐条列出快照标题【pass】（截图 `node-card-keyevents.png`）
- 点击星形节点后卡文本：
  `微软拟售OpenAI模型使用权遭美众院问询，Anthropic完成史上最大IPO` / `2026-11-19` / `★ 已确认关键线索：Anthropic以每股83美元筹资810亿美元完成史上最大IPO` / `5 条原关系` / `＋ 选入本次泳道`。

### A-4 左栏成员条目 / 右栏②条目 ★【未覆盖（数据）】
- 本窗口 4 条候选链成员与 0 条补充推荐均无 confirmed 注记，DOM 命中 0；组件测试覆盖：`BoardComposeOverlay — 左栏成员 ★`、`LaneComposeInspector — 右栏② ★`。

### A-5 后端投影字段【pass】
- `GET /api/semantic-boards/1980/persistent-topics/compose-candidates?days=30&omit_embeddings=1` → `200`，222 sections / 21 带 `confirmed_key_events`。
- 示例：`{ "marker_id": 182, "thread_id": 15257, "title": "模型竞赛升温：Kimi与GLM同日释放升级信号", "confirmed_at": "2026-10-24T21:48:24+08:00" }`。
- 空集合契约：无标记 section 序列化为 `confirmed_key_events: []`（handler 测试 `TestComposeCandidates_ConfirmedKeyEventsEmptyArray`）。

### A-6 双视口【未覆盖】
- 1440×900 / 1920×1080 实拍未执行（headless 视口固定 1280×633）。
- 静态依据：本 change 未改栏宽 / 行高 / 布局模式（三栏 `300px / 1fr / 320px`、日期轴、行高沿用已归档 change `2026-11-23-rework-lane-compose-swimlane-canvas` 的验收结果）；组件测试覆盖行几何自查。

## 与原型对账

| 原型增量 | 实现落点 | 结果 |
|---|---|---|
| 已选节点 → 实心星 + 缩小 ✓ | `LaneComposeCanvas.vue` `starPath` + `checkPath` `is-star` | 组件测试覆盖 |
| 未选节点 → 空心星 | 同上（`hasKeyStar` 分支） | 实拍 1 例（A-2） |
| 左栏条目日期行 ★ | `LaneComposeCandidates.vue` | 组件测试覆盖（本数据无样例） |
| 右栏②条目日期行 ★ | `LaneComposeInspector.vue` | 组件测试覆盖（本数据无样例） |
| 节点卡明细行 | `LaneComposeCanvas.vue` 卡内 `key-events` | 实拍 A-3 |

## 残余风险

- **星形在 0.5× 缩放下辨识度**：沿用通用像素下限（渲染半径 ≥3px，非星形专属），星形面积约为同半径圆的一半；本次未在 50% 实拍，属已知取舍，不引入星形专属放大（放大=伪造接近度）。
- **真实数据里 ★ 密度低**：board 1980 的 21 个带标记 section 只有 1 个进入当前画布层，用户首屏可能看不到星形；属数据分布，不是渲染缺陷。
