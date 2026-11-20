# related-events Specification

## Purpose

为阅读事件标签的用户提供基于名称、描述与辅助标签语义向量的直接相关事件推荐。不要求统一主体身份，不归并事件，不推断阶段或发酵；日报 thread 仅是叙事引用，不承担此能力的事件身份。

## Requirements

### Requirement: 查看相关事件

系统 SHALL 以 event 标签为输入，使用名称、描述和已有辅助标签的确定性文本 embedding，在同版块有效可见 event 标签中找相关内容。缺辅助标签 SHALL 允许降级为名称与描述，SHALL NOT 要求 subject_key、实体 ID、指纹信任或 thread 向量。

候选 SHALL 为请求开始时间前30×24小时内创建的同版块有效标签，排除自身、已合并及仅mismatch入板的标签；窗口含两端。结果 SHALL 按 cosine>=实际阈值过滤后，按分数降序、ID升序最多返回10条。阈值前 SHALL NOT 用无关排序截断候选。时间表达的是标签收录时间，SHALL NOT 冒充事件发生时间。

结果 SHALL 只表达直接相关，不修改任何标签/文章/事件线身份、归属、辅助别名或旧热度，不传递聚类。新关联向量 SHALL 与旧业务向量隔离，文本版本或实际模型空间变更后旧向量 SHALL 不参与计算。GET SHALL 不发模型调用、不写业务或缓存；索引由显式刷新操作生成，失败可重试。

API SHALL 返回实际阈值、窗口、向量版本、候选数与已就绪数，以及 ready/partial/not_ready；缺索引 SHALL NOT 伪装成无结果，服务错误 SHALL 明确反馈。界面 SHALL 在既有event标签卡中按需展开名称、描述、相似度与可用文章来源，不新增阶段/发酵状态，不阻塞原阅读。

#### Scenario: 文本表示稳定且辅助标签可缺失
- **WHEN** 相同event标签的辅助标签顺序或重复项变化，或该标签没有辅助标签
- **THEN** 顺序/重复变化 SHALL 不改变规范化文本与签名；缺辅助标签 SHALL 仍使用名称与描述；空白名称 SHALL 标记不可索引而不生成虚构向量

#### Scenario: 关联不依赖旧身份字段
- **WHEN** 两个语义相关event标签的subject_key不同、实体ID缺失、指纹不可信，或只被watch/composite线索引用
- **THEN** 上述字段和引用形态 SHALL 不否决相关查询，event标签在满足版块、窗口、索引与阈值条件时仍可返回

#### Scenario: 同版块窗口与稳定排序
- **WHEN** 存在相同分数、恰好达到阈值、恰在窗口边界、跨版块、自身及mismatch候选
- **THEN** 系统 SHALL 先过滤业务范围和有效空间，再按阈值及稳定排序取最多10条；阈值及时间边界 SHALL 包含，跨版块、自身与mismatch SHALL 不返回

#### Scenario: 相关连接不改身份也不传递
- **WHEN** A达到与B的相似阈值、B达到与C的阈值，而A与C未达到阈值
- **THEN** A的查询 SHALL 不因B返回C，所有标签、thread归属、旧事件线、辅助别名和热度 SHALL 保持原值

#### Scenario: 索引未就绪与依赖失败
- **WHEN** 源向量未就绪、部分候选缺向量、结果确为空，或索引刷新模型/查询服务失败
- **THEN** 系统 SHALL 分别反馈not_ready、partial、ready空集或明确错误；失败刷新 SHALL 可重试且不改源数据；GET SHALL 不为补齐索引发起模型调用

#### Scenario: 模型空间或文本变更后旧向量失效
- **WHEN** 标签名称/描述/辅助标签变化，或模型空间/表示版本变化（含同维更换模型）
- **THEN** 不匹配当前签名的向量 SHALL 不参与相似计算，直到显式刷新成功；重跑相同版本刷新 SHALL 复用已有效缓存而非重复计算

#### Scenario: 相关列表复用既有阅读入口
- **WHEN** 用户在归档文章的event标签卡展开相关事件并查看某条来源，或切换标签时旧请求迟到
- **THEN** 界面 SHALL 展示接口排序、原标签名称/描述和相似度，通过既有文章预览打开对应来源；无来源 SHALL 明示且不可伪造；迟到响应 SHALL 不覆盖新标签，非event卡 SHALL 不发相关查询

### Requirement: 在线调整相关阈值

系统 SHALL 通过既有匹配配置GET/PUT和前端匹配规则提供 `related_event_similarity_threshold`，默认0.86（2026-11-10 S3 实测后由 0.82 上调），有效值为[0,1]有限数。设置 SHALL 与旧板匹配阈值、距离与min3配置独立；保存后新请求 SHALL 使用新值，不要求重算向量。非法提交 SHALL 拒绝且不覆盖原设置；存量缺项 SHALL 回退默认，非法存量 SHALL 告警并回退默认。

#### Scenario: 阈值保存后新查询生效
- **WHEN** 用户在匹配规则中保存0.86，之后调高或调低再重新展开相关事件
- **THEN** 重开配置 SHALL 显示保存值，随后查询 SHALL 使用并返回新阈值，结果 SHALL 随阈值重新过滤且无需重新embed，其他匹配参数 SHALL 不变

#### Scenario: 非法阈值不覆盖配置
- **WHEN** 用户提交空白、非有限数、范围外值，或配置保存失败
- **THEN** 系统 SHALL 拒绝非法值且保留旧配置，界面 SHALL 保留输入并提示可重试；缺省值 SHALL 为0.86，合法边界0和1 SHALL 可保存

