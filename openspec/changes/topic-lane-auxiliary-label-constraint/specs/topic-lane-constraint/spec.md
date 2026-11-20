## Purpose

话题泳道的辅助标签约束：为话题绑定主体短词（如「以色列」「黎巴嫩」）作为 section 锚定到该话题的准入条件，解决「宽口径话题被单边内容殖民」——被吸进的内容成为质心滚动窗口的一员、质心随之偏移、下一批同类内容吸附更容易，形成自我强化的闭环。约束以**向量**为载体（而非 tag id 集合），因标签边按 `tag_edge_retention_days`（默认 7 天）回收、孤立标签随之清理，基于标签 id 的约束会自行失效。

## ADDED Requirements

### Requirement: 话题辅助标签约束数据模型

系统 SHALL 维护 `board_persistent_topic_constraints` 表，持久化每个话题的辅助标签约束。每行 SHALL 包含：`topic_id`（外键到 `board_persistent_topics`，ON DELETE CASCADE）、`term`（用户输入的主体短词，VARCHAR(100)，SHALL NOT 为空）、`term_embedding`（约束锚向量，**无维度限定 `vector`**——维度只活在运行时与数据里，不由 schema 承载）、`embedding_model`（计算该锚向量所用模型的标识，作为向量空间标识）、`mode`（`required`｜`preferred`）、`margin`（相对偏好严格度，NUMERIC，取值范围 [0, 1]）、`max_distance`（该约束的绝对距离上限，可空 NUMERIC，取值范围 [0, 1]）、`created_at`、`updated_at`。

`mode` SHALL 受 CHECK 约束为 required/preferred 二态，默认 required。`margin` SHALL 受 CHECK 约束限定在 [0, 1]，表示**相对偏好严格度**（见「约束覆盖判定语义」）；`max_distance` SHALL 为可空的 NUMERIC（取值范围 [0, 1]），表示该约束的**绝对距离上限**。同一话题内同一 `term` SHALL 唯一。表 SHALL 在 `topic_id` 上建 BTree 索引。建表 SHALL 通过显式数据库迁移完成，不依赖 gorm AutoMigrate。

约束 SHALL NOT 通过新增列的方式挂在 `board_persistent_topics` 上——既有表结构 SHALL 保持不变，存量话题在无约束行时行为与现状逐字一致。

#### Scenario: 添加约束写入完整行

- **WHEN** 用户为话题 #2 添加辅助标签「以色列」，mode=required，margin=0.05，max_distance=0.55，锚向量由模型 `qwen3-embedding` 计算得到
- **THEN** 系统 SHALL 写入一行，`topic_id`=2、`term`="以色列"、`embedding_model`="qwen3-embedding"、`mode`=required、`margin`=0.05、`max_distance`=0.55

#### Scenario: 模式取值受约束

- **WHEN** 尝试写入 `mode`='optional'
- **THEN** 系统 SHALL 因 CHECK 约束拒绝

#### Scenario: margin 与 max_distance 越界受约束

- **WHEN** 尝试写入 `margin`=1.5 或 `max_distance`=2.0
- **THEN** 系统 SHALL 因 CHECK 约束拒绝

#### Scenario: 同一话题内短词重复被拒

- **GIVEN** 话题 #2 已有约束 `term`="以色列"
- **WHEN** 用户再次为话题 #2 添加 `term`="以色列"
- **THEN** 系统 SHALL 因唯一约束拒绝，且 SHALL NOT 产生第二条约束行

#### Scenario: 话题删除级联清理约束

- **WHEN** 话题 #2 被删除
- **THEN** 其全部约束行 SHALL 随外键级联删除

### Requirement: 约束锚向量计算与向量空间标识

用户提交辅助标签时，系统 SHALL 用**当前 embedding 顺位**（`ai_routes` 中 capability=embedding 的 ordered_failover 路由）计算该短词的向量，落库并同时记录 `embedding_model`。

用户 SHALL NOT 被要求输入 description 或任何补充说明——约束锚的语义由短词本身承载，补充描述带来的心智负担过高。短词 SHALL 受长度上限约束（100 字符）。

