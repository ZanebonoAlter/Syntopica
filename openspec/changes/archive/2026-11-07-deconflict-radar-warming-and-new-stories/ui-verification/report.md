# UI 验收报告 — deconflict-radar-warming-and-new-stories

验收日期：2026-11-07 ｜ 方式：agent-browser 机械断言（`eval --stdin` 跑 JS 断言，不靠目测截图）｜ 复测（review R-1 修复后）：2026-11-07，见「复测（R-1 修复后）」节

## 环境

- 静态托管地址：`http://127.0.0.1:5100/tags?z=1794058422`（带 cache-busting query；`backend-go/frontend/` 为已部署新版）
- 真接口实况（`curl -s http://127.0.0.1:5100/api/semantic-boards/overview`，2026-11-07 实测）：

  ```
  top_stories story_id: [5017, 5026, 5056, 5015, 5014]   （集合与 brief 一致）
  new_stories story_id: [5052, 5110, 5074]               （集合与 brief 一致）
  交集: 空（三档互斥成立）
  cross_day_story_count: 0
  new_count: 133
  warming_count: 0
  ```

- 真接口实况（R-1 修复后复测，2026-11-07 22:19 实测，`curl -s --noproxy '*' http://127.0.0.1:5100/api/semantic-boards/overview`，后端已重新构建部署）：

  ```
  new_total_count: 138        （R-1 新增字段已下发：当日有证据 ∧ 无基线的全量，去 Top 前、截断前）
  new_count: 133              （去 Top 后的新起全量）
  len(new_stories): 3         （story_id: 5052 / 5110 / 5074）
  cross_day_story_count: 0
  warming_count: 0
  top_stories story_id: [5017, 5026, 5056, 5015, 5014]  → top ∩ new = ∅
  → 「今日新起」档预期渲染：3 条条目 + 摘要行「另有 130 条今日新起事件线（未在今日该读中列出）。」（130 = new_count 133 − 列出 3，实测见 B-1）
  ```

- 真库佐证（只读 SELECT，包 `BEGIN; … ROLLBACK;`，容器 syntopica-postgres）：`board_stories` 共 910 条，其中 `first_seen_date < last_seen_date` 0 条 → 与 `cross_day_story_count: 0` 相符（接口口径 `HasBaseline && HasTodayEvidence` 比该 SQL 更严，当前数据下两者同为 0；只数基线会错报的教训见 tasks 1.3 测试内注释）。
- 浏览器纪律：agent-browser 命名会话 `radar-accept-6d5795ec8cb0`，单会话串行，**全部命令 `taskset -c 2,3` 限核**；结束执行 `agent-browser close`，`pgrep -cx chromium` = 0（归零确认；期间 dev-process-guard 一次预警所指 5 进程即本会话浏览器栈，已随 close 全部清理）。
- 视口：1440×900（主断言）→ 1920×1080 → 390×844（A-7 逐档复检）。

## 复测（R-1 修复后）

- 背景：review R-1（High）修复后新增字段 `new_total_count`，后端已重新构建部署、静态产物已重铺；「另有 N 条」摘要行从死代码变为可达，需实测。
- 环境：静态托管 `http://127.0.0.1:5100/tags?z=1794061117`（cache-busting query）；agent-browser 命名会话 `radar-retest-r1`，单会话串行，全部命令 `taskset -c 2,3` 限核，结束 `agent-browser close` 后 `pgrep -cx chromium` = 0（归零确认；期间 dev-process-guard 一次预警所指 5 进程即本会话浏览器栈，已随 close 全部清理）。视口：1440×900（主断言）→ 1920×1080 → 390×844。
- 真接口实况见「环境」节复测块（138 / 133 / 3 / 0 / 0 / top∩new=∅）。

### B-1 摘要行出现且正确【pass】

判定依据（`agent-browser eval --stdin`，1440×900）：

```json
"B1_exists": true,
"B1_text": "另有 130 条今日新起事件线（未在今日该读中列出）。",
"B1_has130": true,
"B1_hasNotListed": true
```

`[data-testid="radar-new-summary"]` 存在，文案含「另有 130 条今日新起事件线」与「未在今日该读中列出」，计数与接口实况自洽：`new_count`(133) − `len(new_stories)`(3) = 130。

### B-2 新起条目仍在【pass】

判定依据：

```json
"B2_ids": { "5052": true, "5074": true, "5110": true },
"B2_allExist": true,
"B2_entryCount": 3
```

