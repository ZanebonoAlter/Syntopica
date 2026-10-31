# UI 验收报告 — add-radar-pipeline-status

> 变更：L0 雷达总览页（`/tags`）右栏底部新增管线状态块（**双队列**：标签队列 / Embedding 队列）+ 手动日报生成入口（带板块勾选）。
> 验收方式：agent-browser 实页（真实后端 + 真实数据库）；仅「构造特定队列状态」用网络 mock（不动数据库）。
> 日期：2026-10-30　环境：树莓派 arm64 / Debian 13，同源静态托管 `http://localhost:5100`。

## 一、验收结论

**pass**（本 change 新增区域全项通过；未覆盖项见 §三）。

证据文件（同目录）：

| 文件 | 视口 / 主题 | 内容 |
| --- | --- | --- |
| `radar-status-1440-editorial.png` | 1440×900 / editorial | 状态块常驻右栏底部，**两个队列各占一行**（真实数据：标签待打标 / Embedding 待向量化） |
| `radar-status-1440-dark.png` | 1440×900 / dark | 暗色主题（token 派生底色，非硬编码） |
| `radar-status-1920-editorial.png` | 1920×1080 / editorial | 宽屏几何（无滚动、无溢出） |
| `radar-status-375-narrow.png` | 375×667 | 窄屏线性化（状态块落到页面下方） |
| `radar-status-embedding-blocked-1440.png` | 1440×900 / editorial | **核心场景**：标签队列已清空、Embedding 积压 170 → 生成仍被挡住 |
| `radar-status-drained-1440.png` | 1440×900 / editorial | 双队列均清空 → 生成可用、缺料说明消失 |
| `radar-status-picker-1440.png` | 1440×900 / editorial | 板块勾选清单展开态（勾选 = 参与生成，默认 12/12 全选） |

## 二、判定条目

### A-1 状态块常驻右栏底部且内容区独立滚动【pass】
`radar-status-1440-editorial.png`：1440×900 下状态块 bottom=881 == 右栏 rail bottom=881，宽 317px 沿用右栏列宽；`scrollWidth 1440 == innerWidth 1440`、`scrollHeight 900 == innerHeight 900`（无横向/纵向溢出）。内容区（今日该读 / 今天可能升温）保留独立滚动（`.rd-rail-scroll` 可视 474px / 内容 562px），状态块不随之滚动。

### A-2 状态块展示五项事实（双队列 + 两个时刻 + 最新落点 + 当日状态）【pass】
实页 DOM 实测：标签队列「待打标 25」、Embedding 队列「待向量化 2」；生成窗口「生成 21:00 · 兜底 23:30」（两个时刻均显式）；最新日报「2026-10-30」；今日状态「今日日报已出（11/12 板块）」。后端核对：`GET /api/poll` 的 `schedulers[daily_report].schedule_time=21:00`、`schedule_deadline=23:30`，与页面一致。

### A-3 两个队列两行并列、量纲分别标注【pass】
`radar-status-1440-editorial.png`：两条独立事实行（`radar-status-queue` / `radar-status-embedding-queue`）+ 两条独立进度条——标签队列「今日完成 1278 / 1279」（文章），Embedding 队列「今日完成 2936 / 2937 个标签」（标签）。两者的分母与单位分别标注，SHALL NOT 混为同一量纲。

### A-4 标签队列清空但 Embedding 队列有积压：生成仍被挡住【pass】
`radar-status-embedding-blocked-1440.png`（网络 mock：标签队列 0 积压、Embedding 170 积压，未动数据库）：标签队列行显示「队列已清空」、Embedding 队列行显示「待向量化 170」；生成按钮 `disabled=true`；缺料说明**只**指向 Embedding：「Embedding 队列有 170 个标签待向量化：泳道分桶会退化，文章可能落进「未归位」桶。」

此条是本轮增量的核心：改前状态块只盯标签队列，此时会误显示「队列已清空 + 生成可用」，而定时任务按双队列判据其实**不会**出日报。

