<!-- ui-impact: minor -->
<!-- ui-approval: not-required -->
<!-- ui-prototype: none -->

# UI 复用契约

## 入口与变更

- **既有叙事工坊首屏（`/tags`）右栏「今日该读」块**：条目从「各版块最新日报标题 + 日期 + 文章数 + 事件数」换成「跨版块全局 Top N 事件线」。每条 = 版块名 + 事件线标题 + 来源数 + 热度；条目可点，跳该事件线所在日报并定位到正文对应 thread。
- **既有右栏「今天可能升温」块**：条目从「版块名 + 版块级倍率 + 版块理由句」换成「事件线标题 + 该事件线自身的倍率 + 该事件线自身的理由句」，并在其下新增「今日新起」子档（近 3 日无基线的新事件线，**不显示倍率**）。
- **既有日报阅读台（`/tags/:boardId/:date`）**：新增 `?story=<id>` 深链入口，落地行为复用刊头「今日重点」已有的正文定位路径（展开 → 滚动 → 聚焦 → 高亮 2s），不新增定位机制。
- 无新路由、无新弹层、无新面板；版块行的「最新日报标题」入口、14 日小矩阵、关注度分档、报头操作区全部保持不变。块标题文案沿用现有「今日该读」「今天可能升温」（用户口述为「今日该看」「今日升温」，本 change 不动标题文案，只换条目内容）。无需 UI 原型。

## 受影响状态

| 状态 | 今日该读（事件线） | 今日升温 / 今日新起 |
|---|---|---|
| 成功 | 跨版块混排，每条带版块名、事件线标题、来源数、热度 | 升温条带自身倍率与自身理由句；新起条不带倍率 |
| 旧响应（后端无 `stories` 段） | 回退既有版块级日报标题列表 | 回退既有版块级升温列表（不报空态） |
| 未回填（`stories_backfilled=false`） | 该版块不产生条目；区块上方标「N 个版块事件身份未回填，未纳入事件线排序」，SHALL NOT 呈现为「今天没有事件」 | 同左 |
| 空态 | 「今天没有可识别的事件线。」 | 升温：「暂未触发升温条件。」；新起：「今天没有新起的事件线。」 |
| 加载 / 错误 | 沿用既有骨架与错误条；首次取数失败不混业务空结论 | 同左 |
| 延迟 / 不完整 | 标注「部分版块数据延迟，本次事件线排序不完整」 | 同左；`heat_incomplete` 标记照旧透出 |
| 无正文落点 | 条目不渲染为可点，title/aria 注「该事件线暂无正文落点」 | 同左 |
| 长文 / 窄屏 | 事件线标题换行不裁断，来源数与热度同行可读 | 同左 |

## 复用组件与布局模式

- 沿用既有 `AppPageShell mode="workspace"` 与右栏两段容器（上滚下驻），不改栏宽、不新增断点；右栏宽度维持既有 `clamp(17rem, 22vw, 29rem)`，条目数上限（Top 5 / 5 / 3）由内容长度而非新滚动容器承载。
- 复用 `BoardRadarReadingRail.vue` 既有 `.rd-read` / `.rd-read-board` / `.rd-read-title` / `.rd-read-meta` / `.rd-warm` / `.rd-warm-why` 样式骨架与主题 token（`--color-accent`、`--color-text-muted` 等），不硬编码新色值；新增的「今日新起」子档样式与既有 `.rd-warm-block` 同族（同一 token、同一边框/底色配方）。
- 版块名沿用 `.rd-read-board` 的强调色小字；事件线标题沿用 `.rd-read-title` 衬线体；来源数与热度沿用 `.rd-read-meta` 次要色小字；倍率沿用 `.rd-warm b` 的等宽强调体。
- 日报侧复用刊头重点定位的既有实现（`report-thread-{thread.id}` 锚点、2s 单高亮、reduced-motion 不平滑滚动、划选不触发定位），不新写一套定位或高亮样式。
- 可达性：条目为 `<button>`（Tab/Enter 可达）；title/aria 给出「版块名 · 事件线标题 · N 个来源 · 热度 X.X」；倍率不是唯一信息载体（文字理由句同时给出当日热度与近 3 日基线）。
- 目标检查：1440×900、1920×1080，另检 390×844 换行不溢出；沿用既有布局，无新弹层尺寸档。

## 验收映射

| 验收点 | 落点 |
|---|---|
| 跨版块混排、每条带版块名/来源数/热度、排序键 | `front/app/features/tags/components/console/BoardRadarView.test.ts`；`backend-go/internal/topicgraph/repository/board_radar_repository_test.go` |
| 升温按事件线级倍率、新起档不显示倍率 | `front/app/features/tags/components/console/BoardRadarView.test.ts` |
| 未回填标注、降级不完整标注、空态文案 | `front/app/features/tags/components/console/BoardRadarView.test.ts` |
| 旧响应回退版块级列表 | `front/app/features/tags/composables/useBoardRadarOverview.test.ts` |
| 响应字段归一（camelCase + 字符串 ID） | `front/app/api/normalizers/boardRadar.test.ts` |
| `?story=` 深链定位与失去目标 | `front/app/features/tags/components/DailyReportReader.test.ts` |
| 完整故事与双桌面视口 / 窄屏 | 人工：实施后用 agent-browser 访问静态 :5100，从雷达右栏点事件线 → 日报定位，检查实际滚动位置 / 焦点 / 单高亮与缺目标降级，证据建账本 change `ui-verification/report.md`；本阶段未执行，不预写通过 |