锚向量计算失败时（embedding 服务不可用、返回不可解析、维度与既有向量不一致），系统 SHALL 返回错误并 SHALL NOT 落库，SHALL NOT 写入空向量或零向量。

`embedding_model` SHALL 随约束行一同持久化，使判定方能够识别该约束锚所属的向量空间。

#### Scenario: 短词计算锚向量并记录模型标识

- **WHEN** 用户提交短词「黎巴嫩」
- **THEN** 系统 SHALL 用当前顺位模型计算其向量、写入 `term_embedding`，并将所用模型名写入 `embedding_model`

#### Scenario: 不要求用户补充描述

- **WHEN** 用户提交辅助标签请求，载荷仅含短词
- **THEN** 系统 SHALL 接受并完成锚向量计算，SHALL NOT 要求 description 或其它补充字段

#### Scenario: 计算失败不落库

- **WHEN** 锚向量计算失败
- **THEN** 系统 SHALL 返回错误，SHALL NOT 写入约束行

### Requirement: 约束覆盖判定语义

对某个待归属 section 与其候选话题，系统 SHALL 按 section 的标签集合逐条判定约束覆盖。

**判定 SHALL 基于「相对偏好」而非单一绝对距离阈值**：向量的绝对距离无法区分「双主体」与「单主体」——实测「以色列」与「黎巴嫩」两个主体锚的余弦距离仅 0.14~0.36，纯单侧内容到「另一侧」锚的距离与双侧内容到同侧锚的距离高度重叠，绝对阈值下正负样本分布不分离；而**同一个 tag 相对更靠近哪一侧锚**这一信号是稳定的。

- **相对偏好**：对每条约束 `i` 与其余约束 `j`，tag `t` 满足约束 `i` 的条件为 `d(t, A_i) + margin_i < d(t, A_j)`，其中 `d` 为余弦距离、`A_i` 为该约束的锚向量、`margin_i` 为该约束的相对偏好严格度。约束 `i` 被覆盖 SHALL 意为 section 的标签集合中**存在至少一个** tag 满足该条件。
- **绝对上限**：`max_distance` 非空时，命中 tag SHALL 同时满足 `d(t, A_i) < max_distance_i`；为空时该项不参与判定。绝对上限用于剔除离锚过远、相对偏好不具判别意义的 tag。
- **单约束退化**：话题只有一条约束（无其它约束锚可比）时，相对偏好无参照，SHALL 仅以 `max_distance` 判定；`max_distance` 为空时该约束 SHALL 视为无法判定，SHALL NOT 参与拦截。
- **多条 required 约束**：SHALL **全部**被覆盖时，约束判定为「满足」；任一 required 约束未被覆盖时，判定为「违反」。不同约束 SHALL 可由标签集合中的**不同标签**分别满足（分腿覆盖）。
- **`preferred` 约束**：SHALL NOT 参与拦截判定，仅作加权信号。
- **跨向量空间**：标签向量与约束锚的 `embedding_model` 不一致时，该标签 SHALL 被跳过——SHALL NOT 计入覆盖，也 SHALL NOT 判为违反（无法判定不等于违反）。
- **无标签的 section**：SHALL 跳过约束检查，SHALL NOT 判为违反。

未覆盖 required 约束时，section SHALL NOT 锚定到该话题。

#### Scenario: 双侧 required 被不同标签分腿覆盖

- **GIVEN** 话题 #2 有 required 约束「以色列」与「黎巴嫩」（margin 各 0.05，max_distance 各 0.55），其 section 的标签集合含 tag「以色列空袭加沙」——该 tag 到「以色列」锚距离 0.31、到「黎巴嫩」锚距离 0.52（相对偏好 0.21 > margin），以及 tag「真主党与以军交火」——该 tag 到「黎巴嫩」锚距离 0.38、到「以色列」锚距离 0.60
- **WHEN** 系统判定该 section 对话题 #2 的约束覆盖
- **THEN** 两条 required 约束 SHALL 均被覆盖，判定结果 SHALL 为「满足」

