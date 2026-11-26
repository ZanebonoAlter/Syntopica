## Purpose

诊断 = 判定版块质量、产出信号/评分/分诊建议，**不改变版块本身**。本族定义两类判定：①版块体检——贴合度归因、事实标签画像、LLM 综合评分与降级、快照趋势、异常通知打扰裁决、分诊信号与合理性闸门、体检纯读边界；②匹配方向校验——除 direct_hit 外的命中须算 tag identity 与 board 向量 cosine，低于阈值标 `direction_mismatch`，并规定该标记在日报与 API 的消费口径。两者都只读不写，故归诊断；匹配决策本身见 `tag-matching`。

## ADDED Requirements

### Requirement: 体检统计与标签贴合度归因

<!-- from: board-health-checkup (2026-10-10 add-board-health-checkup) -->

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

<!-- from: board-health-checkup (2026-10-10 add-board-health-checkup) -->

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

<!-- from: board-health-checkup (2026-10-10 add-board-health-checkup) -->

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

<!-- from: board-health-checkup (2026-10-10 add-board-health-checkup) -->


体检 SHALL 将统计快照与标签分诊信号（磁铁信号、最近邻证据、描述污染预筛、核心主体标记）一并交 LLM 综合裁决，单次调用产出：评分、评语、打扰裁决与**构成标签分诊清单**；MUST NOT 使用固定权重公式合成评分。

分诊清单的合法性 SHALL 分三级处置，**MUST NOT 因模型漏判部分标签而丢弃整轮裁决**：

1. **合法完整**：覆盖全部构成标签 → 原样持久化。
2. **合法但不完整**（漏判部分构成标签，且覆盖率 ≥ 最小覆盖率下限）→ 系统 SHALL 保留评分、评语与已给出的分诊项，并把缺失的构成标签逐条补齐为 ⚪ 证据不足，证据摘要 SHALL 说明「本轮 LLM 未覆盖该标签、下轮再判」。补齐项 MUST NOT 被猜成 keep/remove/rewrite——缺证据就只能是证据不足。
3. **非法**（category 不在闭集、引用未知或重复 aux_id、evidence 为空、rewrite 缺建议文案，或覆盖率低于最小覆盖率下限）→ 整轮降级：纯统计与信号部分照常持久化，评分、评语与分诊清单标记为空。LLM 调用失败（超时/接口错误）同样整轮降级。

最小覆盖率下限 SHALL 有固定默认值（默认 50%），只在体检内部使用，MUST NOT 因调参而改变「漏判即补证据不足」的主语义。降级与补齐 MUST NOT 使体检轮次失败、重试或阻塞后续版块。

分诊清单 SHALL 对每个构成标签给出四类之一，且每条 MUST 附证据摘要（信号数值或统计事实，MUST NOT 虚构证据）：

- 🔴 **建议摘除**：须有磁铁信号成立（失败子集相对健康子集的贴合差 + 失败挂载流向）或等价强证据；仅凭贴合度位次（low_sim_ratio 高）MUST NOT 判摘除；磁铁信号不成立或信号不足时 MUST NOT 判摘除；
- 🟡 **建议重写描述**：须有最近邻漂移/窄化证据或描述事件性预筛命中，并给出建议的新描述文案；对国家、地区、城市、机构、人物等实体类标签，**同类最近邻（如「伊朗」的最近邻是「中东地区」「中东国家」）MUST NOT 记为漂移/窄化证据**；
- 🟢 **保留澄清**：cosine 天然低但挂载方向通过率高（短实体名天花板）或绝对健康（cosine_mean ≥ 0.6），或**该标签被标记为核心主体**，或合理多归属：失败挂载集中流向语义重叠的对手版块属边界重叠，是版块定义问题而非标签污染，应归 keep（必要时 rewrite 版块定义），多归属不是摘除的充分依据；
- ⚪ **证据不足**：信号 insufficient、样本 < 最小样本量，或本轮模型未覆盖，下轮再判。

评分判据 SHALL 防三类系统性误读（fix-board-health-llm-misread，2026-10-22 实证），判据在分诊层同步生效：

1. **贴合度分化须双条件**：low_sim_ratio 是板内相对 P25 线的位次占比，「分化明显」判据须同时满足相对垫底与绝对不健康（多数达标标签 cosine_mean < 0.6）；identity 事件句 × 短名词标签的 cosine 天然带 0.51-0.76，mean ≥ 0.6 的标签 low_sim_ratio 高只是位次不是跑题证据。
2. **短实体标签不扣分且不判摘**：人名/地名/技术名词等短标签语义聚焦，cosine_mean 低于板内泛概念标签属正常形态——评分不扣分依据，分诊 MUST NOT 仅因 cosine 低归入摘除。
3. **composite 常态保护**：composite_hit 仅在板挂载组合标签时可能产生；无组合挂载的板 reason_dist 全为单标签路径是常态非病征，该病征仅在弱路径占比主导且 ghost 占比同时偏高时成立。

