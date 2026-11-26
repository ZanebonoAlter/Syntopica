# Design: 版块升级/接口两族合并

## D1 为什么按「管线 vs 接口」切，不按「都跟版块有关」切

6 个 spec 的共同对象都是版块，但改动连带关系只有两组：

| 组 | 内部连带 | 跨组连带 |
|---|---|---|
| 升级管线（upgrade / expand / time-window） | 强：候选收集 → 时间窗过滤 → 预聚类 → LLM 建议 → 用户确认 是同一条链，改任一段都要理解上下游 | 弱：升级产出的建议通过 API 暴露，但算法改动不改变接口形状 |
| 对外接口（management-api / article-api / editing-ui） | 强：DTO 字段一改，三个文件都要动（后端 handler、前端类型、对话框展示） | 弱：接口改动不影响升级判定逻辑 |

判据落到操作上：**调升级阈值只开一个文件，改接口字段只开一个文件**。

## D2 board-upgrade 就地扩容（不新建）

名字本来就该是 `board-upgrade`——它覆盖的就是「升级」这件事。`board-upgrade-expand` 是它的扩充方向分支，`upgrade-candidate-time-window` 是它的候选过滤参数。三者合成一个 `board-upgrade` 是还原，不是改名。

代价同上一族 D8：原有 16 条留在原位，新增 7 条追加末尾，排序不如新族整齐。接受。

## D3 board-api 命名与边界

不叫 `board-management-api`（那会让人以为只覆盖管理端点），而叫 `board-api`——它覆盖三类对外契约：

1. **版块 CRUD 与配置**（management-api）
2. **版块文章列表**（article-api，含归档口径、AI 判定、按日计数）
3. **前端交互契约**（editing-ui：对话框字段、direction_mismatch 默认隐藏、MatchDetailPanel 展示）

`board-editing-ui` 并进来而非单立，因为它只有 71 行 4 条，其中 2 条本身就是 API 类型契约。单立一个 71 行的族违反「族要能独立回答一个问题」的标准。

## D4 排序与分类

沿用已验证的两段式：business 在前（保持来源顺序）、impl 在后（标 `<!-- kind: impl -->`）。

**预判的 impl 候选**（review 重点看）：

- `API type updates`（editing-ui）——前端类型同步，属施工
- `MatchTopicTag 加载方向校验数据`类条目若有——数据加载方式
- `updateBoard refreshes embedding on description change` → business（改描述必须刷新向量，是正确性契约，不是实现选择）

## D5 归档机制

1. `.openspec.yaml` 声明 `retire_capabilities: true`
2. 归档前摘掉 4 个被退役 spec 的 `<!-- domain: -->`
3. 归档后补 marker：`board-upgrade` 保留原 marker `semantic-board`；`board-api` → `semantic-board, api`（management-api 与 article-api 原本都带 `api` 域，editing-ui 带 `ui` 域——J 段上限 3，取舍见 D6）

## D6 board-api 的 marker 取舍

成员并集：`semantic-board, api, ui`（3 个，正好在 J 段上限内）。

保留全部三个：`board-api` 改接口字段时，前端（ui）与 API 契约（api）都要同步，这两个次域是「改这族必须读哪些」的真实来源，不是实现手段。这是本仓第一次把 `ui` 与 `api` 同时挂上，原因是这族确实横跨前后端边界。

## D7 已知损失

指向 4 个被删 capability 的 archived delta 落 `domain-history.sh` 第 6 段；只碰过它们的 change 会从反查消失（与既往两族同一损失模式，别名表仍待做）。

`board-concept-management` 不参与本次合并——它有并发会话未提交的改动，且内容全 DEPRECATED，退役应独立处理。