摘要行不取代条目：5052 / 5074 / 5110 三条 `[data-testid="radar-new-<id>"]` 均存在，新起条目恰 3 条（同时等价复核了 A-3/A-5 的存续）。

### B-3 三态互斥不重叠【pass】

判定依据：

```json
"B3_summary": true, "B3_covered": false, "B3_noNew": false,
"B3_statesPresent": 1,
"B3_exclusive": true
```

同一时刻仅「另有 N 条」一态渲染，`radar-new-covered` 与 `radar-no-new` 均不出现（当前实况走截断态分支）。

### B-4 互斥仍成立（新起∩该读=空）【pass】

判定依据（DOM 提取 testid story_id 集合）：

```json
"B4_new_ids": ["5052", "5110", "5074"],
"B4_top_ids": ["5017", "5026", "5056", "5015", "5014"],
"B4_intersection": [],
"B4_exclusive": true
```

与接口层实况（top∩new=∅）互相印证，等价复核了 A-4。

### B-5 升温空态文案仍在【pass】

判定依据（`[data-testid="radar-no-warm"]`.innerText）：

```json
"B5_text": "当前没有跨天延续的事件线。 跨天延续能力尚在建设中。",
"B5_hasNoCrossDay": true,
"B5_hasBuilding": true,
"B5_notHasNotTriggered": true
```

含「当前没有跨天延续的事件线」与「建设中」，不含「暂未触发升温条件」，与 `cross_day_story_count: 0` 的 no-cross-day 分支一致，等价复核了 A-2。

### B-6 无 NaN/undefined【pass】

判定依据：`document.body.innerText` 含 `NaN` = false，含 `undefined` = false。

### B-7 三视口无横向溢出【pass】

判定依据（每档 `documentElement.scrollWidth - innerWidth` + 摘要行裁切检查）：

```json
1440×900:  { "diff": 0 }
1920×1080: { "diff": 0, "sum_clipped": false, "sum_whiteSpace": "normal", "sum_rectInViewport": true, "entry_inViewport": true }
390×844:   { "diff": 0, "sum_clipped": false, "sum_whiteSpace": "normal", "sum_rectInViewport": true, "entry_inViewport": true }
```

三视口横向溢出差均 0（≤2 达标）；390 档摘要行 clientWidth=361 = scrollWidth=361（无裁切），`whiteSpace: normal` 可正常换行——当前文案在 361px 内单行放下，若文案变长自然折行而非裁切。

### B-8 「N 条均已列入」与「今天没有新起」两态【未覆盖：单测覆盖，未取得实测样本】

真库当前实况为 `new_total_count=138 ∧ new_count=133 ∧ len(new_stories)=3`（走 B-1 截断态分支），「全被覆盖」与「没有新起」两态**造不出**，未取得实测样本。单测清单（命令+期望，本次复测实跑）：

- `front/app/features/tags/components/console/BoardRadarView.test.ts`：:418「新起全被该读覆盖」断言 `radar-new-covered` 文案「4 条今日新起事件线均已列入「今日该读」」且另两态不出现；:428「当日没有新起」断言 `radar-no-new` 文案「今天没有新起的事件线」；:436-446 三态互斥单测断言；:451 旧响应缺 `new_total_count` 降级（照列条目不出摘要）。
- 后端三联口径：`backend-go/internal/topicgraph/repository/board_radar_stories_test.go` 的 `TestRadarStoryFeedNewTotalCount`（截断态 10/5/3、全被覆盖态 3/0/0、无新起态 0/0/0）与 `TestRadarStoryFeedNewStoriesSummaryCountWhenFullyCovered`（断言 `new_total_count > 0` 而 `new_count == 0` 两态可区分）。
- 命令+期望：`cd front && pnpm test:unit app/features/tags/components/console/BoardRadarView.test.ts --maxWorkers=2` → 本次实跑 `Test Files 1 passed (1) ｜ Tests 42 passed (42)`；后端 `cd backend-go && go test ./internal/topicgraph/repository/ -run 'RadarStoryFeedNewTotalCount|RadarStoryFeedNewStoriesSummary' -count=1` → ok（6b.2 修复时实跑绿，红绿证据见 review.md R-1 处置）。

### 与 A 条的取代/复核关系