分诊判据 SHALL 另防两类误读：① **核心主体保护**——被标记为核心主体的标签 MUST NOT 判建议摘除或建议重写；② **板级证据不作标签级证据**——版块级的失败挂载流向（magnet_flow）与版块级共享的失败子集只可作为佐证，MUST NOT 充当单个标签判摘除的必要证据。版块 description 为空时，系统 SHALL 在提示中显式告知模型「本板无描述文本，核心主体只能依据核心主体标记判定」，MUST NOT 让模型自行臆断板块范围。

每板块每轮体检 SHALL 输出一行可检索日志，含构成标签数、LLM 覆盖数、补齐数与降级判定；整轮降级 SHALL 记录具体原因。日志口径 MUST 使人能在不查库的情况下判断「这轮为什么没有依据」。

#### Scenario: LLM 产出评分与评语
- **WHEN** 版块「AI 前沿」体检统计与分诊信号完成并送 LLM 综合裁决
- **THEN** 快照 SHALL 含 LLM 评分、评语与全部构成标签的分诊清单（每条含类别与证据摘要）

#### Scenario: 模型漏判部分标签仍保留整轮裁决
- **WHEN** 版块有 50 个构成标签，LLM 只返回 45 条合法分诊项与合法评分
- **THEN** 快照 SHALL 保存评分、评语与 45 条原判分诊项，缺失的 5 个标签补为 ⚪ 证据不足（证据说明本轮未覆盖），分诊清单长度 SHALL 等于 50，MUST NOT 因漏判丢弃评分

#### Scenario: 补齐项不被猜成可执行建议
- **WHEN** 某个构成标签本轮未被 LLM 覆盖
- **THEN** 该标签的分诊类别 MUST 为 ⚪ 证据不足，MUST NOT 被补成建议摘除或建议重写，界面 MUST NOT 为其提供摘除/重写入口

#### Scenario: 覆盖率过低仍整轮降级
- **WHEN** 版块有 60 个构成标签，LLM 仅返回 12 条合法分诊项（覆盖率 20%，低于下限）
- **THEN** 评分、评语与分诊清单 SHALL 全部为空并记录原因，纯统计与信号部分照常持久化

#### Scenario: 非法分诊项整轮降级
- **WHEN** LLM 返回的某条分诊项 category 为 `delete`（闭集外）或引用不存在的 aux_id
- **THEN** 评分、评语与分诊清单 SHALL 标记为空并记录原因，体检轮次继续处理后续版块

#### Scenario: LLM 失败降级不丢统计
- **WHEN** 某版块的 LLM 综合评分调用超时
- **THEN** 该版块快照的统计与信号部分 SHALL 照常持久化，评分/评语/分诊清单为空，体检轮次继续处理后续版块

#### Scenario: 降级与补齐都有日志
- **WHEN** 一轮体检完成（含降级板块与补齐板块）
- **THEN** 每个板块 SHALL 各有一行日志给出标签数/覆盖数/补齐数/是否降级，整轮降级行 MUST 带原因

#### Scenario: 短实体标签不因低 cosine 被判摘除
- **WHEN** 构成标签「OpenAI」cosine_mean=0.57、挂载方向通过率 100%、无磁铁信号
- **THEN** 分诊 SHALL 归入 🟢 保留澄清（附短名天花板证据），MUST NOT 归入建议摘除

#### Scenario: 磁铁标签凭失败子集证据判摘
- **WHEN** 构成标签「X」磁铁信号成立（失败子集均值 0.72 vs 健康子集均值 0.52、失败挂载 40 个且集中流向某对手版块）
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

#### Scenario: 核心主体标签不得判摘除或重写
- **WHEN** 构成标签「伊朗」被标记为核心主体（出现在板块 description 中）
- **THEN** 分诊 SHALL 归入 🟢 保留澄清并说明核心主体，MUST NOT 归入建议摘除或建议重写

#### Scenario: 实体标签的同类最近邻不算漂移证据
- **WHEN** 构成标签「伊朗」的最近邻为「中东地区 0.922」「中东国家 0.905」（同类地域概念）且 desc_flags 未命中
- **THEN** 分诊 MUST NOT 以该最近邻为漂移证据判建议重写描述

#### Scenario: 板级失败挂载流向不作单标签摘除必要证据
- **WHEN** 板块 magnet_flow 的 top_board 为某对手板块，但某构成标签自身磁铁信号不成立
- **THEN** 该标签 MUST NOT 判建议摘除

### Requirement: 体检快照持久化与趋势

<!-- from: board-health-checkup (2026-10-10 add-board-health-checkup) -->


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

<!-- from: board-health-checkup (2026-10-10 add-board-health-checkup) -->

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

<!-- from: board-health-checkup (2026-10-10 add-board-health-checkup) -->


体检 SHALL 每日自动执行且可由**三条日报产出路径**触发，三者共用同一体检语义（纯读统计：不修改匹配结果、不生成扩充建议、不自动执行任何版块变更）：

1. **日报定时主轮**成功后（当日日报优先产出，主轮有失败版面则不跑）；
2. **回填补出日报**后（回填为本轮补出了至少一份日报时）；
3. **手动生成端点**（`POST /api/daily-reports/generate`）成功产出至少一份日报后，scope SHALL 限定为本次生成的板块。