### Requirement: 历史样本验证相关性

正式交付前 SHALL 用AI版块历史event标签验证同一文本表示和阈值的相关性，不等影子运行一两周（2026-11-10 实测运行于起始值 0.82，实测后默认值上调为 0.86，历史数据原样留档不改写）。试验 SHALL 仅读取业务源数据，在内存或本地临时文件计算；正常模型调用审计可写，SHALL NOT 写相关索引缓存、业务身份、归属或合并关系。结果 SHALL 如实注明模型/表示/范围/覆盖、分数和人工相关性判定，不以旧跨天线距离或旧C1 min3结论替代。

#### Scenario: 历史样本验证相关而非同一事件
- **WHEN** 对AI版块1980近30天有效event标签运行历史试验，查看0.82下的结果及阈值附近样本
- **THEN** 输出 SHALL 可逐对审核“是否值得一起看”并记录未索引/失败情况；未实测 SHALL 明示待验证，不预填准确率，不重算任何thread或story身份，不推断阶段/发酵

### Requirement: 日报同事件聚簇

日报生成时，系统 SHALL 在**同一版块当天**的 event 标签范围内做同事件聚簇，作为「今日重点」的候选源。聚簇 SHALL 复用 related-events 的事件语义表示 `related-event-v1`（名称 + 描述 + 当前有效辅助标签）与同一相似度阈值 `related_event_similarity_threshold`（默认 0.86，有效值 `[0,1]`，缺项回退默认、坏值告警回退、非法提交拒绝且不覆盖），SHALL NOT 为聚簇另立表示或另立阈值。

聚簇 SHALL 为**当天口径**：候选集 SHALL 为该版块当天日报中命中的 event 标签（去重后），SHALL NOT 跨天、不引入前一日标签，SHALL NOT 做事件身份归并、合并、传递扩展或阶段判定。聚簇结果 SHALL 只在内存中计算，SHALL NOT 写入 `related_event_embeddings` 或任何业务表、缓存、身份表与关系表。

聚簇 SHALL 产生向量时走标准 AI 路由（capability 为 embedding），请求 SHALL 携带非空 operation（与索引刷新 operation 区分），每次调用尝试无论成败 SHALL 落 `ai_call_logs` 审计（operation / capability / token / latency / metadata；embedding 路径按仓内 N/A 惯例不记 prompt / response_snippet，截断保护只适用 chat 路径）。同一版块同一天的向量 SHALL 以批量调用取得（分批上限与单批超时沿用既有索引刷新口径），SHALL NOT 逐标签单独调用，也 SHALL NOT 为同一批标签重复计算。

每个聚簇 SHALL 给出：成员 event 标签集合、簇内文章数（按内容身份去重，见 `lane-heat-metric`「线索计数的内容身份去重口径」）、代表标签与描述。综合稿（`scope=composite`）的 thread SHALL NOT 进入聚簇，也不 SHALL NOT 计入簇内文章数。

向量不可得时，系统 SHALL 按「语义向量兜底 → 主体近似兜底」逐级降级，并 SHALL 如实标注本次聚簇口径（向量聚簇 / 语义向量兜底 / 主体近似），SHALL NOT 把不同口径的结果混为一谈，也 SHALL NOT 静默把降级结果呈现为向量聚簇。

#### Scenario: 同版块当天同事件聚成一簇

- **WHEN** 某版块当天的两个 event 标签语义相似度达到实际阈值
- **THEN** 二者 SHALL 归入同一聚簇
- **AND** 该簇 SHALL 给出成员标签集合与按内容身份去重后的簇内文章数

#### Scenario: 聚簇不跨天不做身份

- **WHEN** 某版块昨天的 event 标签与今天的某个标签语义高度相似
- **THEN** 昨天的标签 SHALL NOT 进入今天的候选集与聚簇
- **AND** 聚簇 SHALL NOT 产生事件身份、合并关系或阶段标记

#### Scenario: 聚簇不落库不改索引

- **WHEN** 日报生成完成聚簇
- **THEN** `related_event_embeddings` 与任何业务表 SHALL 保持写入前状态
- **AND** 既有 related-events 查询结果 SHALL 不受影响

#### Scenario: 批量调用并留审计

- **WHEN** 某版块当天有 N 个候选 event 标签
- **THEN** 聚簇 SHALL 以批量 embedding 调用取得向量（分批上限与单批超时沿用既有索引刷新口径），SHALL NOT 逐标签单独调用
- **AND** 每次调用 SHALL 携带非空 operation 并在 `ai_call_logs` 留下成功或失败记录

#### Scenario: 阈值在线可调且与相关事件同源

- **WHEN** 把 `related_event_similarity_threshold` 从 0.86 调到 0.9
- **THEN** 次日日报的聚簇 SHALL 使用 0.9 过滤
- **AND** 该键 SHALL 与相关事件查询共用同一生效值，不另设独立阈值

#### Scenario: 向量不可得时如实降级

- **WHEN** 聚簇所需的 embedding 调用失败或无可用路由
- **THEN** 系统 SHALL 降级到语义向量兜底，再不可得时降级到主体近似
- **AND** 本次聚簇 SHALL 标注实际口径，SHALL NOT 呈现为向量聚簇

#### Scenario: 综合稿不进聚簇

- **WHEN** 某 thread 的 `scope=composite`（命中 2 个及以上 event 标签）
- **THEN** 该 thread SHALL NOT 进入任何聚簇
- **AND** SHALL NOT 计入任何簇的簇内文章数
