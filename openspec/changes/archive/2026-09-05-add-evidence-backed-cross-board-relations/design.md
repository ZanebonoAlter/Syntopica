## Context

现有语义版块负责按相似度组织材料，不表示版块之间的因果或上下文关系；`board_brief` 只消费本版块态势卡并保持无工具的单次生成，`board_investigation` 则通过共享研究循环使用内部泳道、博查和正文抓取，但 `get_lane_detail` 白名单由父简报机械派生。博查已经以 `summary:false` 提供原始搜索命中，调查链也已有工具去重、允许工具守门、反证纪律、完整工具结果留存和不可变结果快照。

本 change 是跨数据增强编排、工具协议、持久化、异步任务、API 和前端的复杂改动。现有升级建议具备 pending/confirm/dismiss、幂等 hash 和 dismiss 冷却模式，可以复用其生命周期思想，但跨版块关系的证据、目标解析和过期语义不同，不直接复用同一张表。

## Goals / Non-Goals

**Goals:**

- 建立 source → 外部证据 → target concept → 内部目标的泛用发现流程，不依赖财经、地缘政治或固定版块规则。
- 将高召回的候选发现与高精度的独立验证分开，并允许 unresolved、insufficient 和 rejected。
- 让调查通过受审计的运行时授权安全读取跨版块泳道，同时保留幽灵 lane 防护。
- 让关系建议具备可核查证据、人工裁决、幂等、冷却和过期生命周期。
- 让确认关系在不改变简报“无联网工具、不可变快照”边界的前提下进入后续简报。
- 手动触发先可用，自动触发复用同一引擎且默认关闭、预算受控。

**Non-Goals:**

- 不构建通用因果推断或金融量化模型，不把时间领先、共现或相关系数设为全局判断规则。
- 不自动创建、合并或修改语义版块/泳道；目标解析只能引用现有内部对象或保持 unresolved。
- 不让 LLM 自动确认关系，也不回写历史简报、新闻 section 或 lifeline。
- 不在本 change 中回填历史关系，亦不建立全量版块两两扫描任务。
- 不把已确认关系混入现有简报 `relationships`；后者仍只描述本版块态势卡关系。

## Decisions

### D1. 采用证据优先的有向发现流水线，而不是版块对扫描

每次运行必须绑定一个不可变 source snapshot：`source_board_id`、父简报、source kind（observation/question）、稳定 source key 和原始文本。流水线分为：

1. **Scout**：基于 source 生成有限搜索计划，调用 `web_search`/`fetch_page`，从原始材料抽取候选关系陈述和 target concept；此阶段不选内部目标。
2. **Resolve**：服务端对 target concept 做内部混合检索，结合 label/description 的词法命中及已有 embedding/centroid 相似度返回 top-K。只有 top-1 达门槛且与 top-2 保持配置化安全间隔时解析为内部 target，否则保留候选列表并标为 unresolved。
3. **Verify**：使用独立 operation/session，只接收关系假设、原始证据、解析快照、目标内部材料、反证结果和替代解释；输出受限关系类型及验证 verdict。
4. **Persist**：rejected 只留在 run 快照；未解析或证据不足的可审阅候选落为 unresolved；supported 且目标已解析的关系最多落为 proposed，等待人工裁决。

这种 source→evidence→concept→target 路径将复杂度从 O(board²) 限制为每个 source 的预算化外部检索和 top-K 内部检索，并允许外部世界提到当前库中不存在的概念。

**替代方案：** 对所有版块做 embedding 两两匹配。该方案仍主要发现语义相似，不擅长发现共同驱动、分化或间接影响，并随版块数量平方增长，因此不采用。

### D2. 内部目标解析是保守的程序决策，不接受模型直接绑定 ID

解析器输出 `resolved | ambiguous | no_match`、候选目标、各组成分和版本化门槛。模型可以抽取 target concept 和说明，但不能直接提交最终 board/lane ID；最终 ID 必须来自解析器返回的真实对象。解析得分低、领先间隔不足、对象已归档或不存在时保持 unresolved。

时间先后、实体共现、数值相关等作为 `evidence_signals[]` 保存，由领域无关的验证 prompt 解释；它们不参与全局硬门槛。排序和 UI 质量等级由程序根据目标解析状态、可核查证据数、独立来源域名数、反证是否完成、验证 verdict 和 gap 计算，忽略模型自报 confidence。