为避免同一晚重复调用 LLM，**回填与手动两条路径**触发前 SHALL 对每个板块做**新鲜度守卫**：该板块最近已有**带评分的**体检快照（`llm_score` 非空）且距今不足最小体检间隔（默认 6 小时）时跳过该板块，跳过的板块 MUST NOT 调用 LLM、MUST NOT 新写快照；跳过数量 SHALL 记入日志。**降级快照（`llm_score` 为空）不构成新鲜度**——降级轮已经证明本轮没产出依据，MUST NOT 把该板挡在窗口外，否则用户想手动重跑补依据会被守卫拦住。

**日报定时主轮尾步是当夜首轮、SHALL NOT 受新鲜度守卫约束**（主轮必须每夜产出当天的依据；若白天手动生成过导致主轮被跳过，当晚就没有当日依据）。因此「同一晚最多一轮 LLM」的保证范围是：主轮 + 回填 + 手动三条路径叠加后，回填与手动不会在主轮之后重复跑同一块板。

体检轮次 SHALL 有两级时间预算：整轮预算默认 30 分钟；每个板块另有独立的单板子超时（默认 6 分钟），单板超时按该板体检失败处理（日志记录耗时），MUST NOT 吃掉后续板块的预算。

手动端点与回填的触发 SHALL 异步执行（不阻塞 HTTP 响应、不影响主 job 返回值），panic MUST 被隔离并只记日志。

扩充建议的生成 MUST 保持用户手动触发；手动触发时候选来源见 board-upgrade-expand 能力的扩充候选召回要求。

#### Scenario: 体检在日报后自动执行
- **WHEN** 当日日报主轮生成流程完成且无失败版面
- **THEN** 体检轮次 SHALL 自动开始执行（回填与手动生成两条路径见下方 scenario）

#### Scenario: 手动生成日报后也能触发体检
- **WHEN** 用户经 `POST /api/daily-reports/generate` 为某板块生成日报成功，且该板块最近 6 小时内没有体检快照
- **THEN** 系统 SHALL 异步为该板块启动一轮体检，HTTP 响应 MUST NOT 等待体检结束

#### Scenario: 回填补出日报后触发体检
- **WHEN** 定时 job 的主轮有失败版面、回填阶段补出了至少一份日报
- **THEN** 系统 SHALL 在回填完成后异步触发一轮体检（新鲜度守卫照常生效；日报 job 的返回 MUST NOT 等待体检结束）

#### Scenario: 降级快照不把板块算成新鲜
- **WHEN** 某板块 10 分钟前刚体检过但那一轮降级（`llm_score` 为空），用户随后手动生成日报
- **THEN** 新鲜度守卫 SHALL 不把该快照算数，体检 SHALL 真跑一轮（为这块板重新争取依据）

#### Scenario: 主轮尾步不受新鲜度守卫约束
- **WHEN** 同一板块白天已经手动触发过体检（已有带评分快照），当晚日报主轮成功
- **THEN** 主轮尾步 SHALL 照常为该板块跑一轮体检（每夜必须有当日依据），MUST NOT 因守卫跳过整个夜晚

#### Scenario: 新鲜度守卫避免重复体检
- **WHEN** 同一板块 6 小时内已有体检快照，随后又手动生成了一次日报
- **THEN** 该板块 SHALL 被跳过且不调用 LLM，只记一条跳过日志

#### Scenario: 单板超时不影响其它板块
- **WHEN** 某板块体检在单板子超时（6 分钟）内未完成
- **THEN** 该板块记一条失败日志（含耗时），体检轮次 SHALL 继续处理其余板块

#### Scenario: 体检不改匹配数据
- **WHEN** 体检轮次完成
- **THEN** 匹配结果表 MUST NOT 出现任何由体检引起的写入

#### Scenario: 扩充建议仍需手动触发
- **WHEN** 体检产出大量未收编候选
- **THEN** 扩充建议 MUST NOT 自动生成；SHALL 等待用户在扩充入口手动触发

### Requirement: 体检数据查询端点只读

<!-- from: board-health-checkup (2026-10-10 add-board-health-checkup) -->


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

<!-- from: board-health-checkup (2026-10-10 add-board-health-checkup) -->


体检 SHALL 为每个活跃版块的每个构成标签计算四类确定性分诊信号，与贴合度统计同轮生成、同一纯读口径（不重算匹配、不筛样本）：

1. **磁铁信号**：构成标签 embedding 对「direction_mismatch=true 的失败挂载子集」与「direction_mismatch=false 的健康挂载子集」的贴合度均值差——**失败子集均值显著高于健康子集均值**（差值达配置阈值）且两侧样本量各自达标时，该标签标记磁铁嫌疑（语义：「磁铁把跑题的挂载吸了过来」）；任一侧样本不足或向量缺失时信号 MUST 标记 insufficient，MUST NOT 按差值硬凑。全体挂载均值（all 侧）SHALL 继续产出供展示与历史兼容，但 MUST NOT 参与嫌疑判定。
2. **最近邻信号**：构成标签在全库 active 辅助标签向量空间中的 top-3 最近邻（排除自身）及其 cosine 值，作为窄化/漂移判断证据随信号输出；标签自身 embedding 缺失时该信号 insufficient。
3. **描述污染预筛**：标签 description 命中文章式词根（「此次/本次/该事件/报道/针对/期间/近日」等）时标记事件性嫌疑；description 为空不标记。
4. **核心主体标记**：构成标签名出现在该版块的 description 或 aliases 中时，该标签 SHALL 标记为核心主体（`core_subject`）——核心主体是版块自身要谈论的对象，MUST NOT 作为摘除或重写对象。**仅当标签名出现在版块标题中时**，核心主体成立还 SHALL 要求该标签名长度 ≥ 3 个字符（2 字短词既可能是简称也可能是泛化地域词，仅凭标题不足以判定，MUST NOT 据此推断全名或泛化词为核心主体）。本判定 SHALL 为纯文本字面匹配（空白与大小写归一后子串），MUST NOT 使用向量相似度或语义推断。

