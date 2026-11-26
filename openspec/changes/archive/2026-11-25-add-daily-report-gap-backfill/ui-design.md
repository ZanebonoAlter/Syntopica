<!-- ui-impact: minor -->
<!-- ui-approval: not-required -->
<!-- ui-prototype: none -->

# add-daily-report-gap-backfill — UI 设计契约

（minor 档：复用既有管线状态块、既有折叠/进度/确认弹窗模式，不新增页面、不改信息架构。）

## 入口与入口变更

| 入口 | 位置 | 变更 |
| --- | --- | --- |
| 管线状态块（L0 `/tags` 右栏底部） | `BoardRadarStatusBlock.vue` 展开态 | **新增**「近 7 天缺口」折叠区（默认折叠，行头带 `N 天 / M 篇`） |
| 缺口区内「补这天」 | 同上，每个缺口日期行 | **新增**单日补填（一次请求，异步执行） |
| 缺口区底部「一次补齐」 | 同上 | **新增**全部缺口批量补填（异步 + 内联进度） |
| 「立即生成今日日报 / 重新生成今日日报」 | 同上，既有按钮 | **不变**；选择「重新生成」并提交时**新增**一次 `useConfirm` 二次确认 |
| 生成进度行 | 同上，`.rs-running` / `.rs-result` 族 | **复用**，补填进度复用同一呈现位（带日期） |

## 受影响状态

| 状态 | 影响 |
| --- | --- |
| loading | 缺口区行头显示「读取中」；SHALL NOT 把未知渲染成「无缺口」 |
| empty | 近 7 天无缺口时 SHALL 显示「近 7 天无缺口」，折叠区不出按钮 |
| error | 缺口取数失败时只让缺口区降级（显示「读取失败 · 重试」），SHALL NOT 影响卡片其余五项事实 |
| success | 补填进行中：行头转进度（`补齐中 2/5`）并禁用「补这天」「一次补齐」；终态后刷新缺口与雷达总览，缺口清空则回 empty |

## 复用组件与布局模式

- 布局模式：**不变**——管线状态块仍是右栏底部常驻卡片，`workspace` 页壳不调整；缺口区是卡片内既有 `.rs-picker` 同族的就地展开，SHALL NOT 打开弹窗、SHALL NOT 跳转页面。
- 复用组件：`AppButton`（size sm）、`AppDialog` / `AppConfirmDialog` + `useConfirm`（二次确认，尺寸档 md）、既有 `.rs-*` 样式族与折叠/进度样式；**不新增**基础组件、不新增自由宽度。
- 窄屏：沿用既有卡片宽度与换行策略，缺口日期行 `flex-wrap` 兜底。

## Interaction Decisions

| ID | 决策 | 模式 | forbidden | 状态 |
| --- | --- | --- | --- | --- |
| D1 | 缺口清单与补齐入口的承载：状态块**就地展开**，与板块勾选清单同款 | `panel` | `dialog`, `drawer`, `page` | `pinned` |
| D2 | 「重新生成今日日报」的二次确认：`useConfirm` 全局确认弹窗（危险样式） | `dialog` | `inline-edit`, `command` | `pinned` |
| D3 | 补填进度：复用卡片内既有进度行（带日期文案），不新增占位层/浮层 | `panel` | `dialog`, `drawer` | `pinned` |

## Data Provenance

| 数据项 | source | 说明 |
| --- | --- | --- |
| 缺口天数 / 缺口篇数 | `api /api/daily-reports/gaps` | 服务端按窗口计算，前端不自行推算 |
| 缺口日期行与板块名 | `api /api/daily-reports/gaps` | 日期 + 该日缺报板块 |
| 补填进度（已补/总数/当前日期） | `api /api/daily-reports/backfill-gaps`（job_id）+ WS `daily_report_progress`（新增 `date`） | 只渲染服务端计数 |
| 补填终态结果 | WS `daily_report_backfill_done` / 运行记录 | 成功、部分失败、失败、超预算四种终态 |

## Prototype

无需原型（minor 档：复用既有卡片折叠 + 既有进度行 + 既有确认弹窗，无新界面结构）。

## Acceptance

- 组件测试：`front/app/features/tags/components/console/BoardRadarStatusBlock.test.ts` —— 缺口折叠区渲染/计数/空态、`补这天` 与 `一次补齐` 的请求与禁用、补填进度、重新生成触发确认弹窗。
- 组件测试：`front/app/features/tags/components/console/BoardRadarStatusBlock.test.ts` 增补「取消确认不发起重新生成」用例。
- 命令：`cd front && pnpm test:unit app/features/tags/components/console/BoardRadarStatusBlock.test.ts --maxWorkers=2` → 全绿。
- 视觉：1440×900 与 1920×1080 两档下卡片展开态缺口区不被裁切、按钮不换行（agent-browser 截图核，证据落 `ui-verification/report.md`）。
- 双主题：浅色（`data-theme=editorial`）与深色（`data-theme=dark`）下缺口区 SHALL NOT 呈现异质嵌套面（不得自带与卡片异色的不透明底色/边框），行分隔用卡片内细线；证据截图两主题各一张。