### A-5 队列有积压时生成按钮禁用并给出缺料说明【pass】
`radar-status-1440-editorial.png`：真实库存在积压时按钮 `disabled=true`；缺料说明**按队列分列**——标签队列一条（"手动生成不等队列，当日才完成打标的文章不会进入当日日报"）、Embedding 队列一条（"泳道分桶依赖标签向量…未归位桶"），两个原因不混成一句。

### A-6 双队列均清空后生成可用、缺料说明消失【pass】
`radar-status-drained-1440.png`（网络 mock：两个队列均 0 积压，未动数据库）：两行均显示「队列已清空」；`radar-status-shortage` 元素不存在；按钮 `disabled=false`，文案「重新生成今日日报」（今日日报已出时的重生成语义）。

### A-7 暗色主题下状态块可读且无硬编码色【pass】
`radar-status-1440-dark.png`：`data-theme=dark` 生效，状态块底色为 `color(srgb 0.941176 0.541176 0.294118 / 0.0510588)`（由 `--color-accent` 派生的 Layer 2 token 组合），文字 `rgba(255, 255, 255, 0.9)`，非硬编码色值。

### A-8 1920×1080 宽屏无滚动无溢出【pass】
`radar-status-1920-editorial.png`：`scrollHeight 1080 == innerHeight 1080`、`scrollWidth 1920 == innerWidth 1920`；状态块 bottom=1061，仍固定在右栏底部；右栏内容区 603/603 **全露**（「今天可能升温」无需滚动即完整可见）。

### A-9 375×667 窄屏不破版且状态块落到页面下方【pass】
`radar-status-375-narrow.png`：`scrollWidth 375 == innerWidth 375`（无横向溢出）；状态块 top=1915、left=14、宽 346（窄屏线性化，落在页面流内而非右栏）；清单由 `flex-direction: column` 保证单列。

### A-10 板块勾选语义为「勾选 = 参与生成」且默认全选【pass】
`radar-status-picker-1440.png`：点「立即生成」后就地展开 `[data-testid=radar-status-picker]`（无 Dialog 元素、URL 不变）；初始「参与 12 / 12」，12 个 checkbox 全部 `checked=true`；取消 2 个后变「参与 10 / 12」且这两个 `checked=false`。

### A-11 全不选时确认按钮禁用、全选可恢复【pass】
`radar-status-picker-1440.png`：点「全不选」→「参与 0 / 12」且 `[data-testid=radar-status-confirm].disabled === true`；点「全选」→「参与 12 / 12」且 `disabled === false`。

### A-12 合并端点下发 Embedding 队列计数且与标签队列互不混淆【pass】
`GET /api/poll` 实测：`data` 的键为 `embedding_queue, notifications, schedulers, tag_queue`；`data.tag_queue` 与 `data.embedding_queue` 为**两个独立字段**（后者带 `completed_today`，前者不带——字段集不同也证明不是同一份数据换名）。实页读到的行值与该响应逐字段一致。

### A-13 mock 撤除后回到真实环境【pass】
`network unroute` + reload 后复核：显示真实值（「标签队列：队列已清空」/「Embedding 队列：队列已清空」，与验收时 `GET /api/poll` 的真实响应一致——真实队列会随抓取/打标实时变动，故以「与同刻 API 响应逐字段一致」为准）；验收过程未改动数据库（mock 仅改浏览器内的响应体）。

### A-14 实际点击「确认生成」触发真实日报生成【未覆盖】
未发起真实生成：该动作会跑真实 LLM 生成管线并整份替换当日日报（破坏性），超出只读验收边界。请求参数正确性由组件单测覆盖（`front/app/features/tags/components/console/BoardRadarStatusBlock.test.ts` 断言请求体只含勾选板块），集合作用域/执行闸/终态广播/终态通知由后端白盒用例覆盖（`backend-go/internal/topicgraph/handler/daily_report_handler_test.go`）。

### A-15 「今日已出」重生成态（按钮文案「重新生成今日日报」+ 替换警示 + 「确认替换」）【未覆盖】
真实库当日（2026-10-30）已出日报，但「清单内替换警示」与「确认替换」文案需展开清单才能看到，而双队列实况存在积压 → 按钮禁用 → 清单不可达（该状态由组件单测覆盖，三例：今日已出→文案、替换警示、确认按钮文案）。

