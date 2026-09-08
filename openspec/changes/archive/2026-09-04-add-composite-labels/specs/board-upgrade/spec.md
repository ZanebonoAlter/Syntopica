## ADDED Requirements

### Requirement: co-tag 高频共现对产出组合标签建议
系统 SHALL 在升级建议生成流程中，基于 co-tag 共现统计产出 compose 决策建议：同一文章内共现频次（co-tag 统计窗口内）≥ composite_cotag_min_cooccurrence（默认 10，ai_settings 可配）且各组件 aux label ref_count ≥ 升级阈值的标签对/三元组，作为候选共现对，经 LLM 裁决是否值得组合（过滤无意义组合，如地名+通用词）；LLM 判定值得组合时产出 compose 建议（含建议的组合标签名、组件 aux label 引用、组合描述），持久化到升级建议表（decision="compose"），suggestion_hash 幂等与 dismissed 冷却期复用现有机制。LLM 判定不值得组合时产出 skip，不落库。

#### Scenario: 高频共现对产出 compose 建议
- **WHEN** 升级建议生成时，「美国国债」与「收益率」在 co-tag 窗口（30 天）内共现 15 次 ≥ 10，且两者 ref_count 均达标，LLM 裁决值得组合
- **THEN** 产出 decision="compose" 的建议（组合名「美债收益率」、组件 [美国国债, 收益率]），经 hash 幂等检查后落库为 pending

#### Scenario: LLM 裁决无意义组合被过滤
- **WHEN** 候选共现对「日本」「市场」共现 12 次，LLM 裁决组合「日本市场」无明确指向语义
- **THEN** 产出 skip，不落库

#### Scenario: 同 hash 建议幂等
- **WHEN** 下一轮生成产出与既有 pending 建议 hash 相同的 compose 建议
- **THEN** 该建议 SHALL 被跳过（skipped），不重复入库

#### Scenario: dismissed 冷却期内拦截
- **WHEN** 用户 dismiss 某 compose 建议，冷却期内同 hash 建议再次生成
- **THEN** 该建议 SHALL 被冷却期拦截（cooldown_blocked），期满后才可重生

### Requirement: compose 建议确认执行
用户确认 compose 建议后，系统 SHALL 在同一事务内创建组合标签（label_type="composite"，source="upgrade_suggest"）及其 composite_components 组件引用、生成组合 embedding、写 ref_count 初始值，并将建议标记为 confirmed。组合创建失败（如 embedder 失败、去重冲突异常）则整体回滚，建议保持 pending。

#### Scenario: 确认创建组合标签
- **WHEN** 用户确认 compose 建议「美债收益率」
- **THEN** 事务内创建组合标签 + 组件引用 + LLM 组合 embedding，建议状态 → confirmed

#### Scenario: 创建失败回滚
- **WHEN** 确认执行时 embedder 调用失败
- **THEN** 整体回滚，组合标签不落库，建议保持 pending，错误返回

#### Scenario: 确认时命中既有组合去重
- **WHEN** 确认执行时 L1/L2 去重命中既有组合标签
- **THEN** 不新建，复用既有组合（ref_count++），建议仍标记 confirmed，执行结果提示复用

### Requirement: 前端渲染 compose 建议
升级建议面板 SHALL 渲染 decision="compose" 的建议：展示建议组合名、组件标签序列、共现证据（共现频次、窗口、代表事件标题）；提供确认执行（创建组合标签）与 dismiss 操作；决策过滤 tab 增加「组合」选项。

#### Scenario: compose 建议卡片
- **WHEN** 建议列表包含 compose 建议「美债收益率」（组件 [美国国债, 收益率]，共现 15 次）
- **THEN** 面板 SHALL 展示组合名、组件序列、共现频次证据，提供确认/dismiss 操作

#### Scenario: 决策过滤包含组合
- **WHEN** 用户切换决策过滤 tab 到「组合」
- **THEN** 列表 SHALL 仅展示 decision="compose" 的建议
