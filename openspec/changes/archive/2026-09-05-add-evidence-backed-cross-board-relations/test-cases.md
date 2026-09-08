# test-cases: add-evidence-backed-cross-board-relations

> 复杂档：关系生命周期（≥3 状态）、保守目标解析算法、运行时授权协议及跨后端/前端异步交互。以下每个故事锚定一个 Requirement；测试文件为计划落点，实施时按 tasks.md 对账。

## 故事 S1：用户从任意简报观察或问题发起预算受控的关系发现（锚 Requirement: 证据优先的关系候选发现）

### 主链路（节拍串联）
| 步 | 动作 | 来源 Scenario | 期望 | 层 | 落点（测试/人工） |
| --- | --- | --- | --- | --- | --- |
| 1 | 从 observation 发起任务 | 从观察手动发现 | 服务端重取来源并创建异步 run，不要求目标版块 | handler | `backend-go/internal/dataenrichment/handler/relation_discovery_handler_test.go` |
| 2 | 从 research question 发起任务 | 从研究问题手动发现 | 复用相同引擎且 source kind/key 可追溯 | handler | 同上 |
| 3 | 生成简报但自动开关为 false | 自动发现默认关闭 | 不联网、不入队、不产建议 | service | `backend-go/internal/dataenrichment/service/relation_auto_trigger_test.go` |
| 4 | 自动开关为 true 且 observation 超预算 | 自动发现遵守预算 | 稳定选择预算内来源并记录 skipped，不枚举版块对 | service | 同上 |

### 变体走查
| # | 变体（组/条目） | 期望答案 | 层 | 落点 |
| --- | --- | --- | --- | --- |
| 1 | 输入：空/纯空白 source key | 400，不创建 run | handler | handler test |
| 2 | 前置：父简报不存在、跨 board、source 已从简报消失 | 404/409，不信任客户端 source 文本 | handler | handler test |
| 3 | 幂等：重复提交 | 返回同一活跃 run 或 409，不双跑 | service/handler | handler test |
| 4 | 可用性：加载、失败、空建议 | UI 分别显示运行态、结构化错误、空态 | component | `front/app/features/tags/components/BoardRelationPanel.test.ts` |

### 效果核对
- 触发原因：实际发现效果依赖博查结果与 LLM 输出，mock 绿灯不能证明可用。
- 核对方法：在真实数据库选取至少 3 个不同业务域的 observation 手动运行，统计预算、搜索调用和候选。
- 量化结果：实施验收时记录；至少 3/3 run 可追溯 source，任何 run 均未扫描全量版块对。
- 结论：待 apply 实测填写。

## 故事 S2：系统保守地把外部概念解析回内部知识（锚 Requirement: 外部概念到内部目标的保守解析）

### 主链路（节拍串联）
| 步 | 动作 | 来源 Scenario | 期望 | 层 | 落点（测试/人工） |
| --- | --- | --- | --- | --- | --- |
| 1 | 解析唯一高分候选 | 唯一目标解析成功 | resolved，记录 ID、词法/向量分与门槛版本 | 函数单测 | `backend-go/internal/dataenrichment/service/relation_resolver_unit_test.go` |
| 2 | 解析两个接近候选 | 多个候选无法消歧 | ambiguous/unresolved，不强选 top-1 | 函数单测 | 同上 |
| 3 | 解析无命中概念 | 外部概念尚无内部目标 | no_match/unresolved，保留概念供重试 | 函数单测 | 同上 |

### 变体走查
| # | 变体（组/条目） | 期望答案 | 层 | 落点 |
| --- | --- | --- | --- | --- |
| 1 | 输入：空白、emoji、引号、超长 concept | 空白拒绝；其余规范化且字符预算截断 | 函数单测 | resolver unit |
| 2 | 前置：空库、单候选、重复候选、归档 lane | 分别 no_match、按门槛判定、去重、归档对象不可 resolved | 函数单测 | resolver unit |
| 3 | 边界：top-1 等于门槛、margin 等于门槛 | 明确按配置的包含边界判定并锁测试 | 函数单测 | resolver unit |
| 4 | 幂等：相同输入重复解析 | 候选排序和状态一致 | 函数单测 | resolver unit |

