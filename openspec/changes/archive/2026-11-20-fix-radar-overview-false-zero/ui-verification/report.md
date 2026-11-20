# ui-verification — fix-radar-overview-false-zero（minor 档）

> 验收方式：组件测试（真实文件路径见 tasks.md「验证」节 T5.3）+ agent-browser 真实浏览器核对（下方判定条目）。
> 视口：headless Chromium 默认视口（1440×900 档）；页面：`http://localhost:5100/tags`（静态托管，与生产访问同源）。
> 活库状态（2026-11-20 复核）：13 个活跃版块，全局锚点日 2026-11-19，其中 3 个版块滞后（台湾新闻/游戏版块锚点 11-18、刚果疫情新闻锚点 11-07）。

### A-1 滞后版块沉底到列表末尾【pass】
- 依据：agent-browser 读取页面 DOM —— 分段标题 `滞后 · 3 个版块` 存在；三行均落在列表末尾，序号 `11 / 12 / 13`；带「数据至 11/18」「数据至 11/18」「数据至 11/07」中性徽标；**不在**前排关注序列（01–08）里。
- 序号连续性：`01–08`（关注）→ 常温折叠段占 `09–10` → 滞后 `11–13`，全列表 01–13 连续不重号（DOM 实测 `ranks = [01..08, 11, 12, 13]`）。
- 截图：`radar-after-2026-11-20.png`。
- 对应契约：delta spec「滞后版块不占前排」。

### A-2 报头统计与口径未回归【pass】
- 依据：DOM 实测报头「8 个板块值得关注」、统计「活跃板块 13 · 热点 0 · 今日线索 66」。
- 交叉核对：`curl -s --noproxy '*' localhost:5100/api/semantic-boards/overview` 连续 10 次采样 —— 13 行、`data_unavailable` 全 false、`data_delayed` 全 false、10 份样本逐字段一致；API 的当日线索合计 75，页面 66，差值 9 = 3 个滞后版块自己锚点日的线索（不进今日口径，见 delta spec「滞后版块的旧日线索不进今日合计」）。
- 墙钟：`0.051–0.509s`（首调用冷启 0.509s，其余 0.05–0.06s），远低于 5s 预算 —— 改造前基线为 0.4–2.5s 且偶发 5.0–10.5s。
- 对应契约：design D6/D7 +「未取到 ≠ 0」。

### A-3 端点超时降级不再呈现为 0【未覆盖】
- 未覆盖原因：改造后端点 p95 ≈ 0.06s（13 版块），**无法在浏览器里稳定复现 5 秒预算超时**（该触发条件已被性能改造基本消除）。
- 替代证据（组件/单测层，均实跑通过）：
  - 后端 `backend-go/internal/topicgraph/repository/board_radar_repository_test.go`：`TestBoardRadarTimeoutRowIsUnknownNotZero`（断言 JSON 是 `"attention":null` 而非 `0`）、`TestBoardRadarDegradedRowIsUnknownNotZero`、`TestBoardRadarCluesDegradedRowIsUnknownNotZero`、`TestBoardRadarRealZeroRowKeepsZeroCounts`、`TestBoardRadarDateUnknownWhenAllRowsUnavailable`；handler `TestGetBoardRadarOverviewUnavailableRowWireShape`。
  - 前端 `front/app/features/tags/composables/useBoardRadarOverview.test.ts`（全部/部分未取到 → 报头不说「0 个板块值得关注」、统计为 null、未取到行不进分档、滞后行旧日线索不进今日合计）、`front/app/features/tags/components/console/BoardRadarStory.test.ts`（未取到行 →「数据未取到」+「—」；日报信息未取到 → 不说「无日报日期 / 今天暂无日报」）、`front/app/features/tags/components/console/BoardRadarView.test.ts`（未取全提示条复用既有 `.rd-error` + 既有重试按钮；业务空态不渲染；四段接号）。