信号 SHALL 作为 LLM 分诊的证据输入随统计快照持久化；信号计算本身 MUST NOT 产生任何写库动作。

构成标签清单 SHALL 逐条携带标签类型（`label_type`：辅助标签 `auxiliary` / 组合标签 `composite`），供消费方区分可执行与只读的分诊项；历史快照缺该字段时消费方 SHALL 按「类型未知」处理，MUST NOT 臆造类型。磁铁信号 SHALL 携带健康子集均值与样本数；核心主体标记为布尔字段，历史快照缺该字段时消费方 SHALL 按「非核心主体」处理，MUST NOT 臆造。

#### Scenario: 磁铁信号失败子集贴合差成立
- **WHEN** 构成标签「X」对 284 个方向通过的挂载 tag 贴合度均值为 0.52、对 40 个 direction_mismatch=true 的失败子集均值为 0.72、两侧样本量均达标
- **THEN** 「X」的磁铁信号 SHALL 成立（失败子集均值显著高于健康子集均值且差值达阈值），并携带两侧均值与样本数

#### Scenario: 核心主体与健康挂载贴合度高时不判磁铁嫌疑
- **WHEN** 构成标签「伊朗」对 284 个方向通过的挂载 tag 均值为 0.70、对 19 个 direction_mismatch=true 的失败子集均值为 0.54
- **THEN** 「伊朗」的磁铁信号 MUST NOT 成立（差值方向相反），MUST NOT 因该信号被判为摘除或重写对象

#### Scenario: 健康子集样本不足不强判
- **WHEN** 版块方向通过的挂载 tag 仅 3 个（低于最小样本量），失败子集有 20 个
- **THEN** 全部构成标签的磁铁信号 SHALL 标记 insufficient，不给出嫌疑结论

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

#### Scenario: 核心主体按版块文本判定
- **WHEN** 版块 description 含「伊朗为核心主体」，构成标签含「伊朗」，版块标题为「中东地缘政治与美伊关系」
- **THEN** 「伊朗」SHALL 标记 `core_subject=true`（description 命中即成立），MUST NOT 因标题只出现简称而漏标

#### Scenario: 简称与泛化词不据标题推断为核心主体
- **WHEN** 版块标题为「中东地缘政治与美伊关系」，构成标签含「伊朗」，版块 description 为空
- **THEN** 「伊朗」MUST NOT 标记 `core_subject=true`（标题命中的是简称「美伊」，MUST NOT 据简称推断全名），该板的核心主体保护依赖版块 description 明示主体

### Requirement: 分诊动作边界

<!-- from: board-health-checkup (2026-10-10 add-board-health-checkup) -->


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

<!-- from: board-health-checkup (2026-10-10 add-board-health-checkup) -->


完整依据 SHALL 默认以紧凑问题清单展示需核对的 remove/rewrite 标签，选择一项后只展开该项证据、动作与下一步，不再按四类分别纵向堆卡片。keep/insufficient 与统计依据 SHALL 按需展开且保留原含义；操作提交、部分完成、全部匹配更新完成和效果待验证 MUST 明确区分。

磁铁证据的对比呈现 SHALL 使用「失败子集 vs 健康子集」口径（与信号判定口径一致）；历史快照缺健康子集均值时 SHALL 回退旧口径展示并显式标注其为旧口径快照，MUST NOT 让用户误以为旧数字与新口径等价。核心主体标签 SHALL 在索引与详情中带可识别的标记。

当本轮分诊因合理性闸门被降级（可执行占比超限）时，界面 SHALL 显示醒目提示说明本轮信号可能不可用、不得按此批量操作；降级后的标签 SHALL NOT 出现在需核对索引中，且 MUST NOT 提供摘除或重写入口。

#### Scenario: 先处理问题而非遍历四组卡片
- **WHEN** 快照含 remove、rewrite、keep、insufficient 四类结果
- **THEN** 默认显示待核对问题索引与当前项处理区；保留/不足及统计依据可展开，所有结果仍可达

#### Scenario: 修复完成不冒充体检验证
- **WHEN** 用户确认的描述修复任务已完成匹配更新
- **THEN** 界面 SHALL 引导处理下一项并标明效果待下一轮体检验证，MUST NOT 更新旧快照评分或声称体检已恢复健康

#### Scenario: 磁铁对比使用健康子集口径
- **WHEN** 标签磁铁信号携带健康子集均值与失败子集均值
- **THEN** 界面 SHALL 以「失败子集 X vs 健康子集 Y」呈现该对比，SHALL NOT 再以含失败行的全体均值作对比