## 故事 S3：独立验证器可以拒绝发现器的第一印象（锚 Requirement: 发现与验证相互隔离）

### 主链路（节拍串联）
| 步 | 动作 | 来源 Scenario | 期望 | 层 | 落点（测试/人工） |
| --- | --- | --- | --- | --- | --- |
| 1 | 将假设、支持/反证和替代解释送入独立 session | 支持证据和反证共同进入验证 | 输入不含 scout 自评分 | service | `backend-go/internal/dataenrichment/service/relation_verifier_test.go` |
| 2 | 材料只支持共同第三方因素 | 共同驱动而非直接因果 | common_driver 或 contested，不得 causal | service | 同上 |
| 3 | 材料无法区分解释 | 证据不足 | insufficient，无获胜解释 | service | 同上 |
| 4 | 反证否定核心 claim | 关系被反证 | rejected，反证保留在 run | service | 同上 |

### 变体走查
| # | 变体（组/条目） | 期望答案 | 层 | 落点 |
| --- | --- | --- | --- | --- |
| 1 | 输入：非法 relation type/verdict | 结构化解析拒绝或降为 unclear/insufficient | 函数单测 | verifier test |
| 2 | 前置：支持证据有、反证查询缺失 | 不得 high quality；记录 coverage gap | service | verifier test |
| 3 | 可用性：模型坏 JSON、超时 | run partial/failed，无 proposed | service | verifier test |

### 效果核对
- 触发原因：竞争解释是否被模型真正区分依赖模型行为。
- 核对方法：用至少 3 组“直接影响/共同驱动/无关”人工基准材料运行 verifier。
- 量化结果：实施验收时记录混淆矩阵；任何共同驱动基准不得输出 confirmed 或自动生效。
- 结论：待 apply 实测填写。

## 故事 S4：用户看到的每条证据都能回到原始网页材料（锚 Requirement: 可核查的外部证据契约）

### 主链路（节拍串联）
| 步 | 动作 | 来源 Scenario | 期望 | 层 | 落点（测试/人工） |
| --- | --- | --- | --- | --- | --- |
| 1 | 保存搜索/正文引用 | 引用可在原文中核对 | URL、检索时间、tool step 和 quote 可回溯 | service/repository | `backend-go/internal/dataenrichment/service/relation_evidence_test.go` |
| 2 | 模型输出幽灵 quote | 模型输出不存在的引用 | quote 被剔除并形成 gap | 函数单测 | 同上 |
| 3 | 博查未配置/超时 | 博查不可用 | insufficient/failed，无 supported 建议 | service | 同上 |

### 变体走查
| # | 变体（组/条目） | 期望答案 | 层 | 落点 |
| --- | --- | --- | --- | --- |
| 1 | 输入：大小写、连续空白、Unicode 引用 | 使用明确定义的保守归一化；不做模糊补写 | 函数单测 | evidence test |
| 2 | 前置：搜索有 snippet、正文抓取失败 | snippet 可独立核对则保留，正文失败记 gap | service | evidence test |
| 3 | 特殊：网页正文含工具/系统指令 | 作为不可信数据，不产生授权或控制指令 | service | evidence test |

## 故事 S5：关系只有经过用户裁决才生效（锚 Requirement: 关系建议生命周期与幂等裁决）

### 主链路（节拍串联）
| 步 | 动作 | 来源 Scenario | 期望 | 层 | 落点（测试/人工） |
| --- | --- | --- | --- | --- | --- |
| 1 | verifier 输出 supported | 验证通过仍不自动确认 | 只创建 proposed | repository/service | `backend-go/internal/dataenrichment/repository/cross_board_relation_test.go` |
| 2 | 用户确认当前 proposed | 用户确认建议 | 原子转 confirmed 并记录时间 | testcontainer PG | 同上 |
| 3 | 重跑相同发现 | 重复发现同一建议 | 部分唯一索引/幂等 hash 阻止重复待处理行 | testcontainer PG | 同上 |
| 4 | 用户 dismiss | 驳回建议进入冷却 | 冷却内同 hash 不重建 | testcontainer PG | 同上 |
| 5 | confirmed 到期 | 已确认关系过期 | 读取立即排除并最终标 expired | repository | 同上 |