**替代方案：** 让 LLM 从 `list_boards` 文本里直接挑目标。该方案难以稳定复现、无法给出明确的 ambiguous 边界，也容易强选，因此不采用。

### D3. 调查工具采用会话级动态 capability grant

增加紧凑的 `search_internal_context` 工具，并允许现有可信导航工具作为授权来源。每次研究循环创建 `DynamicLaneGrantSet`：

- 初始集合来自父简报泳道；
- `search_internal_context` 或 `list_lanes` 成功结果中的 lane IDs 由服务端 `AfterToolResult` 钩子解析并加入集合；
- `get_lane_detail` 在执行前由 `BeforeToolCall` 检查集合；
- 仅可信工具的结构化结果可授予权限，模型文本、网页内容和历史会话均不能授予；
- 每次 grant 记录 tool、step、board/lane 和原因，冻结到调查 input snapshot。

为避免影响 topic 分析和 QA，`runToolLoop` 的前置检查/结果观察接口保持可选；未注入动态策略的旧调用方行为不变。跨版块 lane 引用在综合 sanitize 时再次用 grant audit 校验，并补齐所属 board ID。

**替代方案：** 给调查开放全部 lane IDs。虽然实现简单，但会让提示词任意下钻全库并破坏现有幽灵 lane 防护，因此不采用。

### D4. 分离运行记录与关系生命周期记录

新增两类持久化实体：

1. `cross_board_relation_runs`
   - 保存 source snapshot、trigger kind、预算快照、queued/running/succeeded/partial/failed 状态、完整 tool calls、gaps、候选和错误；
   - 用于异步轮询与失败审计，不直接代表有效关系。
2. `cross_board_relations`
   - 保存 run/source 引用、source/target board 列、typed source/target refs、target concept、mapping snapshot、relation type、claim/mechanism、verification verdict、quality grade、evidence/counterevidence、status、suggestion hash、confirmed/dismissed/expired 时间和理由；
   - target board/lane 可空以表达 unresolved；查询 source/target board 使用普通列，细粒度引用和证据使用 JSONB。

稳定 hash 覆盖规范化 source key、解析目标或 target concept、关系类型、核心 claim 和证据版本。对 unresolved/proposed 建部分唯一索引；dismiss 后按 hash 冷却。`confirmed` 在 `expires_at` 到期后由读取路径立即视为无效，并由维护任务批量标记 expired，避免维护任务延迟造成错误注入。

**替代方案：** 把关系塞入 `topic_enrichment_result.sectors`。结果快照不可变且缺少独立裁决/过期查询能力，会迫使重写历史结果，因此不采用。

### D5. 验证器使用通用竞争解释和证据契约

验证器的枚举固定为：

- 类型：`causal | common_driver | divergence | correlated | contextual | unclear`
- verdict：`supported | contested | insufficient | rejected`

验证输入必须区分支持证据、反证、替代解释和 gap。证据引用必须指向本次持久化的原始 tool result，并通过保守文本核对；无法核对的 quote 不参与质量等级。网页内容以不可信数据块注入，明确禁止遵循网页中的指令，降低 prompt injection 风险。没有合格证据时不得产生 supported。

发现器和验证器使用不同 operation/session；验证器看不到发现器的自评分，只能看到假设、解析快照和材料。这种“盲验”降低确认偏误，同时复用现有调查的中性查询、反证和证据不足纪律。

**替代方案：** 一次 LLM 同时发现、选目标、打分和定论。该方案确认偏误强且无法定位失败阶段，因此不采用。

### D6. 关系确认是唯一生效边界

状态转换规则：

- Resolve/verify 不充分 → `unresolved`
- resolved + supported → `proposed`
- 用户操作：`proposed → confirmed | dismissed`
- 重新解析：`unresolved → unresolved | proposed`
- 到期：`confirmed → expired`

非法或重复转换返回冲突但保持幂等。确认时重新检查 source、target、证据版本和 expires_at，避免用户在陈旧详情页确认失效建议。裁决接口记录理由；dismiss 冷却复用升级建议模式但使用独立配置。

第一版不自动把 contested 提升为 proposed，也不自动确认任何关系。自动发现只改变候选产生方式，不改变生效边界。