#### Scenario: 旧快照回退旧口径并标注
- **WHEN** 历史快照的磁铁信号缺健康子集均值
- **THEN** 界面 SHALL 回退展示旧口径对比并标注其为旧口径快照，MUST NOT 静默展示旧数字

#### Scenario: 核心主体标记可见
- **WHEN** 构成标签被标记为核心主体
- **THEN** 界面 SHALL 在该标签的索引条目与详情处显示核心主体标记

#### Scenario: 闸门降级时提示信号不可用
- **WHEN** 快照记录本轮分诊因可执行占比超限被降级
- **THEN** 界面 SHALL 显示提示说明本轮信号可能不可用、不得按此批量操作，且被降级的标签 MUST NOT 出现在需核对索引中

### Requirement: 分诊合理性闸门

<!-- from: board-health-checkup (2026-10-10 add-board-health-checkup) -->


LLM 给出的分诊清单在持久化与呈现前 SHALL 经规则层闸门校验；闸门 MUST NOT 依赖 LLM 自觉，只依据确定性信号与版块文本判定。

闸门 SHALL 包含三条，顺序执行、彼此独立：

1. **核心主体拦截**：被标记为核心主体的标签若被判为建议摘除或建议重写，系统 SHALL 在持久化前改判为 🟢 保留澄清，证据 SHALL 标注「核心主体保护」并保留原判内容可追溯，MUST NOT 以可执行建议呈现；改判后 MUST NOT 携带重写建议文案。
2. **摘除证据校验**：被判为建议摘除的标签，若其磁铁信号不成立或信号不足，系统 SHALL 在持久化前将其降级为 ⚪ 证据不足并说明原因，MUST NOT 以建议摘除呈现。
3. **可执行占比闸门**：当建议摘除与建议重写的合计条数占分诊清单总条数的比例超过配置阈值（默认 80%），系统 SHALL 在持久化前把全部可执行项降级为 ⚪ 证据不足，并在快照记录闸门结果（可执行条数、总数、占比、阈值、是否降级、原因）；降级 MUST NOT 丢弃分诊证据本身，MUST NOT 改写评分与评语。

闸门记录中的**可执行条数与占比 SHALL 取自 LLM 原始裁决**（核心主体拦截与摘除证据校验生效之前的状态），以如实反映「本轮模型有多激进」；若取降级后的状态，占比会随前两道闸门的处理结果漂移，MUST NOT 使闸门记录失真。

闸门降级后的标签 MUST NOT 出现在需核对的索引中，MUST NOT 提供摘除或重写入口。闸门阈值 SHALL 可经配置调整，缺省值 SHALL 有固定默认。

#### Scenario: 核心主体被判可执行时规则层改判保留
- **WHEN** LLM 将核心主体标签「伊朗」判为建议重写并给出新描述文案
- **THEN** 持久化前该条 SHALL 改判为 🟢 保留澄清，证据 SHALL 标注「核心主体保护」并保留原判，MUST NOT 以可执行建议呈现，MUST NOT 携带新描述文案

#### Scenario: 摘除建议必须有成立的磁铁信号
- **WHEN** LLM 判某标签为建议摘除，但该标签的磁铁信号不成立（或信号不足）
- **THEN** 该条 SHALL 在持久化前降级为 ⚪ 证据不足并注明原因，MUST NOT 以建议摘除呈现

#### Scenario: 可执行占比超限触发整轮降级
- **WHEN** 版块 47 个构成标签中 47 个被判建议摘除或建议重写（LLM 原始可执行占比 100%，超过阈值）
- **THEN** 全部可执行项 SHALL 在持久化前降级为 ⚪ 证据不足，快照 SHALL 记录闸门结果（可执行条数 47、总数 47、占比 1.0、阈值 0.8、是否降级 true、原因）

#### Scenario: 闸门占比取 LLM 原始裁决而非降级后状态
- **WHEN** LLM 原始裁决中 47 个构成标签全部被判建议摘除，但其中多数的磁铁信号不成立（会被摘除证据校验降级）
- **THEN** 快照记录的闸门占比 SHALL 仍为 1.0（取原始裁决），MUST NOT 因前序闸门降级而记为 0

#### Scenario: 占比未超限不降级
- **WHEN** 某版块 47 个构成标签中 13 个被判保留澄清、34 个被判建议重写（可执行占比 72%，低于阈值）
- **THEN** 分诊 SHALL 原样持久化，MUST NOT 被闸门降级

#### Scenario: 降级项不提供修复入口
- **WHEN** 某分诊项被闸门降级为证据不足
- **THEN** 界面 MUST NOT 为其提供摘除或重写入口，MUST NOT 出现在需核对索引中

#### Scenario: 闸门不影响评分与评语
- **WHEN** 闸门把本轮可执行项全部降级为证据不足
- **THEN** 快照的评分与评语 SHALL 照常保留，MUST NOT 因闸门降级被清空

### Requirement: Direction check for non-direct_hit matches

<!-- from: board-direction-check (2026-05-29 board-direction-check-and-board-editing) -->


