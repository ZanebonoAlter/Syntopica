# board-health-checkup Specification

## Purpose
版块体检：对活跃版块的构成标签质量与成员语义做每日纯读统计，产出 LLM 综合评分、体检快照与异常通知，并以事实标签画像 diff 提供高证据的扩充候选来源；体检自动化与扩充建议手动生成严格分离。

## Requirements

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

体检 SHALL 将统计快照与标签分诊信号（磁铁信号、最近邻证据、描述污染预筛）一并交 LLM 综合裁决，单次调用产出：评分、评语、打扰裁决与**构成标签分诊清单**；MUST NOT 使用固定权重公式合成评分。LLM 调用失败或返回非法时 SHALL 降级：纯统计与信号部分照常持久化，评分、评语与分诊清单标记为空，体检轮次 MUST NOT 因此失败或重试阻塞。

分诊清单 SHALL 对每个构成标签给出四类之一，且每条 MUST 附证据摘要（信号数值或统计事实，MUST NOT 虚构证据）：

- 🔴 **建议摘除**：须有磁铁信号成立（失败子集贴合差 + 失败挂载流向）或等价强证据；仅凭贴合度位次（low_sim_ratio 高）MUST NOT 判摘除；
- 🟡 **建议重写描述**：须有最近邻漂移/窄化证据或描述事件性预筛命中，并给出建议的新描述文案；
- 🟢 **保留澄清**：cosine 天然低但挂载方向通过率高（短实体名天花板）或绝对健康（cosine_mean ≥ 0.6），防误杀；
- ⚪ **证据不足**：信号 insufficient 或样本 < 最小样本量，下轮再判。

评分判据 SHALL 防三类系统性误读（fix-board-health-llm-misread，2026-10-22 实证），判据在分诊层同步生效：

1. **贴合度分化须双条件**：low_sim_ratio 是板内相对 P25 线的位次占比，「分化明显」判据须同时满足相对垫底与绝对不健康（多数达标标签 cosine_mean < 0.6）；identity 事件句 × 短名词标签的 cosine 天然带 0.51-0.76，mean ≥ 0.6 的标签 low_sim_ratio 高只是位次不是跑题证据。
2. **短实体标签不扣分且不判摘**：人名/地名/技术名词等短标签语义聚焦，cosine_mean 低于板内泛概念标签属正常形态——评分不扣分依据，分诊 MUST NOT 仅因 cosine 低归入摘除。
3. **composite 常态保护**：composite_hit 仅在板挂载组合标签时可能产生；无组合挂载的板 reason_dist 全为单标签路径是常态非病征，该病征仅在弱路径占比主导且 ghost 占比同时偏高时成立。

#### Scenario: LLM 产出评分与评语
- **WHEN** 版块「AI 前沿」体检统计与分诊信号完成并送 LLM 综合裁决
- **THEN** 快照 SHALL 含 LLM 评分、评语与全部构成标签的分诊清单（每条含类别与证据摘要）

#### Scenario: LLM 失败降级不丢统计
- **WHEN** 某版块的 LLM 综合评分调用超时
- **THEN** 该版块快照的统计与信号部分 SHALL 照常持久化，评分/评语/分诊清单为空，体检轮次继续处理后续版块

#### Scenario: 短实体标签不因低 cosine 被判摘除
- **WHEN** 构成标签「OpenAI」cosine_mean=0.57、挂载方向通过率 100%、无磁铁信号
- **THEN** 分诊 SHALL 归入 🟢 保留澄清（附短名天花板证据），MUST NOT 归入建议摘除

#### Scenario: 磁铁标签凭失败子集证据判摘
- **WHEN** 构成标签「X」磁铁信号成立（失败子集均值 0.52 vs 全体 0.68、失败挂载 40 个且集中流向某对手版块）
- **THEN** 分诊 SHALL 归入 🔴 建议摘除并附两侧均值、样本数与流向证据

#### Scenario: 证据不足标签归入下轮再判
- **WHEN** 构成标签「Y」失败子集样本仅 2 个（低于最小样本量）且贴合度达标
- **THEN** 分诊 SHALL 归入 ⚪ 证据不足，MUST NOT 给出摘除或重写结论

#### Scenario: 相对垫底但绝对健康的标签不判分化
- **WHEN** 板块达标标签 cosine_mean 全部 ≥ 0.6（如俄乌板 0.632-0.700）且个别短实体标签 low_sim_ratio 0.5-0.7
- **THEN** 评分 SHALL NOT 因「贴合度分化」降档，分诊 SHALL NOT 因此判摘