- **A-8【未覆盖】已被 B-1 / B-8 取代**：A-8 记录的「新起全被该读覆盖时给『另有 N 条』摘要」是 R-1 判定的死代码行为，修复后契约已改——全被覆盖态改用 `radar-new-covered`（「N 条均已列入」），「另有 N 条」摘要归属截断态；A-8 所引旧用例（`newStories:[] ∧ newCount:133` 不可能载荷）已删除。摘要行本身现为 B-1 **实测通过**；「全被覆盖」与「没有新起」两态仍未实测，由 B-8 记录（单测清单+命令）。
- A-1～A-7 结论在复测中均复核仍成立：B-2 等价复核 A-3/A-5，B-4 等价复核 A-4，B-5 等价复核 A-2，B-6 复核 A-6，B-7 复核 A-7；A-1 的三档结构分离由 B-3/B-5（两档 testid 同屏共存且互不包含）与 B-4（集合级分离）间接复核，未逐字重跑结构断言。

## 机械断言（A-1～A-8）

### A-1 三档结构分离【pass】

判定依据（`agent-browser eval --stdin`）：

```json
"A1_newBlock_exists": true,
"A1_newBlock_tag": "SECTION",
"A1_noWarm_count": 1,
"A1_new_not_inside_warmSection": true,
"A1_warmSection_class": "rd-rail-block rd-warm-block",
"A1_newBlock_class": "rd-rail-block rd-new-block",
"A1_sibling": true
```

`[data-testid="radar-new-block"]` 存在且本身是 `section.rd-rail-block.rd-new-block`；与 `[data-testid="radar-no-warm"]` 所在 `section.rd-rail-block.rd-warm-block` 为同级兄弟（`parentElement` 相同），**不是**其后代——「今日新起」已从「今天可能升温」卡片内部提出为同级独立分档。

### A-2 升温空态文案【pass】

判定依据（`[data-testid="radar-no-warm"]`.innerText，storyMode 分支）：

```json
"A2_text": "当前没有跨天延续的事件线。\n跨天延续能力尚在建设中。",
"A2_has_noCrossDay": true,
"A2_has_building": true,
"A2_not_has_notTriggered": true
```

含「当前没有跨天延续的事件线」与「建设中」，不含「暂未触发升温条件」——与真接口 `cross_day_story_count: 0` 的 `no-cross-day` 成因分支一致。

### A-3 新起档内容【pass】

判定依据（三条新起条目逐一断言，样例为 5052）：

```json
{ "id": 5052, "exists": true, "has_geLaiYuan": true, "has_reDu": true, "has_jieZhi": true, "no_x": true, "no_bei": true,
  "sample": "国常会部署完善资本市场稳市机制\n中国国内新闻 · 1 个来源 · 热度 0.9 · 截至 11/06" }
```

5052 / 5074 / 5110 三条 `[data-testid="radar-new-<id>"]` 均存在，meta 行含「个来源」「热度」「截至」，不含「×」与「倍」——新起档不显示倍率。

### A-4 三档互斥（DOM 提取）【pass】

判定依据（从 DOM 提取 testid 的 story_id 集合，新起侧按数字 id 口径过滤）：

```json
"A4_top_ids": ["5017", "5026", "5056", "5015", "5014"],
"A4_new_ids": ["5052", "5110", "5074"],
"A4_intersection": [],
"A4_exclusive": true
```

「今日新起」与「今日该读」story_id 集合无交集，与接口层核对结果一致。

### A-5 该读条目数【pass】

判定依据：`document.querySelectorAll('[data-testid^="radar-story-"]').length` = **5**。

### A-6 无 NaN/undefined【pass】

判定依据：`document.body.innerText` 含 `NaN` = false，含 `undefined` = false。

### A-7 无横向溢出（三档视口）【pass】

判定依据（每档视口下 `documentElement.scrollWidth - innerWidth` 与 `.rd-read-meta` / `.rd-read-title` 裁切检查）：

```json
1440×900:  { "diff": 0, "noOverflow": true,  "clippedCount": 0 }
1920×1080: { "diff": 0, "noOverflow": true,  "clippedCount": 0 }
390×844:   { "diff": 0, "noOverflow": true,  "clippedCount": 0 }
```

三视口横向溢出差均为 0（≤2 达标），meta/title 行无 `scrollWidth > clientWidth + 2` 裁切。

### A-8 摘要行分支（新起全被该读覆盖）【未覆盖】