对每个匹配成功的 tag-board 结果（composite_hit 除外），系统 SHALL 按以下次序执行方向判定，未获豁免的挂载任一门触发即标记 `direction_mismatch=true`：

0. **全库 gap 豁免**：若挂载版块的 identity cosine 与该 tag 对**全部 active 版块**（`semantic_labels` WHERE label_type='board' AND status='active'，不限本次匹配候选）identity cosine 最大值的差值 < DirectionGapExemptMargin（默认 0.10；差值 < 1e-9 的并列最优恒在此集合内），则豁免绝对门与相对门，`direction_mismatch=false`——长事件 tag × 短板名 embedding 的 cosine 全挤窄带（实证 0.55-0.65）时全库第一名常被语义重叠的磁铁板（如「中国国内新闻」）抢走，真家排 2-5 名的窄带差距是排序噪声不是跑题。若 DirectionFloor > 0 且该 cosine < DirectionFloor，豁免不生效（地板线优先）。
1. **绝对门**：未获豁免的挂载，cosine(tag identity embedding, board embedding) < DirectionSimThreshold（默认 0.5，ai_settings 现值 0.6）→ 标记。
2. **相对门**：未获豁免的挂载，gap = bestCos − cosine(tag identity embedding, board embedding) ≥ DirectionRelativeMargin（默认 0.15）→ 标记。bestCos 为该标签**本次匹配成功的全部版块**（含同轮其他候选，含将被标 mismatch 的候选）中 identity cosine 的最大值。

规则：
- composite_hit 豁免：不执行方向校验，`direction_mismatch` 恒为 false（组合命中即指向一致）。
- direct_hit 同样执行上述判定（与间接规则一致）。
- 全库豁免参照系 MUST 覆盖本次匹配候选之外的 active 版块（垃圾单挂的真家常在候选外）。
- 豁免与各门仅消费 identity cosine，不影响 score；DirectionGapExemptMargin ≤ 0 时回退为「差值 < 1e-9 的并列最优」豁免（即本 change 前行为）。
- tag identity embedding 缺失时跳过全部判定，`direction_mismatch=false`；board embedding 缺失的版块不进入全库参照系，挂载版块自身 embedding 缺失时跳过对应门。

#### Scenario: near-best mount within gap margin exempted
- **WHEN** tag「高市早苗在众议院预算委员会发表个人家务诉苦言论」挂载版块「日本新闻」cosine=0.581，全库最优为「中国国内新闻」cosine=0.647（gap 0.066 < DirectionGapExemptMargin=0.10）AND DirectionSimThreshold=0.6
- **THEN** direction_mismatch=false（gap 豁免——窄带内磁铁板压制的真家挂载不算跑题）

#### Scenario: mount beyond gap margin still flagged
- **WHEN** tag「Anthropic推出Claude Opus 5.5模型」挂载版块「全球科技巨头动态」cosine=0.597，全库最优为「生成式AI与大模型厂商」cosine=0.811（gap 0.214 ≥ 0.10）AND cosine < 0.6
- **THEN** direction_mismatch=true（真家明显更近的次优挂载，绝对门拦截）

#### Scenario: rank-1 mount always within gap margin
- **WHEN** 挂载版块即全库 cosine 最大者（差值 0）
- **THEN** direction_mismatch=false（恒在豁免集合内，与 margin 取值无关）

#### Scenario: gap margin disabled falls back to tie-only exemption
- **WHEN** DirectionGapExemptMargin ≤ 0 AND 挂载版块非并列最优 AND cosine < 0.6
- **THEN** 回退本 change 前行为：绝对门照常标记（只有差值 < 1e-9 的并列最优获豁免）

#### Scenario: floor revokes gap exemption
- **WHEN** 挂载 gap < margin 获豁免 AND DirectionFloor > 0 AND cosine < DirectionFloor
- **THEN** 豁免不生效，回退双门判定

#### Scenario: hit_rate match passes direction check
- **WHEN** tag matches board via hit_rate rule AND（gap 豁免生效 OR（cosine >= DirectionSimThreshold AND 相对门 gap < margin））
- **THEN** direction_mismatch=false, match recorded normally

#### Scenario: weighted match fails direction check
- **WHEN** tag matches board via weighted rule AND 未获豁免 AND cosine < DirectionSimThreshold
- **THEN** direction_mismatch=true, match still recorded with original score

#### Scenario: hit_rate match fails direction check
- **WHEN** tag matches board via hit_rate rule AND 未获豁免 AND cosine(tag_identity_embedding, board_embedding) < DirectionSimThreshold
- **THEN** direction_mismatch=true, match still recorded with original score

#### Scenario: relative gate catches multi-mount cross-domain leak
- **WHEN** tag 同轮匹配版块 A（cosine 0.81）与 B（cosine 0.60）AND 两处均未获全库 gap 豁免 AND DirectionRelativeMargin=0.15
- **THEN** B 的 direction_mismatch=true（相对门 gap 0.21 拦截），A 正常

#### Scenario: legal dual-mount within margin survives
- **WHEN** tag 同轮匹配版块 A（cosine 0.80）与 B（cosine 0.68），gap 0.12 < DirectionRelativeMargin
- **THEN** A、B 均 direction_mismatch=false（相对门内合法双挂）

