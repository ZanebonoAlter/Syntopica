# UI 验证报告：关键事件时间视野「其他」兜底泳道

- 变更：`add-key-event-other-lane`（ui-impact: minor）
- 环境：本机静态托管 `http://localhost:5100`（`bash scripts/dev/deploy-frontend.sh` 构建铺盘 + 后端重启），agent-browser 无头会话
- 视口：1280×633（minor 档不强制双视口；布局沿用现有 workspace 主舞台）

### A-1 未关注 candidate 的关键事件在「其他」行显形 【pass】

- 页面：`/tags/3030?view=events`（版块「开发者工具与开源生态」，14 天窗，选中日 2026-11-24）
- 现象：休眠组之后出现「其他」行；行头 `其他 / 3 事件 · 最近 11/24 / 当日 1 条`；行内 11-18 两条标签（「腾讯混元开源Hy4 flash三版本模型」「双模型分工：Astra 攻专业场景，Sol 主打日常性价比」）+ 11-24 日聚焦卡片（「GPT-6.2 Sol上线ChatGPT，主打日常对话优化」，含关联文章 1 篇可展开）。
- 对照数据：`topic_key_event_markers` 中 board 3030 的 confirmed 事件里有 3 条挂在未关注 candidate 话题（11-18×2、11-24×1），此前视图一条都不显示。
- 依据：`agent-browser get count '[data-testid="key-event-row-__other__"]'` → `1`；`get text '[data-testid="key-event-rowhead-__other__"]'` → 上述行头三行；截图 `ui-verification/other-lane-3030-14d.png`。

### A-2 视图头当日读数含兜底行 【pass】

- 现象：视图头显示 `11/24 · 1 条`，与该日「其他」行节点/卡片数一致（无「行里有节点、头部 0 条」）。
- 依据：`agent-browser get text '[data-testid="key-event-selected-count"]'` → `11/24 · 1 条`；同页行头 `当日 1 条`。

### A-3 兜底行位置与常显（不参与休眠折叠） 【pass】

- 现象：「其他」行位于休眠泳道组之后（DOM 顺序：`key-event-dormant-toggle` → `key-event-row-__other__`）；休眠组未展开时该行仍可见。
- 依据：同一会话中 `get count` 命中 1；页面截图中休眠组行与「其他」行的相对位置。

### A-4 无兜底事件时不占行 【未覆盖】

- 原因：本版块当前窗口内 3 条未归属事件是真实数据，无法在不改数据的条件下构造「0 条」形态；该行为由组件单测覆盖（`KeyEventTimelineView.test.ts` 用例「无兜底事件时不渲染该行」）。

### A-5 审查修复后复核（最新产物） 【pass】

- 时间：2026-11-25 01:50（`deploy-frontend.sh --no-restart` 重新构建铺盘后）。
- 现象：`/tags/3030?view=events` 行集合 = `502` / `1804` / `__other__`；「其他」行头 `其他 / 3 事件 · 最近 11/24 / 当日 1 条`；视图头 `11/24 · 1 条`；行名呈弱化色（`is-other` 样式生效，节点/标签/卡片保持满对比度）。
- 依据：agent-browser `eval` 读取 DOM（`view=true`、行列表、行头 innerText、读数 innerText）；截图 `ui-verification/other-lane-3030-14d.png`（本轮覆盖）。