### D7. 简报以独立、机械字段消费确认关系

`EnrichBoard` 在组装 prompt 前查询该 board 作为 source 或 target 的 confirmed 且未过期关系，按 `quality_grade DESC, confirmed_at DESC, id ASC` 排序并施加条数/字符预算。渲染为“已确认外部关系背景”，明确它不是本次态势卡事实；选中 ID、证据引用和截断数冻结进 input snapshot。

`board_brief.relationships` 的解析、lane 白名单和证据规则保持不变。新增 `cross_board_relations` 字段由服务端根据选中行机械装配，LLM 不生成它；这样既允许 summary/uncertainties 参考已确认背景，又不会让跨版块 lane 绕过当前 `parseBoardBrief` 的 active-card 防护。确认或过期只影响下一份简报，旧简报保持不可变。此过程不注册或调用 web 工具。

**替代方案：** 扩大现有 `relationships[].lane_ids` 接受其他版块泳道。这会混淆“本次卡片观察”和“历史确认关系”，破坏 ghost-lane 校验及 evidence_refs 语义，因此不采用。

### D8. 手动与自动触发共用异步运行器

新增 relation-discovery job kind，沿用现有版块异步任务的 202/409、轮询和视图 epoch 防串台模式：

- 手动 API 接收 parent brief ID、source kind 和 source key；服务端必须从父简报重取并验证 source，不能信任客户端上传的任意 source 文本。
- 自动入口在新 brief 成功持久化后、review 之外以 non-fatal 方式尝试 enqueue；版块未启用、预算为零或已有冲突任务时跳过并留痕，绝不回滚简报。
- 自动入口按稳定顺序选择预算内 observations；不处理全 sparse 简报，不从历史简报回填。
- 搜索次数、候选数、正文抓取数、总 loop 和超时全部冻结进 run budget snapshot，便于审计和重跑。

前端在现有版块增强面板增加“关系建议”区域和 observation/question 的“发现关联”动作，复用异步轮询与迟到响应守卫。列表默认展示 unresolved/proposed，详情展示映射依据、证据、反证、gap 和状态历史。

## Risks / Trade-offs

- **[外部搜索产生噪声或错误关系]** → 发现与验证分离、保守解析、证据原文核对、允许 insufficient/rejected，并以人工确认为唯一生效边界。
- **[网页中的 prompt injection 影响模型]** → 网页材料作为明确分隔的不可信数据注入；工具结果不能授予内部访问权限；最终引用还要机械核对。
- **[动态授权泄漏跨会话权限]** → grant set 仅存在单次 run，工具成功结果才可更新；执行前和综合后双重校验并持久化审计。
- **[博查调用成本和延迟上升]** → 自动发现默认关闭、异步执行、按版块预算；同 source/hash 幂等和 dismiss 冷却减少重复检索。
- **[目标版块重命名、归档或泳道迁移导致关系陈旧]** → 关系保存稳定 ID 和映射快照；消费时校验对象存在性，失败则排除并进入重新解析/过期流程。
- **[确认关系在语境变化后继续误导]** → 必填 expires_at、读取时即时判过期、UI 展示确认时间与证据时间，后续允许撤销或重新发现。
- **[简报 prompt 被外部关系挤占]** → 独立条数/字符预算、确定性排序和截断计数；全 sparse 简报仍不得把确认关系伪装成本期观察。
- **[新增状态和 UI 使首版范围较大]** → 后端流水线、手动入口、裁决与简报消费先形成纵向闭环；自动触发在同一引擎上以默认关闭配置接入，不另造第二套流程。

## Migration Plan

1. 通过现有迁移机制新增 relation run、relation lifecycle 表及索引；新增配置默认值时保持自动发现关闭。
2. 部署后先启用手动入口、动态授权和建议审阅，旧简报/调查/关系数据无需回填。
3. 在测试版块人工运行并核对 unresolved 比例、引用可追溯率、重复建议率和博查调用预算。
4. 只有确认手动链路稳定后，按版块显式开启自动发现；自动任务仍只产生建议。
5. 回滚时先关闭自动配置和入口；旧版应用忽略新表及简报新增 JSON 字段，既有数据继续工作。若保留表，已确认关系不会被旧版消费；无需破坏性删表。
