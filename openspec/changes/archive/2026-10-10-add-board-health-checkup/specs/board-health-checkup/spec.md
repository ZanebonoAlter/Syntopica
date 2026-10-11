## Purpose

版块体检：对活跃版块的构成标签质量与成员语义做每日纯读统计，产出 LLM 综合评分、体检快照与异常通知，并以事实标签画像 diff 提供高证据的扩充候选来源；体检自动化与扩充建议手动生成严格分离。

## ADDED Requirements

### Requirement: 体检统计与标签贴合度归因
体检 SHALL 对每个活跃版块产出两级统计：版块级指标使用全部挂载匹配行；标签级指标（每个构成标签的贴合度分布）SHALL 使用该构成标签 embedding 与该版块全部挂载 tags 的 identity embedding 的余弦相似度分布，MUST NOT 按 tag 的 aux 挂载数量筛选样本（不做功劳归因，只量语义贴合）。构成标签的样本数低于配置的最小样本量时，该标签 SHALL 标记为「数据不足」且不产出均值/中位数等分布统计（不得伪造）。

#### Scenario: 构成标签贴合度分布全覆盖
- **WHEN** 版块「AI 前沿」有 400 个挂载 tags（无论各自挂几个 aux）且构成含标签「推理」
- **THEN** 「推理」的贴合度样本 SHALL 覆盖该 400 个 tags（与 tag 挂几个 aux 无关）

#### Scenario: 样本不足显示数据不足
- **WHEN** 版块仅 3 个有向量的挂载 tags，低于配置的最小样本量
- **THEN** 该版块全部构成标签 SHALL 标记为数据不足，不展示均值/中位数

#### Scenario: 低分线按板块内相对分位
- **WHEN** 某版块内全部构成标签贴合度均值的 P25 分位为 0.62
- **THEN** 低分占比 SHALL 按 cosine < 0.62 计算（板块内自适应相对线，不用全局固定线）

### Requirement: 贴合度重算口径
体检的标签级贴合度统计 SHALL 现场重算构成标签 embedding × 挂载 tag identity embedding 的余弦相似度（匹配结果行上的规则分数为分层值，MUST NOT 作为均值/中位数的输入）。任一侧 embedding 缺失 SHALL 跳过分布统计并计入「向量缺失」计数，MUST NOT 按 0 计入分布；构成标签自身 embedding 缺失时该标签 SHALL 标记为数据不足。

#### Scenario: cosine 重算进入分布
- **WHEN** 构成标签「推理」的 embedding 与某挂载 tag 的 identity embedding 余弦相似度为 0.83
- **THEN** 0.83 SHALL 进入构成标签「推理」的贴合度分布样本

#### Scenario: 规则分数不作为相似度统计输入
- **WHEN** 某匹配行 match_reason 为 direct_hit、score 恒为 0.7
- **THEN** 标签级均值/中位数统计 MUST NOT 使用该规则分数

#### Scenario: 向量缺失跳过且计数
- **WHEN** 某挂载匹配行的 tag embedding 为 NULL
- **THEN** 该行不进入相似度分布，体检结果 SHALL 含「向量缺失」计数

### Requirement: 事实标签画像与未收编候选
体检 SHALL 对每个活跃版块计算事实标签画像：已挂载到该版块的 tags 所携带的辅助标签中、未出现在该版块构成里的，按成员 tag 数与文章覆盖数聚合频次，产出未收编候选清单。候选 MUST 排除已挂载进版块构成与 disabled 状态的标签。体检轮次内未收编候选 SHALL 只持久化为体检数据，MUST NOT 生成任何扩充建议（pending 行）。

#### Scenario: 高频未收编标签进入候选清单
- **WHEN** 已挂到版块「AI 前沿」的 tags 中有 12 个携带 aux「算力」，且「算力」不在版块构成中
- **THEN** 体检快照的未收编候选 SHALL 含「算力」及其成员 tag 数与文章覆盖数

#### Scenario: 已挂载构成标签不进候选
- **WHEN** aux「大模型」已在版块「AI 前沿」构成中
- **THEN** 「大模型」MUST NOT 出现在未收编候选清单里

#### Scenario: 体检轮次不落扩充建议
- **WHEN** 体检 job 完成一轮全部版块的统计
- **THEN** 扩充建议表 MUST NOT 因此新增任何 pending 行

#### Scenario: 未收编候选可一键收编
- **WHEN** 用户在体检面板的未收编候选行点击「收编」
- **THEN** 系统 SHALL 经既有手动挂载接口将该 aux 加入版块构成（用户手动确认语义，不经 LLM 建议管道），并提示历史归属不自动回填

