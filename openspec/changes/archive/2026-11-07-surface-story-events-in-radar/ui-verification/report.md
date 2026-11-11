# UI 验收报告 — surface-story-events-in-radar

> 环境：静态托管 `http://127.0.0.1:5100/`（同源），真库已回填（`board_stories` 910 条活跃）。
> 浏览器：`agent-browser` 限核 `taskset -c 2,3` 串行操作；结束后 chromium 进程归零。
> 后端已带新代码上线（overview 响应含 `stories` 段）；前端已重建部署。
> 本报告覆盖**初版实现**与 **review 修复后**两轮观测，差异处均已注明。

## 验收条目（归档对账格式）

### A-1「今日该读」条目单位为事件线【pass】
依据：右栏实测文本 = 「今日该读 / READING LIST / 俄以总统令临时接管日本烟草在俄资产 / 俄乌冲突 / 2 个来源 · 热度 1.3 · 截至 11/06 / 特朗普以百分百支持为AI发展定调 / …」，条目为事件线标题 + 版块名 + 来源数 + 热度 + 该版块锚点日，**不再**是版块日报标题。截图 `radar-rail.png`。

### A-2 跨版块全局 Top N 且排序按热度【pass】
依据：API `stories.top_stories` = 俄乌冲突 1.292 / 美国新闻 0.892 / 中国国内新闻 0.890 / 俄乌冲突 0.885 / 俄乌冲突 0.885，跨版块混排；前端渲染与 API **逐条一致**（heat desc → participants desc → story_id asc，排序前不四舍五入，展示保留一位小数）。

### A-3「今天可能升温」条目为事件线级、倍率挂在事件线上【pass】
依据：当前真库数据下升温清单为空（「暂未触发升温条件。」），版块名不再承载倍率；报头计数改用服务端 `warming_count`（=0），页头显示「10 个板块值得关注，暂未出现升温」。截图 `radar-events.png`。

### A-4「今日新起」独立成档、不显示倍率【pass】
依据：右栏渲染「今日新起」独立区块；条目文本实测 = 「俄以总统令临时接管日本烟草在俄资产 / 俄乌冲突 · 2 个来源 · 热度 1.3 · 截至 11/06」，**含来源数与热度**（R-1 修复后）、**不含任何倍率数字**（正则 `/倍率|×/` 命中 0）。截图 `radar-390.png`。

### A-5 未回填版块诚实降级【pass（R-2 修复后）】
依据：**修复前**初版把 3 个**实际已回填**的版块（影视作品 22 条事件线 / 游戏版块 49 条 / 卫生健康 3 条）误判为「未回填」并整板剔除，界面对用户显示假话「3 个版块事件身份未回填」。**修复后**：`unbackfilled_board_ids` = `[]`（count=0），三板 `story_identity_state` 均为 `backfilled`，`no_story_board_ids` = `[]`；右栏不再出现未回填提示。`new_count` 由 120 升至 **138**（更多事件线进入候选池）。

### A-6 滞后版块不被整板剔除、且显示自己的日期【pass】
依据：API `lagging_board_ids` = `[3122, 246030]`（刚果疫情新闻 11-05、卫生健康 11-05，全局 R=11-06）；两板仍参与排序，条目 `anchor_date` 分别为 2026-11-05。

### A-7 排序不完整明示【pass（单测覆盖，未取得降级实测样本）】
依据：真库当前 `partial=false`、`degraded_board_ids=[]`，**未造出降级样本**；该行为由隔离 PG 用例覆盖（`TestRadarOverviewStoryFeedPartialOnStoryComputationFailure`：注入单版块计算失败 → 断言 `partial=true` 且该板 id 进 `degraded_board_ids`）。**未取得实测样本，不声称实测通过。**

### A-8 点击事件线 → 日报正文定位（`?story=` 深链）【pass】
依据：点击右栏条目 → URL 变为 `/tags/2165/2026-11-06?story=5017`；直达该 URL 后实测 `document.activeElement` 的 `data-story-id` = **5017**（目标线程已获得焦点）、`.drm-reader` 滚动容器 `scrollTop` = **2884**（内容高 4173，已滚到目标区）。截图 `report-located.png`。
注：高亮类 `drm-thread--lit` 为 **2 秒瞬时**，验收须用「焦点 + 滚动位置」判定，**不可**在页面稳定后查高亮类（会误判为失败）。

### A-9 找不到目标不跳假位置【pass（单测覆盖）】
依据：spec Scenario「深链参数定位与失去目标」；`DailyReportReader.test.ts` 断言目标缺失时给提示、不滚动。

### A-10 双视口无溢出、无 NaN/undefined【pass】
依据：1440 与 1920 两档 `scrollWidth == innerWidth`；页面文本无 `NaN` / `undefined` 命中。

### A-11 旧响应回退分支【pass（单测覆盖）】
依据：响应缺 `stories` 段时前端回退版块级日报列表（`useBoardRadarOverview.test.ts` 覆盖）。

### A-12 窄屏 390×844【pass（R-7 补测）】
依据：视口 390×844 下 `document.documentElement.scrollWidth - innerWidth` = **0**；8 个事件线条目**无一被裁切**（`clipped_items` 空）；meta 行文本横向溢出 0 处；标题正常换行；「2 个版块数据滞后」提示与「今日新起」meta 完整可读。截图 `radar-390.png`。

### A-13 滞后版块提示可见【pass（R-6 修复后）】
依据：右栏渲染「2 个版块数据滞后（各条目按自己的锚点日计）。」；`lagging_board_ids` / `degraded_board_ids` 已归一并被消费（修复前只归一、无人消费）。

## 截图

| 文件 | 描述 |
|---|---|
| `radar-events.png` | 雷达总览 1440 档：报头「10 个板块值得关注，暂未出现升温」+ 板块矩阵（数据截至 11月6日） |
| `radar-rail.png` | 右栏「今日该读」事件线清单 +「今天可能升温」空态 +「今日新起」档 |
| `radar-390.png` | 390×844 窄屏：条目换行正常、无裁切、滞后提示与新起 meta 完整 |
| `report-located.png` | 日报正文定位到 `?story=5017` 的目标线程（焦点 + 滚动位置证据） |

## 环境备注

- 验收期间 `vcgencmd get_throttled` 保持 `0x50000`（历史欠压、当前未降频）；浏览器单会话串行，结束后 chromium 归零。
- 期间曾出现一次「前端渲染与 API 不一致」的观测，根因是**浏览器缓存了旧 SPA 页面**（`agent-browser open` 复用旧 tab，未带 cache-busting 参数）；加 `?z=<时间戳>` 强制重载后完全一致。**非代码缺陷**。