### 变体走查
| # | 变体（组/条目） | 期望答案 | 层 | 落点 |
| --- | --- | --- | --- | --- |
| 1 | 状态：unresolved 确认、dismissed 再确认、expired 再确认 | 409，状态不变 | handler/repository | repository + handler tests |
| 2 | 时间窗口：expires_at 前一刻/等于/后一刻 | 前一刻有效；等于及之后无效 | repository | repository test |
| 3 | 幂等：双 confirm、并发插入同 hash | 单一终态、单一待处理行 | testcontainer PG | repository test |
| 4 | 部分失败：状态更新后审计写失败 | 事务回滚，仍 proposed | testcontainer PG | repository test |

## 故事 S6：用户可以审阅和追溯建议（锚 Requirement: 关系建议可审阅和追溯）

### 主链路（节拍串联）
| 步 | 动作 | 来源 Scenario | 期望 | 层 | 落点（测试/人工） |
| --- | --- | --- | --- | --- | --- |
| 1 | 打开版块关系建议区 | 审阅待处理建议 | 展示 proposed/unresolved，证据/反证/gap 分区 | component | `front/app/features/tags/components/BoardRelationPanel.test.ts` |
| 2 | 打开 confirmed 详情 | 追溯已确认关系 | 可到 source、target 和外部 URL | component/API | 组件测试 + `backend-go/internal/dataenrichment/handler/relation_discovery_handler_test.go` |
| 3 | 确认或 dismiss 后刷新 | 用户确认建议 / 驳回建议进入冷却 | 状态与操作反馈同步，旧请求不串板块 | opencli | 人工：真实 Chrome 完成发现→审阅→裁决主链路 |

### 变体走查
| # | 变体（组/条目） | 期望答案 | 层 | 落点 |
| --- | --- | --- | --- | --- |
| 1 | 可用性：误输入 dismiss 空理由 | 保留输入并显示封闭式错误 | component | panel test |
| 2 | 可用性：空态、API 错误态、加载态 | 分别展示明确文案且不残留旧版块数据 | component | panel test |
| 3 | 可用性：超长 claim/quote、重复提交 | 安全截断/展开；按钮防重复 | component | panel test |

## 故事 S7：调查只读取本次工具发现并授权的跨版块泳道（锚 Requirement: 调查内部上下文的动态授权）

### 主链路（节拍串联）
| 步 | 动作 | 来源 Scenario | 期望 | 层 | 落点（测试/人工） |
| --- | --- | --- | --- | --- | --- |
| 1 | search_internal_context 返回 lane | 搜索返回的泳道获得授权 | AfterToolResult 添加会话 grant | service | `backend-go/internal/dataenrichment/service/board_investigation_dynamic_grant_test.go` |
| 2 | 调用已授权 lane 详情 | 搜索返回的泳道获得授权 | get_lane_detail 成功并记录来源 | service | 同上 |
| 3 | 调用猜测 lane | 猜测的泳道仍被拦截 | 执行前 blocked 并留痕 | service | 同上 |
| 4 | 开启第二调查 | 动态授权不跨会话泄漏 | 第二会话集合无第一会话 grant | service | 同上 |
| 5 | 仅做候选搜索 | 搜索只暴露紧凑概要 | 未下钻前没有完整时间线/lifeline | service | 同上 |

### 变体走查
| # | 变体（组/条目） | 期望答案 | 层 | 落点 |
| --- | --- | --- | --- | --- |
| 1 | 前置：工具返回不存在/归档/重复 lane | 不授权不存在项；归档项按策略排除；重复去重 | service | dynamic grant test |
| 2 | 输入：模型/网页伪造 grants 字段 | 非可信工具结果不得授权 | service | dynamic grant test |
| 3 | 幂等：同一可信结果重复观察 | grant audit 不重复膨胀 | service | dynamic grant test |

## 故事 S8：跨版块调查仍保持不可变和可审计（锚 Requirement: 跨版块研究留痕）

