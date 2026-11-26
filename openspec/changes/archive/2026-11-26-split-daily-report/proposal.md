<!-- complexity: medium -->
<!-- ui-impact: none -->
<!-- constraint-domains: daily-report, semantic-board -->

# Proposal: 日报 spec 拆分与归族（1417 行单文件 → 3 族）

## Why

`daily-report-system` 是全仓最肥的单个 spec：**1417 行 / 41 个 Requirement / 221 个 Scenario**，一个文件里同时装着数据模型、LLM 聚类裁决、API、WebSocket、前端报纸布局、定时调度、补档守卫、提示词约束、重点选题排序。改任何一件事都要在里面翻。

实测它的 41 条按功能分布如下（合计 1449 行）：

| 组 | 内容 | Requirement | 行数 |
|---|---|---:|---:|
| A 生成管线核心 | 数据模型、去重、质量筛选、聚类数/排序、LLM 分组、分段并行、编排流水线、存储、路由绑定、lane 归属、prompt 历史隔离、事实锚、标题内容化、跨天边界 | 15 | 470 |
| B 接口层 | 生成 API（异步）、WebSocket 进度、查询 API、重新分桶 API | 4 | 217 |
| C 前端展示 | 报纸布局时间线、生成进度、TagsPage Tab、全屏预览层、线索直达、刊头空态、重点依据定位 | 7 | 285 |
| D 调度与补档 | 定时任务复用、缺档补档、重建窗口守卫、推荐兼容、调度时刻可读、终态通知口径、缺口可见性、缺口异步补填、补填留档 | 9 | 260 |
| E 重点选题 | highlights 健壮性、今日重点聚簇候选、同事件聚簇先筛、线索 AI 标记排序、候选疑似占比竞争 | 5 | 159 |
| F 泳道分桶 | 泳道内主题分桶 | 1 | 58 |

## What Changes

**拆成 3 族，同时把两个散件归位**：

| 族 | 组成 | Requirement | 行数 |
|---|---|---:|---:|
| `daily-report-system`（就地扩容，保留原名） | A + E + F 留原位 + `daily-report-match-fallback`（2） | 23 | 711 |
| `daily-report-interface`（新建） | B + C + 从 D 移入 2（红线 20/35）+ `radar-pipeline-status`（5） | 18 | 787 |
| `daily-report-ops`（新建） | D 剩余 7 | 7 | 227 |
| `daily-report-system` | **保留原名**，只留 A+E+F（删 16 条搬走） | — | — |

判据：**改生成算法/选题规则 → 只开 `daily-report-system`；改端点/界面/状态块/终态口径 → 只开 `daily-report-interface`；改补档/缺档/告警 → 只开 `daily-report-ops`**。

`radar-pipeline-status` 归 interface 而非独立保留（推翻上轮判断）：上一轮拿它跟「诊断/研究」比，判「零连带」是对的；这一轮它的兄弟是「日报前端展示」，两者描述同一条链（标签队列积压 → 日报生成窗口 → 当日是否已出），并入才对。

## Impact

- **spec 数量**：150 → 150（退役 2 个 + 新建 interface/ops 2 个，`daily-report-system` 保留原名就地扩容）
- **历史追溯**：3 个文件的部分 delta 会指向改名/拆出的 capability，落 `domain-history.sh` 第 6 段
- **代码**：零改动
- **用户可见行为**：无
- **回退**：`git revert`

## 不做的事

- 不改任何 spec 正文（41 + 2 + 5 = 48 条逐字搬运）
- 不并 `data-enrichment`（684 行，认知工作台，属独立域）
- 不拆 `daily-report-system` 的 711 行（在 D5 的 1200 预警线下；若再长，拆分点是「聚类裁决」vs「选题排序」）