#### Scenario: 单侧 required 未覆盖判违反

- **GIVEN** 同上话题，但该 section 的标签集合仅含上述「以色列空袭加沙」一条 tag
- **WHEN** 系统判定约束覆盖
- **THEN** 「黎巴嫩」约束 SHALL 判定为未被覆盖，整体判定结果 SHALL 为「违反」

#### Scenario: 纯单侧内容不因相对偏好误判为双侧

- **GIVEN** 话题 #2 有 required 约束「以色列」与「黎巴嫩」，某 section 的标签集合全部是纯以色列内政 tag（每个 tag 到「以色列」锚更近，到「黎巴嫩」锚更远）
- **WHEN** 系统判定约束覆盖
- **THEN** 「黎巴嫩」约束 SHALL 判定为未被覆盖，判定结果 SHALL 为「违反」

#### Scenario: preferred 未覆盖不拦截

- **GIVEN** 话题 #2 只有 preferred 约束「真主党」，section 标签集合无标签满足其覆盖条件
- **WHEN** 系统判定约束覆盖
- **THEN** 判定结果 SHALL 为「满足」（preferred 不参与拦截）

#### Scenario: 跨向量空间的标签被跳过

- **GIVEN** 某约束锚的 `embedding_model` 为 `qwen3-embedding`，某 section 标签向量的模型标识为本地 gguf 模型
- **WHEN** 系统判定约束覆盖
- **THEN** 该标签 SHALL 被跳过，既不计入覆盖也不判违反

#### Scenario: 无标签 section 跳过约束检查

- **WHEN** section 的标签集合为空
- **THEN** 系统 SHALL 跳过约束检查，SHALL NOT 判为违反

#### Scenario: 单个标签不得独自满足多条 required 约束

- **GIVEN** 话题 #2 有 required 约束「以色列」与「黎巴嫩」，section 的标签集合只有「以色列空袭加沙」一个标签，该标签到「以色列」锚距离 0.20、到「黎巴嫩」锚距离 0.51
- **WHEN** 系统判定约束覆盖
- **THEN** 「黎巴嫩」约束 SHALL 判定为未被覆盖，整体判定结果 SHALL 为「违反」

#### Scenario: 单约束话题按绝对上限判定

- **GIVEN** 话题 #2 只有一条 required 约束「以色列」，max_distance=0.45，section 的标签集合含一个到「以色列」锚距离 0.30 的 tag
- **WHEN** 系统判定约束覆盖
- **THEN** 该约束 SHALL 判定为被覆盖（无其它约束锚可比，退化为绝对上限判定）

### Requirement: 约束对归属判定的影响

话题存在 required 约束时，系统 SHALL 在将该 section 锚定到该话题**之前**完成约束覆盖判定：

- 判定为「满足」时，SHALL 按既有三层分桶行为归属（`l1_direct` 直挂仍不调用 LLM）。
- 判定为「违反」时，该 section **SHALL NOT** 锚定到该话题；原属 L1 直挂路径的 SHALL 降级进入 L2 band 交 LLM 重新裁决；L2 裁决中 LLM 选定的话题若其 required 约束同样未被覆盖，系统 SHALL 沿用既有「switch 到候选集外 → 降级为 new」的范式处理，SHALL NOT 锚定到约束不满足的话题。

话题无任何约束行时，约束判定 SHALL 为「满足」，既有归属行为 SHALL 逐字不变，SHALL NOT 产生额外 LLM 调用。

#### Scenario: 满足约束保持 L1 直挂

- **GIVEN** 话题 #2 的 required 约束被该 section 的标签集合全部覆盖，且某 tag 到该话题质心距离 0.10（< L1 阈值 0.18）、话题为 active 且非 vacuum
- **WHEN** 系统执行三层分桶
- **THEN** 该 tag 所属 section SHALL 仍以 `lane_tier`=l1_direct 归属该话题，SHALL NOT 额外调用 LLM

#### Scenario: 违反约束的 L1 直挂降级 L2