#### Scenario: single-board mount unaffected by relative gate
- **WHEN** tag 本次仅匹配一个版块（未获全库豁免）AND cosine < DirectionSimThreshold
- **THEN** 绝对门照常标记；相对门 gap≡0 恒不触发

#### Scenario: composite_hit exempt
- **WHEN** tag matches board via composite_hit rule
- **THEN** 不执行方向校验，direction_mismatch=false（组合命中即指向一致）

#### Scenario: tag or board embedding unavailable
- **WHEN** tag identity embedding is NULL OR board embedding is NULL
- **THEN** 对应门与豁免跳过，direction_mismatch=false

#### Scenario: direct_hit match
- **WHEN** tag matches board via direct_hit rule AND（未获 gap 豁免 AND 任一门触发）
- **THEN** direction_mismatch=true（direct_hit 与间接规则同样校验）

#### Scenario: global-best mount exempted despite low absolute cosine
- **WHEN** tag「日本首相高市早苗任命武部新接任农林水产大臣」挂载版块「日本新闻」AND cosine(tag_identity, 日本新闻)=0.5826 为该 tag 对全部 active 版块 identity cosine 的最大值 AND DirectionSimThreshold=0.6
- **THEN** direction_mismatch=false（全库最优挂载恒在 gap 豁免集合内，挂载计入日报）

#### Scenario: non-best mount below threshold still flagged
- **WHEN** tag 挂载版块 B，cosine(tag_identity, B)=0.55 < 0.6，且存在 active 版块 A（可能不在本次匹配候选内）cosine(tag_identity, A)=0.82 更高（gap 0.27 ≥ DirectionGapExemptMargin=0.10）
- **THEN** 挂载 B 的 direction_mismatch=true（未获 gap 豁免，绝对门照常拦截）

#### Scenario: tied global best also exempted
- **WHEN** tag 挂载版块 A 与 B 的 identity cosine 并列为全部 active 版块中最大值（均 0.58 < threshold 0.6）
- **THEN** 两处挂载均 direction_mismatch=false（并列最优差值 0 恒在豁免集合内，不算跑题）

#### Scenario: global best reference spans beyond this run's candidates
- **WHEN** tag 本次仅匹配上榜版块 B（唯一候选，相对门 gap=0），但全库 active 版块中 A 的 identity cosine 更高（A 因 aux 交集为空未进入本次候选）且差值 ≥ DirectionGapExemptMargin
- **THEN** B 未获全库 gap 豁免；若 cosine(tag_identity, B) < DirectionSimThreshold 则 direction_mismatch=true

#### Scenario: single-board mount exempt when globally best
- **WHEN** tag matched only board B AND cosine(tag_identity, B)=0.55 < DirectionSimThreshold=0.6 AND B is the tag's global best board
- **THEN** direction_mismatch=false（单挂且全库最优，豁免生效）

### Requirement: DirectionSimThreshold configurable

<!-- from: board-direction-check (2026-05-29 board-direction-check-and-board-editing) -->


DirectionSimThreshold 通过 `ai_settings` 表配置，key 为 `semantic_board_match_direction_sim_threshold`，默认 0.5。与现有 12 个匹配参数一致的管理方式。

#### Scenario: custom threshold
- **WHEN** ai_settings contains `semantic_board_match_direction_sim_threshold` with value 0.6
- **THEN** DirectionSimThreshold=0.6 used for direction check

#### Scenario: no custom threshold
- **WHEN** ai_settings does not contain the key
- **THEN** default 0.5 used

### Requirement: Daily report excludes direction_mismatch tags

<!-- from: board-direction-check (2026-05-29 board-direction-check-and-board-editing) -->


collectBoardTags 查询排除 direction_mismatch=true 的标签，包括主查询和 fallback 补算。

#### Scenario: direction_mismatch tag excluded from daily report
- **WHEN** tag has direction_mismatch=true in topic_tag_board_labels for target board
- **THEN** tag is NOT included in daily report for that board

### Requirement: Board articles API direction_mismatch filtering

<!-- from: board-direction-check (2026-05-29 board-direction-check-and-board-editing) -->


getBoardArticles 的 filtered_tags 查询默认排除 direction_mismatch=true 的标签。通过 query param `?show_direction_mismatch=true` 可包含。

#### Scenario: default request
- **WHEN** getBoardArticles called without show_direction_mismatch param
- **THEN** filtered_tags excludes direction_mismatch=true tags

#### Scenario: explicit include
- **WHEN** getBoardArticles called with show_direction_mismatch=true
- **THEN** filtered_tags includes all tags including direction_mismatch=true

### Requirement: Match detail API returns direction_sim

<!-- from: board-direction-check (2026-05-29 board-direction-check-and-board-editing) -->


getTagMatchDetail 响应新增 `direction_sim` 字段（float64），为 tag identity embedding 与 board embedding 的 cosine 值。无 embedding 时返回 null。

#### Scenario: direction_sim available
- **WHEN** both tag identity embedding and board embedding exist
- **THEN** response includes direction_sim value

#### Scenario: direction_sim unavailable
- **WHEN** tag or board embedding is NULL
- **THEN** response direction_sim is null/omitted

