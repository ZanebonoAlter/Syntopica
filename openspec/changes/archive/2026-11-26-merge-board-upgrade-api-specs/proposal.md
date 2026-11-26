<!-- complexity: simple -->
<!-- ui-impact: none -->
<!-- constraint-domains: semantic-board, api -->

# Proposal: 版块升级/接口两族 spec 合并（6 → 2）

## Why

`semantic-board` 域里 6 个 spec 共 869 行组成一条完整的「升级建议 → 用户确认 → 接口暴露」链路，但被切成 6 个文件，且混着两种性质：

| spec | 行数 | 性质 |
|---|---:|---|
| board-upgrade | 279 | 升级建议**管线**（触发/预聚类/LLM 建议/用户确认/冷启动） |
| board-upgrade-expand | 90 | 扩充方向（锁定单版块 + 双路召回 + 二分类） |
| upgrade-candidate-time-window | 70 | 候选**时间窗过滤** |
| board-management-api | 211 | 版块 **CRUD + 升级建议 API + 回填 + 匹配参数** |
| board-article-api | 148 | 版块**文章列表 API**（筛选/分页/归档口径/AI 判定） |
| board-editing-ui | 71 | **前端对话框** + direction_mismatch 展示控制 |

改一个「升级建议」功能要同时开 upgrade / expand / time-window / management-api 四个文件；改一个「版块编辑对话框」要开 editing-ui 和 article-api 两个。而升级算法与 CRUD 接口之间几乎没有连带——它们是两层。

## What Changes

**6 → 2**，按「业务管线」vs「对外接口」切：

| 族 | 吸收 | 行数 | Requirement |
|---|---|---|---:|
| `board-upgrade`（就地扩容） | board-upgrade-expand + upgrade-candidate-time-window | 439 | 23 |
| `board-api` | board-management-api + board-article-api + board-editing-ui | 430 | 25 |

判据：**改升级阈值/冷却期/召回策略 → 只碰 `board-upgrade`；改接口字段/分页/前端交互 → 只碰 `board-api`**。

`board-editing-ui` 归 `board-api` 而非单立：它 4 条里 2 条是 API 类型契约（`API type updates`、`MatchDetailPanel direction check display`），2 条是对话框交互——都是「对外接口长什么样」，只是消费端一个是前端一个是第三方。

## Impact

- **spec 数量**：156 → 152（退役 **5** 个 + 新建 1 个；`board-upgrade` 保留原名就地扩容）
- **历史追溯**：指向 5 个被删 capability 的 archived delta 落 `domain-history.sh` 第 6 段
- **代码**：零改动
- **用户可见行为**：无
- **回退**：`git revert`

## 不做的事

- **不动 `board-concept-management`**（40 行全 DEPRECATED）：该 spec 有并发会话未提交的改动（marker 规范化），退役它会覆盖他人工作。它已被 `tagging` + `board-api` + `board-upgrade` 完整替代，退役应作为独立小 change 处理
- 不合泳道/画布、话题、日报族
- 不改 spec 内容（重复/过时条款记入 review.md）