### A-16 三态终态计数（成功 / 真失败 / 当日无内容）实页呈现【未覆盖】
依赖真实生成失败或当日无内容的实页场景，本次未构造；由后端白盒用例（`backend-go/internal/topicgraph/handler/daily_report_handler_test.go` 三态恒等式）与前端 composable 用例（`front/app/features/tags/composables/useRadarPipelineStatus.test.ts` 三态直读 + 旧后端回落）覆盖。

### A-17 其余路由矩阵（`/` `/search` `/discovery` `/settings` `/tags/:boardId` `/tags/:boardId/:date` `/tags/:boardId/research`）【未覆盖】
本 change 未修改这些路由的任何文件：`AppHeaderShell`/`AppHeaderView`/`AppPageShell` 未动，`BoardRadarView.vue` 与 `BoardConsoleShell.vue` 无共享代码路径。新增文件仅为 `front/app/features/tags/` 下两个 feature 私有文件、`front/app/composables/useEmbeddingQueueProgress.ts`、`front/app/utils/todayKey.ts`。此为显式判定（未覆盖＝本 change 范围外），非遗漏。

## 三、路由矩阵（layout.md §UI 复审的全系统与产品任务覆盖）

| 路由 | 判定 | 依据 |
| --- | --- | --- |
| `/tags` | **覆盖**（桌面 1440×900 editorial + dark，见 §二） | 本 change 的目标路由 |
| `/` `/search` `/discovery` `/settings` `/tags/:boardId` `/tags/:boardId/:date` `/tags/:boardId/research` | **未覆盖** | 见 A-17（显式判定：本 change 未改动这些路由） |

**产品任务走查（≥3 条）**：

1. **判断今天日报为什么还没出** → `/tags` 右栏底部状态块直接给出：两个队列各自的积压、生成窗口 21:00/兜底 23:30、今日状态、**分队列的缺料原因**（标签队列没打标 / Embedding 没向量化）。**通过**（A-2、A-3、A-5）。
2. **区分「日报没出」是队列没跑完还是真出不来** → 标签队列清空但 Embedding 积压时，状态块明确指出「Embedding 队列仍有 170 个标签待向量化…会落进未归位桶」，并挡住手动生成（避免产出降级日报）。**通过**（A-4）。
3. **在明确知道后果的前提下手动补生成** → 双队列均清空后点「立即生成」→ 展开板块清单 → 默认全选 12 个 → 取消勾选不参与的板块 → 「确认生成」。**通过**（A-6、A-10、A-11）。

## 四、右栏密度折损（已知、已缓解）

双队列使状态块从约 235px 长到 **265px**（1440×900），右栏内容区可视高度相应从约 538px 降到 **474px**，隐藏量从约 24px 增到 **88px**——即「今天可能升温」块在 1440×900 下需滚动才能看全（1920×1080 下 603/603 全露，不受影响）。

- **已缓解**：缺料说明的 Embedding 句改用更短措辞（少一行折行，块高从 339px 降到 265px）。
- **未完全消除**：两个队列各带一条进度条占约 56px。若日后认为密度仍不够，可把进度计数并入行内（`待打标 1 · 今日 1278/1279`）省掉两条进度条——此为视觉取舍，未擅自改。

## 五、遗留观察（不阻断，供后续 change）

1. **定时路径 WS 终态消息与手动路径不同型**：定时发 `daily_report_complete`（无三态计数），手动发 `daily_report_done`（带三态）。全仓无 `daily_report_complete` 消费方，未统一（design D12 已记录）。
2. **定时路径收集阶段失败时不广播 WS 终态消息**（早退在广播之前），手动路径会广播。
3. `notification.DailyReportSuccess` / `DailyReportFailedSummary` 现已无生产调用方，为待清理死代码。
4. 主 spec `notification-center` 与 `docs/reference/{database,api}` 里仍有旧的「保存 42 条条目」示例文案，与新口径漂移，建议后续单独开 delta 同步。
5. **Embedding 队列计数无 WS 事件**：状态块靠 30s 空闲轮询感知队列变化（与标签队列同款），队列清空到按钮可用的最坏延迟约一个轮询周期。