- **GIVEN** 同上话题与距离，但该 section 的标签集合未覆盖其 required 约束
- **WHEN** 系统执行三层分桶
- **THEN** 该 tag SHALL 进入 L2 band 交 LLM 裁决，SHALL NOT 直挂到该话题

#### Scenario: L2 裁决选中约束不满足的话题时降级

- **GIVEN** L2 裁决中 LLM 输出「留」指向话题 #2，而 #2 的 required 约束未被该 section 的标签集合覆盖
- **WHEN** 系统处理裁决结果
- **THEN** 系统 SHALL 沿用「switch 到候选集外 → 降级为 new」的既有范式处理，SHALL NOT 锚定到话题 #2

#### Scenario: 无约束话题行为不变

- **GIVEN** 话题 #9 无任何约束行
- **WHEN** 系统执行三层分桶
- **THEN** 归属行为 SHALL 与引入约束前逐字一致，SHALL NOT 产生额外 LLM 调用

### Requirement: 约束管理接口

系统 SHALL 提供话题约束的读取与写入接口：

- `GET /api/daily-reports/topics/:id/constraints` SHALL 返回该话题的约束列表，每条含 `term`、`mode`、`margin`、`max_distance`、`embedding_model` 与创建/更新时间。**SHALL NOT 返回 `term_embedding` 原始向量**。
- `PUT /api/daily-reports/topics/:id/constraints` SHALL 以**全量覆盖**语义批量替换该话题的约束集合，SHALL 为幂等操作（相同请求重复提交产生相同状态）。请求体 SHALL 支持新增、更新与删除（删除通过在该次提交中省略该条实现）。

话题不存在时，系统 SHALL 返回 404。请求中含重复 `term`、非法 `mode`、越界 `margin` 或越界 `max_distance` 时，系统 SHALL 返回 400 并 SHALL NOT 部分写入。

#### Scenario: 读取约束列表

- **GIVEN** 话题 #2 有两条约束
- **WHEN** 客户端请求该话题的约束列表
- **THEN** 系统 SHALL 返回两条约束的元信息，且 SHALL NOT 返回原始向量

#### Scenario: 批量替换幂等

- **GIVEN** 话题 #2 当前有约束 A、B
- **WHEN** 客户端两次提交相同的「只保留 A」替换请求
- **THEN** 两次提交后的约束集合 SHALL 相同，且第一次提交后约束 B SHALL 已被删除

#### Scenario: 非法载荷不部分写入

- **WHEN** 客户端提交的替换请求中含一条 `mode`='optional' 的约束与一条合法约束
- **THEN** 系统 SHALL 返回 400，且该话题的约束集合 SHALL 保持提交前的状态（SHALL NOT 部分写入）

#### Scenario: 话题不存在返回 404

- **WHEN** 客户端请求话题 #99999 的约束列表
- **THEN** 系统 SHALL 返回 404

### Requirement: 约束不参与话题生命周期

话题辅助标签约束 SHALL NOT 影响话题的生命周期状态机：`planLifecycle` SHALL 仍只更新命中计数（`hit_count` / `consecutive_hits` / `last_seen_date`），SHALL NOT 因约束判定结果变更任何 `status`；约束的增删改 SHALL NOT 触发任何状态变更或自动归档。

约束判定为「违反」导致的归属变化 SHALL NOT 减少话题的命中计数——该 section 本就未归属该话题，不构成一次「未命中」。

#### Scenario: 约束违例不减少命中计数

- **GIVEN** 话题 #2 昨日命中计数为 10
- **WHEN** 今日某 section 因约束违反未归属话题 #2
- **THEN** 话题 #2 的 `hit_count` SHALL 保持 10（该 section 未构成一次命中），`consecutive_hits` 按既有未命中规则处理

#### Scenario: 增删约束不改变话题状态

- **GIVEN** 话题 #2 的 status 为 active
- **WHEN** 用户为其增删约束
- **THEN** 话题 #2 的 status SHALL 保持 active，SHALL NOT 触发任何状态变更