### Requirement: LLM 综合评分与降级
体检 SHALL 将统计快照交 LLM 综合裁决，产出单一评分与一句评语；MUST NOT 使用固定权重公式合成评分。LLM 调用失败或返回非法时 SHALL 降级：纯统计部分照常持久化，评分与评语标记为空，体检轮次 MUST NOT 因此失败或重试阻塞。

#### Scenario: LLM 产出评分与评语
- **WHEN** 版块「AI 前沿」体检统计完成并送 LLM 综合裁决
- **THEN** 快照 SHALL 含 LLM 评分与评语

#### Scenario: LLM 失败降级不丢统计
- **WHEN** 某版块的 LLM 综合评分调用超时
- **THEN** 该版块快照的统计部分 SHALL 照常持久化，评分/评语为空，体检轮次继续处理后续版块

### Requirement: 体检快照持久化与趋势
每轮体检 SHALL 为每个活跃版块持久化一条体检快照（统计、未收编候选、评分、评语、时间戳）。快照 SHALL 可按版块查询最近 N 条以支撑评分趋势展示；历史快照 MUST NOT 被后续轮次覆盖或修改。

#### Scenario: 每轮落一条快照
- **WHEN** 某日体检轮次完成版块「AI 前沿」
- **THEN** 该版块新增一条当日快照

#### Scenario: 历史快照不被覆盖
- **WHEN** 版块「AI 前沿」已存在 7 条历史快照且新体检轮次完成
- **THEN** 系统 SHALL 新增一条快照，7 条历史快照内容不变

#### Scenario: 查询趋势序列
- **WHEN** 前端请求版块「AI 前沿」的体检趋势
- **THEN** 系统 SHALL 返回按时间升序的最近 N 条快照评分序列

### Requirement: 异常通知打扰裁决
体检评分异常（评分缺失或低于配置阈值）时，SHALL 将体检数据快照交 LLM 裁决是否值得通知用户；裁决值得时 SHALL 经现有通知系统创建通知（可跳转到对应版块）。裁决不值得或 LLM 裁决失败时 MUST NOT 落通知（静默，不重试轰炸）。同一版块同一异常在冷却期内 MUST NOT 重复通知。

#### Scenario: 异常且裁决值得则通知
- **WHEN** 版块「新能源」评分低于阈值且 LLM 裁决值得打扰
- **THEN** 系统 SHALL 创建一条可跳转到该版块的通知

#### Scenario: 裁决不值得则静默
- **WHEN** 版块「新能源」评分略低于阈值但 LLM 裁决不值得打扰
- **THEN** 系统 MUST NOT 创建通知

#### Scenario: 冷却期内不重复通知
- **WHEN** 版块「新能源」昨日已因评分异常通知过，今日异常仍在冷却期内
- **THEN** 系统 MUST NOT 再次创建该版块的异常通知

### Requirement: 体检自动化与扩充手动边界
体检 job SHALL 每日自动执行（挂日报生成流程完成后触发，当日日报优先产出），且为纯读统计：不修改匹配结果、不生成扩充建议、不自动执行任何版块变更。扩充建议的生成 MUST 保持用户手动触发；手动触发时候选来源见 board-upgrade-expand 能力的扩充候选召回要求。

#### Scenario: 体检在日报后自动执行
- **WHEN** 当日日报生成流程完成
- **THEN** 体检轮次 SHALL 自动开始执行

#### Scenario: 体检不改匹配数据
- **WHEN** 体检轮次完成
- **THEN** 匹配结果表 MUST NOT 出现任何由体检引起的写入

#### Scenario: 扩充建议仍需手动触发
- **WHEN** 体检产出大量未收编候选
- **THEN** 扩充建议 MUST NOT 自动生成；SHALL 等待用户在扩充入口手动触发

### Requirement: 体检数据查询端点只读
体检数据 SHALL 经只读端点查询：最新快照（含统计、未收编候选、评分、评语）与趋势序列。端点 MUST NOT 写库或触发体检执行。版块不存在时返回 404；版块为 disabled 或非 board 类型时不报错，返回空结果。

#### Scenario: 查询最新快照
- **WHEN** 请求版块「AI 前沿」的体检数据且存在快照
- **THEN** 端点 SHALL 返回最新快照与趋势序列

#### Scenario: 无快照返回空态
- **WHEN** 版块「新能源」刚创建、体检从未跑过
- **THEN** 端点 SHALL 返回空态数据（非错误）

#### Scenario: 版块不存在 404
- **WHEN** 请求的板块 ID 不存在
- **THEN** 端点 SHALL 返回 404