#### Scenario: 无组合挂载的板不因 reason_dist 判病
- **WHEN** 板块 reason_dist 全为单标签路径（weighted/max_sim/hit_rate）且 composite_hit=0 且 ghost 占比低
- **THEN** 「弱命中路径主导」SHALL NOT 单独作为降档病征或摘除证据

#### Scenario: 真分化仍被拦截
- **WHEN** 板块多数达标标签 cosine_mean < 0.6 且 low_sim_ratio 大面积偏高
- **THEN** 评分 SHALL 按「贴合度分化明显」正常降档（双条件同时满足，防误读不等于放松真病征）

### Requirement: 体检快照持久化与趋势

每轮体检 SHALL 为每个活跃版块持久化一条体检快照（统计、分诊信号、未收编候选、评分、评语、分诊清单、时间戳）。快照 SHALL 可按版块查询最近 N 条以支撑评分趋势展示；历史快照 MUST NOT 被后续轮次覆盖或修改；历史快照缺少分诊字段时消费方 SHALL 按旧形态展示（向后兼容）。

#### Scenario: 每轮落一条快照
- **WHEN** 某日体检轮次完成版块「AI 前沿」
- **THEN** 该版块新增一条当日快照

#### Scenario: 历史快照不被覆盖
- **WHEN** 版块「AI 前沿」已存在 7 条历史快照且新体检轮次完成
- **THEN** 系统 SHALL 新增一条快照，7 条历史快照内容不变

#### Scenario: 查询趋势序列
- **WHEN** 前端请求版块「AI 前沿」的体检趋势
- **THEN** 系统 SHALL 返回按时间升序的最近 N 条快照评分序列

#### Scenario: 历史快照缺分诊字段兼容
- **WHEN** 消费方读取 2026-10 之前的历史快照（metrics 无分诊信号与清单字段）
- **THEN** 统计与评分部分 SHALL 正常返回，消费方按旧形态展示且不报错

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

体检数据 SHALL 经只读端点查询：最新快照（含统计、分诊信号、未收编候选、评分、评语、分诊清单）与趋势序列。端点 MUST NOT 写库或触发体检执行。版块不存在时返回 404；版块为 disabled 或非 board 类型时不报错，返回空结果。

#### Scenario: 查询最新快照
- **WHEN** 请求版块「AI 前沿」的体检数据且存在快照
- **THEN** 端点 SHALL 返回最新快照与趋势序列（含分诊清单）

#### Scenario: 无快照返回空态
- **WHEN** 版块「新能源」刚创建、体检从未跑过
- **THEN** 端点 SHALL 返回空态数据（非错误）

#### Scenario: 版块不存在 404
- **WHEN** 请求的板块 ID 不存在
- **THEN** 端点 SHALL 返回 404

### Requirement: 标签分诊信号计算

体检 SHALL 为每个活跃版块的每个构成标签计算三类确定性分诊信号，与贴合度统计同轮生成、同一纯读口径（不重算匹配、不筛样本）：

1. **磁铁信号**：构成标签 embedding 对「direction_mismatch=true 的挂载 tag 子集」与「全部挂载 tag」的贴合度均值差——失败子集均值显著低于全体均值（差值达配置阈值）且失败子集样本量达最小样本量时，该标签标记磁铁嫌疑；任一侧样本不足或向量缺失时信号 MUST 标记 insufficient，MUST NOT 按差值硬凑。
2. **最近邻信号**：构成标签在全库 active 辅助标签向量空间中的 top-3 最近邻（排除自身）及其 cosine 值，作为窄化/漂移判断证据随信号输出；标签自身 embedding 缺失时该信号 insufficient。
3. **描述污染预筛**：标签 description 命中文章式词根（「此次/本次/该事件/报道/针对/期间/近日」等）时标记事件性嫌疑；description 为空不标记。

信号 SHALL 作为 LLM 分诊的证据输入随统计快照持久化；信号计算本身 MUST NOT 产生任何写库动作。

构成标签清单 SHALL 逐条携带标签类型（`label_type`：辅助标签 `auxiliary` / 组合标签 `composite`），供消费方区分可执行与只读的分诊项；历史快照缺该字段时消费方 SHALL 按「类型未知」处理，MUST NOT 臆造类型。

#### Scenario: 磁铁信号失败子集贴合差成立
- **WHEN** 构成标签「A」对版块全部 400 个挂载 tag 的贴合度均值为 0.68，其中 40 个 direction_mismatch=true 的挂载子集均值为 0.52，差值超过配置阈值且失败子集样本量达标
- **THEN** 「A」的磁铁信号 SHALL 成立并携带两侧均值与样本数