> **本条已被 B-1 / B-8 取代**（见「与 A 条的取代/复核关系」）：R-1 修复后「全被覆盖」态改用 `radar-new-covered` 文案，本节所述「另有 133 条」摘要行为死代码时代的旧行为，所引旧用例已删除。以下为首版验收时的原始记录。

真库 `new_stories` 非空（3 条），**造不出**「全被该读覆盖」的实况，未取得实测样本。该分支由单测覆盖：

- 清单：`front/app/features/tags/components/console/BoardRadarView.test.ts` 用例「新起档全部被该读覆盖时给一行摘要，不重复也不整档消失」（断言 `radar-new-summary` 存在、文案含「另有 133 条今日新起事件线」「未在今日该读中列出」）；后端口径由 `board_radar_stories_test.go` 的 `TestRadarStoryFeedNewStoriesSummaryCountWhenFullyCovered` 钉住。
- 命令+期望：`cd front && pnpm test:unit app/features/tags/components/console/BoardRadarView.test.ts --maxWorkers=2` → 39 用例全过（本次验收实跑：`Tests 78 passed (78)`，含该文件）。

## 5.3 先红后绿证据（/tmp 副本外科手术式回退，未动仓库实现代码）

回退方式：把本 change 的三个行为退回旧行为（删 top 互斥 `continue`；跨天计数退化为只数 `HasBaseline`；前端归一/派生/文案退回单一空态与无摘要行），在 `/tmp` 副本跑定向测试；随后恢复当前实现复跑。仓库工作区全程未改。

后端（`cd /tmp/rg-backend && go test ./internal/topicgraph/repository/ -run 'RadarStoryFeedNewStoriesExclude|RadarStoryFeedNewStoriesSummary|RadarStoryFeedCrossDayStoryCount|RadarStoryFeedNewCountNotAffectedByTrim' -count=1 -v`）：

```
— 旧行为（红）—
--- FAIL: TestRadarStoryFeedNewStoriesExcludeTopStories
--- FAIL: TestRadarStoryFeedNewStoriesSummaryCountWhenFullyCovered   （Should be zero, but was 4）
--- FAIL: TestRadarStoryFeedNewCountNotAffectedByTrim                （expected 3, actual 8）
--- FAIL: TestRadarStoryFeedCrossDayStoryCount                       （expected 2, actual 3：stale 旧线被只数基线口径误计入）
— 恢复当前实现（绿）—
--- PASS: TestRadarStoryFeedNewStoriesExcludeTopStories
--- PASS: TestRadarStoryFeedNewStoriesSummaryCountWhenFullyCovered
--- PASS: TestRadarStoryFeedNewCountNotAffectedByTrim
--- PASS: TestRadarStoryFeedCrossDayStoryCount
ok    syntopica-backend/internal/topicgraph/repository
```

前端（`cd /tmp/rg-front && pnpm test:unit app/api/normalizers/boardRadar.test.ts app/features/tags/composables/useBoardRadarOverview.test.ts app/features/tags/components/console/BoardRadarView.test.ts --maxWorkers=2`）：

```
— 旧行为（红）—  Test Files 3 failed (3) ｜ Tests 6 failed | 72 passed (78)
   （含 radar-new-summary 缺失、升温空态两成因缺失、crossDayStoryCount 归一缺失）
— 恢复当前实现（绿）—  Test Files 3 passed (3) ｜ Tests 78 passed (78)
```

仓库本体原位复跑（5.2 口径）：同样 3 文件 `Tests 78 passed (78)`。

## 验证命令记录（7.1～7.3）

- 7.1：`cd backend-go && go build ./...`（exit 0）`&& go vet ./...`（exit 0）`&& golangci-lint run ./internal/topicgraph/...`（0 issues）`&& gofmt -l internal/topicgraph/`（无输出）
- 7.2：`cd front && pnpm exec nuxi typecheck`（exit 0）`&& pnpm lint`（exit 0，8 个存量 warning、0 error）
- 7.3：见「环境」节真接口 5 数 + 真库 SQL 佐证；`new_count`(133) 为去 Top 后全量、`new_stories`(3 条) 为截断上限内列表，口径一致性由 1.4 `TestRadarStoryFeedNewCountNotAffectedByTrim` 钉住（红绿证据如上）。
- 2.5/5.1：`cd backend-go && go test ./internal/topicgraph/repository/ -run 'Radar' -count=1`（ok，13.4s）`&& go test ./internal/topicgraph/handler/ -run 'Radar' -count=1`（ok，27.6s）

## 截图

