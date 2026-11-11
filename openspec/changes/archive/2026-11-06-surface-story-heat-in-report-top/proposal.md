<!-- complexity: complex -->
<!-- ui-impact: minor -->
<!-- constraint-domains: daily-report, semantic-board -->

# Surface story heat in report top

## Why

矩阵日格数字是事件线数，颜色却取来源数最多的一条线，现有提示不点名，用户无法验证颜色。实测 board 1974 的 29 个有数据日报日中有 13 天含多条事件线，歧义不是偶发。

日报「今日重点」仍只看标签/聚类，不消费刚落地的事件线与热度，既无法解释选题，也无法追踪昨日重点或定位正文。

## What Changes

- 矩阵 timeline 增加 `hottest_story_id` / `hottest_story_title`；title 与 aria 点名同一条来源数最多的事件线。图例改为「颜色 = 当天被报道最多的那条事件线的来源数」。格值、颜色算法与点击入口不变。
- 日报先按既有 48h/24h 事件线热度筛 top 8 候选，LLM 只从候选挑 2–3 条写事实文案；`story_id` 与 `tag_ids` 并存，来源数/热度由服务端补入而非模型编造。
- 昨日重点中出现同一存活 `story_id` 时标「进展」，并明确这不是线程 `is_development` 判定。
- 点击重点定位本期正文中同事件线第一条 thread 并短暂高亮；旧数据与找不到目标时诚实降级，不新增事件线详情页。
- 调整时序：SaveReport → 有界尽力归属 → 跨期去重 → 候选 → highlights → title/summary。归属失败或无事件线回退现有标签重点路径，保持正文可用。

## Impact

- 后端：topicgraph 日报模型、生成编排、归属调用边界、读写仓储与 lane-dynamics DTO；复用既有热度证据，不改热度公式、雷达阈值（3.3/0.7）或分区生成算法。
- 前端：观察矩阵提示/图例、日报刊头徽标和来源数字、正文锚点及滚动高亮；沿用现有布局，无新路由、弹层或额外交互步骤。
- 存储：只扩展 highlights JSON 和响应字段，无新增表/列迁移。历史 highlights 不回刷。
- 依赖：上游 `2026-11-06-add-event-story-heat`；主契约 `daily-report-system`、`workshop-board-console`。
- 性能：夜间生成增加最多 60s 归属等待预算；保持单条 highlights 调用路径（既有非法响应重试除外），不做异步二次改标题。

## 部署后影响与需要的操作

本任务仅产出待评审制品，当前运行行为不变，无部署或数据操作。后续实现部署后，新日报重点可见来源依据、昨日延续标记与正文定位，矩阵提示可读到事件线标题。无需用户清理/迁移/重新生成数据；旧日报照常显示标题理由，不伪造事件身份、来源数字或进展，若想获得新重点仅按既有允许重建窗口手动重建，禁止超窗重建。
