# UI 验收报告 — surface-story-heat-in-report-top

> 环境：静态托管 `http://127.0.0.1:5100/`（同源），真库已回填（`board_stories` 744 条、`stories_backfilled=true`）。
> 浏览器用 `taskset -c 2,3` 限核（本机曾因构建欠压重启两次）。

## 验收条目（归档对账格式）

### A-1 矩阵格子点名最热事件线【pass】
依据：DOM 实测 `title` = 「… 2026-11-01：2 个事件线 · 最热：战事拖累中期选举，MAGA选民出现倒戈（1 个来源） · 未归组 4 个事件 · 2 个主题桶」——「最热」段直接写出事件线标题与来源数；截图 `matrix-1440.png`、`matrix-1920.png`。

### A-2 四数字顺序未变【pass】
依据：点名段插在「最热 N 个来源」位置，`事件线数 → 最热（含点名）→ 未归组 → 主题桶` 顺序保持；截图 `matrix-1440.png`。

### A-3 图例措辞改为指向明确【pass】
依据：图例文本 = 「数字 = 当日事件线数；颜色 = **当天被报道最多的那条事件线**的来源数（0 / 1-2 / 3-5 / 6+）· '—' 无分区 · '?' 未取到」；截图 `matrix-1920.png`。

### A-4 无最热线时不追加点名片段的降级【pass】
依据：单测覆盖（`observationLanes.test.ts`「无最热线不追加」），组件层对 `hottestStoryId` 为 null 时不渲染点名段。

### A-5 超长标题截断【pass】
依据：单测覆盖 `HOTTEST_STORY_TITLE_MAX=30` 截断 + 省略号。

### A-6 双视口无横向溢出、无 NaN【pass】
依据：1440/1920 两档 `scrollWidth == innerWidth`；页面文本无 `NaN`/`undefined` 命中。

### A-7 旧报告兼容（无 story_id 时不显示来源数与「进展」）【pass】
依据：现有报告 highlights 为旧数据（无 `story_id`/`participant_count`），页面正常渲染、无占位符、无徽标；逻辑由 `highlightEvidence()` 单测覆盖（缺省→null 不显示，真实 0 则显示「0 个来源」）。

### A-8 点击定位与「进展」徽标的渲染分支【未覆盖】
原因：需**新生成**一份带 `story_id` 的日报（要跑真实 LLM 与写库）才能端到端看到徽标与跳转；本 change 明确不补造历史重点。已由前端单测覆盖分支逻辑（`dailyReportMagazine`/刊头测试），但无真实数据截图。

## 截图

| 文件 | 描述 |
|---|---|
| `matrix-1440.png` | 1440 档矩阵：格 title 点名最热事件线 + 新图例 |
| `matrix-1920.png` | 1920 档矩阵：宽屏布局 |
| `radar-1920.png` | 1920 档雷达：新版部署后回归（attention 与分档未变） |

## 环境说明

验收期间 `vcgencmd get_throttled` 保持 `0x50000`（历史欠压、当前未降频）；浏览器单会话串行操作，结束后 chromium 进程归零。
