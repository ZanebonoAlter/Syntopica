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

**判据**（与既往两族一致，成文备查）：

| 类型 | 判据 | 本族实例 |
|---|---|---|
| business | 约束系统对外产出什么：数据正确性、用户可见行为、接口字段/参数/分页语义、阈值与默认值、业务不变量 | 升级建议幂等、候选时间窗语义、DTO 形状、direction_mismatch 默认隐藏 |
| impl | 约束代码怎么组织/怎么写，或记录已完成施工 | `API type updates`（前端类型同步） |

**注意**：business 条款点名的 SQL 表名 / Go 函数名（如 `Board rematch API` 里的 `MatchTopicTag`）是「在哪生效」的定位信息，不构成 impl 判据——判据看的是它约束的是**外部可观察结果**还是**内部写法**。

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

保留全部三个：`board-api` 改接口字段时，前端（ui）与 API 契约（api）都要同步，这两个次域是「改这族必须读哪些」的真实来源，不是实现手段。

**更正（R-8）**：这并非本仓首次同时挂 `ui` + `api`——`client-poll-budget` 已是 `domain: api, ui`。同时要意识到代价：`ui` 与 `api` 是**工具域**，挂上去会让「改前端布局」时的反查结果里混进契约 spec，稀释工具域的纯度。本族仍选择挂，因为 `board-api` 确实横跨前后端；但后续族若无同样跨度，不应照抄。

## D7 已知跨族重复（4 组，登记不修）

本族按「管线 vs 接口」切，但升级建议这条链路在两侧都落了地（R-3 实测）：

| 功能 | `board-upgrade` 落点 | `board-api` 落点 | 性质 |
|---|---|---|---|
| 触发/生成 | 辅助标签聚类升级触发、预聚类压缩候选、LLM 判断升级/跳过 | 升级建议 API（写了「执行聚类 + LLM 判断流程」） | **越界**：API spec 里写了算法流程 |
| 确认执行 | 用户确认后执行升级建议、ConfirmSuggestion API 合并路径保持不变 | 升级建议 DTO 的 Scenario「确认执行」 | 重复 |
| 回填 | 手动回填队列（all/unassigned/board 三模式 + 入队） | 回填触发 API（同一组模式 + 进度查询） | 同一功能两条 |
| 面板 | 生成入口模式选择、前端渲染 compose 建议 | 升级建议面板支持逐项处理和重新生成 | 同一面板 3 条契约分两族 |

**处置：保留现状 + 后续收口**。本 change 不改正文（会打破 32=32 守恒，且改契约属语义变更）。

分工约定（自本 change 起生效）：**改接口字段/参数/分页只看 `board-api`；改聚类/判定/阈值只看 `board-upgrade`；两边都动时先看这条**。

收口（把 `board-api` 里的生成侧描述降为指针、把 `board-upgrade` 的「手动回填队列」并入接口族）需另立 change——登记在 tasks 7.4。

## D8 红线 6/7 的 spec 覆盖不足（登记）

`docs/reference/flow/semantic-board.md` 红线 6（`suggestion_hash` 幂等、同 hash pending 则 skipped）与红线 7（dismiss 后 14 天冷却期内同 hash 不得重生），在 spec 里只有 `board-upgrade-expand` 的一条 Requirement 写了「`suggestion_hash` 幂等与 dismissed 冷却期复用现有机制」+ 1 个 Scenario。

**不足**：①只覆盖 compose 场景，未覆盖 create_new / merge_into_existing；②没写 14 天默认值；③没写冷却起算点（`resolved_at`）；④「复用现有机制」指向的是当时被删的旧条款，读者无从查证。

代码侧完整实现（`backend_go/.../board_upgrade_suggestion.go:34,67`），所以这是**文档欠债**不是实现缺失。补独立条款需另立 change。

## D9 前端契约归族判据

`board-editing-ui` 归 `board-api`，但 `upgrade-candidate-time-window` 的「前端时间窗口选择器」留在 `board-upgrade`——review 指出这不一致（R-7）。

判据：**前端契约跟着「它配置的是谁」走，不跟「它在哪个页面」走**。

- 时间窗口选择器配置的是**升级候选的收集范围**（算法参数），跟 `board-upgrade` ✓
- 编辑对话框字段、`MatchDetailPanel` 展示配置的是**接口返回什么**，跟 `board-api` ✓

这条判据与 D1 的「管线 vs 接口」同源：前端控件配置算法 → 归算法族；前端控件展示接口数据 → 归接口族。

## D10 已知损失

指向 4 个被删 capability 的 archived delta 落 `domain-history.sh` 第 6 段；只碰过它们的 change 会从反查消失（与既往两族同一损失模式，别名表仍待做）。

`board-concept-management` 不参与本次合并——它有并发会话未提交的改动，且内容全 DEPRECATED，退役应独立处理。