#### Scenario: 失败子集样本不足不强判
- **WHEN** 版块仅有 2 个 direction_mismatch=true 的挂载 tag，低于失败子集最小样本量
- **THEN** 全部构成标签的磁铁信号 SHALL 标记 insufficient，不给出嫌疑结论

#### Scenario: 最近邻证据含 cosine 与排除自身
- **WHEN** 计算构成标签「上海」的最近邻信号
- **THEN** SHALL 返回全库 active 辅助标签中 cosine 最高的 3 个（排除「上海」自身）及各自 cosine 值

#### Scenario: description 事件性词根命中
- **WHEN** 构成标签「卡塔尔」的 description 为「位于中东的阿拉伯国家，此次派遣代表团前往伊朗进行外交磋商」
- **THEN** 该标签的描述污染预筛 SHALL 标记事件性嫌疑

#### Scenario: 向量缺失信号不硬算
- **WHEN** 构成标签自身 embedding 为 NULL
- **THEN** 其磁铁信号与最近邻信号 SHALL 均标记 insufficient，MUST NOT 按 0 或缺失值参与嫌疑判定

#### Scenario: 构成清单携带标签类型
- **WHEN** 某版块构成含辅助标签与组合标签
- **THEN** 快照的构成标签清单 SHALL 逐条携带 `label_type`（auxiliary / composite）

### Requirement: 分诊动作边界

分诊清单是建议而非执行：**摘除**建议 SHALL 仅经用户确认后调用既有手动摘除接口完成，系统 MUST NOT 因分诊自动移除任何构成标签；**重写 description** 建议 SHALL 展示可编辑建议文案与污染证据，只有用户另行核对并确认共享影响范围后才可进入独立的描述修复任务（见 auxiliary-label-description-repair）。体检轮次/查询本身 MUST NOT 修改 description/embedding 或写匹配结果表/升级建议表；用户确认的修复写链与体检纯读链 MUST 隔离。

构成清单中的**组合标签**（composite）SHALL 与辅助标签一同进入分诊裁决（证据照常展示，不隐藏、不当它健康），但描述修复链只覆盖辅助标签：组合标签的**重写 description** 建议 SHALL 只读展示、SHALL NOT 提供描述修复入口；**摘除**建议 SHALL 仍经既有构成摘除接口由用户确认后执行。

#### Scenario: 摘除经用户确认走既有接口
- **WHEN** 用户在体检卡分诊清单的 🔴 行点击「摘除」并确认
- **THEN** 系统 SHALL 调用既有版块构成摘除接口移除该构成标签，本轮无任何新的执行路径

#### Scenario: 重写建议不自动执行
- **WHEN** 分诊清单将某标签归入 🟡 建议重写描述
- **THEN** 系统 SHALL 展示建议与核对入口；在用户另行明确确认前，MUST NOT 修改 description/embedding 或创建执行任务

#### Scenario: 分诊不产生匹配写入
- **WHEN** 一轮含分诊清单的体检完成
- **THEN** 匹配结果表与升级建议表 MUST NOT 因此新增或修改任何行

#### Scenario: 组合标签的分诊项只读（仅摘除可处理）
- **WHEN** 分诊把一个组合标签（composite）归入建议重写描述
- **THEN** 界面 SHALL 只读展示该条证据并说明不能在此处修复（不提供描述修复入口），其「建议摘除」SHALL 仍可经既有构成摘除接口由用户确认后执行

### Requirement: 分诊以问题处理为主线

完整依据 SHALL 默认以紧凑问题清单展示需核对的 remove/rewrite 标签，选择一项后只展开该项证据、动作与下一步，不再按四类分别纵向堆卡片。keep/insufficient 与统计依据 SHALL 按需展开且保留原含义；操作提交、部分完成、全部匹配更新完成和效果待验证 MUST 明确区分。

#### Scenario: 先处理问题而非遍历四组卡片
- **WHEN** 快照含 remove、rewrite、keep、insufficient 四类结果
- **THEN** 默认显示待核对问题索引与当前项处理区；保留/不足及统计依据可展开，所有结果仍可达

#### Scenario: 修复完成不冒充体检验证
- **WHEN** 用户确认的描述修复任务已完成匹配更新
- **THEN** 界面 SHALL 引导处理下一项并标明效果待下一轮体检验证，MUST NOT 更新旧快照评分或声称体检已恢复健康