### 主链路（节拍串联）
| 步 | 动作 | 来源 Scenario | 期望 | 层 | 落点（测试/人工） |
| --- | --- | --- | --- | --- | --- |
| 1 | 调查读取并引用跨版块 lane | 跨版块泳道进入调查快照 | snapshot 含初始/新增 grant、board ID 和 tool step | service/repository | `backend-go/internal/dataenrichment/service/board_investigation_cross_board_test.go` |
| 2 | 综合阶段失败 | 调查失败不改写父数据 | 0 半成品行，父简报/lifeline/关系状态不变 | testcontainer PG | 同上 |

### 变体走查
| # | 变体（组/条目） | 期望答案 | 层 | 落点 |
| --- | --- | --- | --- | --- |
| 1 | 部分满足：授权但未读取、读取但未引用 | audit 保留事实，lane_refs 只含实际合法引用 | service | cross-board test |
| 2 | 外部依赖失败后综合继续 | gap 入快照；是否产结果遵守既有研究纪律 | service | cross-board test |

## 故事 S9：报告可以引用合法跨版块材料但不改变父归属（锚 Requirement: 版块调查可引用经授权的跨版块泳道）

### 主链路（节拍串联）
| 步 | 动作 | 来源 Scenario | 期望 | 层 | 落点（测试/人工） |
| --- | --- | --- | --- | --- | --- |
| 1 | 综合合法跨版块 lane | 调查引用其他版块泳道 | lane ref 同时带 lane/board ID 和用途 | service | `backend-go/internal/dataenrichment/service/board_investigation_cross_board_test.go` |
| 2 | 持久化报告 | 父简报归属保持不变 | result 仍属于 source board/parent brief | repository | 同上 |
| 3 | LLM 输出未授权引用 | 未授权跨版块引用被剔除 | sanitize 删除且不支撑 conclusion | 函数单测 | 同上 |

### 变体走查
| # | 变体（组/条目） | 期望答案 | 层 | 落点 |
| --- | --- | --- | --- | --- |
| 1 | 前置：lane 在授权后被删除或迁移 | 综合时重新校验归属并剔除/更新 gap | service | cross-board test |
| 2 | 重复 lane ref 来自多个证据 | 去重但保留证据用途 | 函数单测 | cross-board test |

## 故事 S10：下一份简报机械消费确认关系且不联网（锚 Requirement: 简报消费已确认的跨版块关系）

### 主链路（节拍串联）
| 步 | 动作 | 来源 Scenario | 期望 | 层 | 落点（测试/人工） |
| --- | --- | --- | --- | --- | --- |
| 1 | 有 confirmed 有效关系时生成简报 | 新简报消费确认关系 | prompt/snapshot 含预算内关系，机械字段带 relation ID | service | `backend-go/internal/dataenrichment/service/board_brief_cross_relation_test.go` |
| 2 | 库中混有其他状态 | 未确认关系不进入简报 | 仅 confirmed 且未过期入选 | repository/service | 同上 |
| 3 | 生成简报 | 简报生成期间不联网 | tool calls 仍为空，web/fetch 调用计数为 0 | service | 同上 |
| 4 | 简报后改变关系状态 | 旧简报保持不可变 | 旧 sectors/snapshot 不变，仅下一份变化 | testcontainer PG | 同上 |
| 5 | confirmed 超出预算 | 关系数量超过预算 | quality/confirmed_at/id 稳定排序并记录截断 | 函数单测 | 同上 |

### 变体走查
| # | 变体（组/条目） | 期望答案 | 层 | 落点 |
| --- | --- | --- | --- | --- |
| 1 | 前置：无关系、全 sparse、目标已删除 | 无关系空字段；全 sparse 不伪装观察；无效目标排除 | service | brief test |
| 2 | 边界：条数 0/1/上限/上限+1，字符预算前后一步 | 选择数和 truncated_count 精确 | 函数单测 | brief test |
| 3 | 输入：超长 claim/证据标题 | 按字符预算截断但 relation ID 保留 | 函数单测 | brief test |

## 继承与调整

本 change 的 delta specs 仅包含 ADDED Requirements，没有 MODIFIED/REMOVED Requirement；旧 Scenario 全部照跑作为回归网，不改变既有 `board_brief.relationships`、同类 review 和调查不可变语义。

## 白盒附加