- `openspec/changes/deconflict-radar-warming-and-new-stories/ui-verification/radar-1440.png`（1440×900 全页，右栏三档）
- `openspec/changes/deconflict-radar-warming-and-new-stories/ui-verification/radar-390.png`（390×844 全页，窄屏）

## 归档门禁（archive-readiness.sh）

第一轮（本报告与 ui-design.md 验收映射落地前）：

```
✓ ① doc-impact verify
✓ ② check-standards
✓ ③ tasks.md 尾三节 + doc-impact 声明
✓ ④ scenario-trace 映射对账
✗ ⑤ review-trace 痕迹对账（exit 1）
    ✗ 未通过：缺 review.md 建账与 <!-- review: waived --> 豁免声明（零痕迹）
✗ ⑦ UI 验收痕迹对账（exit 1）
    ✗ 未通过：minor 档验收映射未指名道姓（验收节须映射到 组件测试 或 agent-browser/人工验证 至少其一）
✓ ⑥ human-feedback 处置对账
✗ 2 项红
```

处置：⑦ 属可修项——已建本报告（A-1～A-7 合法判定条目）并在 `ui-design.md` 补「验收」节指名组件测试与 agent-browser 实物路径；⑤ **不在本子线程辖区修**：独立 review（tasks 6b.1）由主线程另派，review.md 须待 review 结论建账，不造假豁免（`<!-- review: waived -->` 不适用——review 是计划内动作而非豁免）。

第二轮（修复后复跑，原样输出）：

```
归档就绪自查：openspec/changes/deconflict-radar-warming-and-new-stories（与 spec-gate 归档门禁同口径六项）

✓ ① doc-impact verify
✓ ② check-standards
✓ ③ tasks.md 尾三节 + doc-impact 声明
✓ ④ scenario-trace 映射对账
✗ ⑤ review-trace 痕迹对账（exit 1）
    ✗ 未通过：缺 review.md 建账与 <!-- review: waived --> 豁免声明（零痕迹）
      修复路径（二选一）：
      1. 建账：在 change 目录创建 review.md，按格式契约记录 review 意见——
             ### R-1 2026-10-12【High】<标题>
             - **状态**：待处理
         （等级词表 High/Medium/Low；状态词表 待处理/已修复/已采纳/已驳回/误报丢弃）
         审了没问题的写显式声明行：- 审毕无发现（审查范围：<说明>）
      2. 豁免：在 proposal.md 头部机器可读注释区（与 ui-impact 同层）声明
         <!-- review: waived -->（豁免理由写在 proposal 正文；waived 占比进 retro 统计防滥用）
✓ ⑦ UI 验收痕迹对账
✓ ⑥ human-feedback 处置对账

✗ 1 项红——按上方各红项输出补齐后重跑本脚本，全绿再归档（勿补一项就试一次归档）
```

⑦ 已转 ✓（本报告 + ui-design.md 验收节）；⑤ 仍 ✗，原因同上，待主线程 6b.1 review 建账后复跑转绿。

第三轮（R-1 修复后复测收尾，review 已建账、6b.1 已勾，原样输出）：

```
归档就绪自查：openspec/changes/deconflict-radar-warming-and-new-stories（与 spec-gate 归档门禁同口径六项）

✓ ① doc-impact verify
✓ ② check-standards
✓ ③ tasks.md 尾三节 + doc-impact 声明
✓ ④ scenario-trace 映射对账
✓ ⑤ review-trace 痕迹对账
✓ ⑦ UI 验收痕迹对账
✓ ⑥ human-feedback 处置对账

✓ 六项全绿，可以 openspec archive deconflict-radar-warming-and-new-stories（树上并发脏文件时门禁还会 warn 提醒，不阻断）
```

⑤ review-trace 已转绿：review.md 已建账（R-1～R-5 五条，全部「已修复」处置完毕，见 review.md）；六项全绿，无遗留红项。

## 遗留与未覆盖项

1. **B-8**（原 A-8，已被取代）：「N 条均已列入」与「今天没有新起」两态未取得实测样本（真库造不出，走 B-1 截断态分支），单测覆盖（清单与命令见 B-8 节）。
2. **门禁 ⑤ review-trace**：review 已建账（review.md，5 条全部处置完毕），待复跑归档门禁确认（见「归档门禁」节第三轮）。
3. chromium 归零确认：复测会话 `radar-retest-r1` 执行 `agent-browser close` 后 `pgrep -cx chromium` = 0。