### Requirement: DirectionRelativeMargin configurable

<!-- from: board-direction-check (2026-05-29 board-direction-check-and-board-editing) -->


DirectionRelativeMargin SHALL 通过 `ai_settings` 表配置，key 为 `semantic_board_match_direction_relative_margin`，默认 0.15。设为 0 时相对门 MUST 关闭，回退为仅绝对门校验。与现有匹配参数一致的管理方式。

#### Scenario: custom margin
- **WHEN** ai_settings contains `semantic_board_match_direction_relative_margin` with value 0.18
- **THEN** relative gate uses margin=0.18 (gap >= 0.18 flags mismatch)

#### Scenario: margin zero disables relative gate
- **WHEN** ai_settings contains `semantic_board_match_direction_relative_margin` with value 0
- **THEN** only the absolute gate applies

#### Scenario: no margin row
- **WHEN** ai_settings does not contain the key
- **THEN** default 0.15 is used

### Requirement: Direction floor configurable

<!-- from: board-direction-check (2026-05-29 board-direction-check-and-board-editing) -->


DirectionFloor SHALL 通过 `ai_settings` 表配置，key 为 `semantic_board_match_direction_floor`，默认 0（关闭）。值 > 0 时作为全库最优豁免的地板线：豁免仅对 cosine ≥ DirectionFloor 的挂载生效；cosine < DirectionFloor 的全库最优挂载豁免不生效，SHALL 回退绝对门与相对门判定（floor 是豁免的下限保险丝，不是第二把绝对刀）——防御「哪都不像却硬挂」的未来形态（当前真库全库最优挂载最低 cosine 0.515 且成色正确，默认关闭无操作量）。与现有匹配参数一致的管理方式。

#### Scenario: floor disabled by default
- **WHEN** ai_settings does not contain `semantic_board_match_direction_floor`
- **THEN** floor=0，全库最优豁免无额外下限

#### Scenario: floor overrides exemption
- **WHEN** ai_settings contains `semantic_board_match_direction_floor` with value 0.55 AND a mount is the tag's global best AND cosine=0.515
- **THEN** direction_mismatch=true（0.515 < floor 0.55，豁免不生效，绝对门照常判定）

### Requirement: DirectionGapExemptMargin configurable

<!-- from: board-direction-check (2026-05-29 board-direction-check-and-board-editing) -->


系统 SHALL 支持通过 ai_settings 键 `semantic_board_match_direction_gap_exempt_margin` 配置全库 gap 豁免的差值门槛：

- 默认 0.10；取值 ≤ 0 表示关闭 gap 扩围（回退并列最优豁免）。豁免带有效带宽 = max(DirectionGapExemptMargin, 1e-9)——亚噪声 margin（0 < margin < 1e-9）实际生效 1e-9（与代码 math.Max 归一一致）。
- 配置加载进 `LoadConfig` 白名单，与 `direction_relative_margin` / `direction_floor` 同管道（DB-only 键，不经 matching-config API 白名单；解析语义复用 margin 类 parse：负值归 0 关闭，越界（> 1.0）或非法值拒绝并保持默认 0.10）。

#### Scenario: custom margin applied
- **WHEN** ai_settings 设置 `semantic_board_match_direction_gap_exempt_margin=0.15` AND 挂载 gap=0.12
- **THEN** 豁免生效（0.12 < 0.15）

#### Scenario: default margin without setting
- **WHEN** ai_settings 无该键
- **THEN** DirectionGapExemptMargin=0.10

#### Scenario: invalid margin rejected
- **WHEN** ai_settings 设置该键为 1.5 或非数值
- **THEN** 配置拒绝，保持默认 0.10（margin 类 parse 语义：fallback 恒指本次加载的默认值，与 relative_margin/floor 同继承语义）

### Requirement: Direction check data loading

<!-- from: board-direction-check (2026-05-29 board-direction-check-and-board-editing) -->
<!-- kind: impl -->


MatchTopicTag 加载 tag identity embedding（`topic_tag_embeddings` WHERE embedding_type='identity'）和所有活跃 board embedding（`semantic_labels` WHERE label_type='board'），传入 evaluateSemanticBoardMatches。embedding 缺失时不阻塞匹配流程。

#### Scenario: tag has no identity embedding
- **WHEN** tag has no identity embedding in topic_tag_embeddings
- **THEN** tagEmbedding passed as nil, direction check skipped for all max_sim matches

#### Scenario: board has no embedding
- **WHEN** a board's semantic_labels.embedding is NULL
- **THEN** that board is excluded from boardEmbeddings map, direction check skipped for that board

### Requirement: direction_mismatch persisted

<!-- from: board-direction-check (2026-05-29 board-direction-check-and-board-editing) -->
<!-- kind: impl -->


topic_tag_board_labels 新增 `direction_mismatch BOOLEAN NOT NULL DEFAULT false` 列。replaceTopicTagBoardLabels 写入时包含此字段。

#### Scenario: persist direction mismatch
- **WHEN** evaluateSemanticBoardMatches returns a match with DirectionMismatch=true
- **THEN** topic_tag_board_labels.direction_mismatch=true for that record