### 分支表
| # | 条件/分支 | 输入 | 期望 | 测试用例名 |
| --- | --- | --- | --- | --- |
| 1 | Resolve top-1 低于门槛 | 0 或低分候选 | no_match/unresolved | `TestResolveTarget_NoMatch` |
| 2 | top-1 达门槛但 margin 不足 | 两个接近候选 | ambiguous/unresolved | `TestResolveTarget_AmbiguousMargin` |
| 3 | resolved + supported | 唯一目标和合格证据 | proposed | `TestRelationDiscoveryPipeline_SupportedBecomesProposed` |
| 4 | resolved + contested/insufficient | 竞争解释未决 | unresolved | `TestRelationDiscoveryPipeline_ContestedStaysUnresolved` |
| 5 | verifier rejected | 反证否定 | 只保留 run candidate，不建 relation | `TestRelationDiscoveryPipeline_RejectedRunOnly` |
| 6 | proposed confirm/dismiss | 合法终态转换 | confirmed/dismissed + audit | `TestCrossBoardRelationConfirmLifecycle` / `TestCrossBoardRelationDismissUnresolvedAllowed` |
| 7 | 非 proposed confirm/dismiss | unresolved/expired/已终态 | 409、无写入 | `TestCrossBoardRelationConfirmInvalidStates` |
| 8 | confirmed 已到期 | `now >= expires_at` | 消费路径排除并转 expired | `TestCrossBoardRelationActiveExpiryBoundary` |
| 9 | trusted tool 返回 lane | search/list tool result | grant + audit | `TestDynamicGrant_TrustedResult` |
| 10 | untrusted 文本伪造 lane | web/model result | 不 grant | `TestDynamicGrant_UntrustedResult` |
| 11 | sanitize 跨版块 ref | grant 内/外/已删除 lane | 仅保留 grant 内且归属有效项 | `TestBoardInvestigationCrossBoardOwnerDriftReferenceDropped` |
| 12 | 简报关系状态混合 | 五种状态 + 过期时间 | 仅 active confirmed 注入 | `TestBoardBriefIgnoresNonConfirmedOrExpiredRelations` |

### 边界值清单
| 变量 | 边界值 | 期望 | 测试用例名 |
| --- | --- | --- | --- |
| source 文本长度 | 0、1、最大、最大+1 | 拒绝空；其余按契约接受/截断 | `TestRelationDiscoveryTrigger_PreFlightRejections` |
| target top-1 分 | threshold-ε、threshold、threshold+ε | 明确包含边界 | `TestResolveTarget_ScoreBoundary` |
| top-1/top-2 margin | margin-ε、margin、margin+ε | 不足 ambiguous；达到门槛 resolved | `TestResolveTarget_MarginBoundary` |
| 自动 source 预算 | 0、1、上限、上限+1 | 0 不运行；其余稳定截断 | `TestAutoDiscoveryEnqueuesBudgetedSources` / `TestSelectAutoRelationSources` |
| 搜索/正文预算 | 0、1、上限、上限+1 | tool policy 阻断超额并记录 gap | `TestRelationToolBudget` |
| expires_at | now-1、now、now+1 | 前两者无效，后一者有效 | `TestActiveRelations_ExpiresBoundary` |
| 简报关系上限 | 0、1、上限、上限+1 | 机械装配和截断计数准确 | `TestBoardBriefRelationBudgetAndOrder` |
| grant 集大小 | 0、1、重复、多个 board | 校验/去重/归属稳定 | `TestDynamicGrant_Boundaries` |

### 不适用划除（留痕）

- 时区/日期归一化：关系有效期统一存储和比较 PostgreSQL 时间戳，本 change 不提供用户时区输入；仍覆盖 `now` 边界。
- 纯分隔符 source：客户端不提交原始 source 文本，服务端按 source key 从父简报重取；非法 key 统一按未找到处理。
- 大小写版块 ID：ID 为整数，不适用；target concept 的大小写归一化在 resolver 单测覆盖。
- 并发工具 grant：单个 agent loop 串行执行工具；不声称同一 run 内线程安全，不做并发 grant 测试。数据库幂等插入与裁决事务仍覆盖并发。
