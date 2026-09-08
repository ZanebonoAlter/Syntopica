## MODIFIED Requirements

### Requirement: 探索 agent 工具集

系统 SHALL 向探索 agent（研究助理）注册多级入口探索工具 + `web_search` 联网检索 + `fetch_page` 正文抓取，**不注册任何金融行情工具**（金融工具已彻底移除，不再"降为可选"）。多级入口工具 SHALL 支持分层下钻：`list_boards`（看版块全景）→ `list_lanes`（版块下泳道）→ `get_lane_detail`（泳道详情按需取）。**`get_lane_detail` 的输出 SHALL 包含该泳道的历史背景记忆摘要（month/year 档 lifeline 归档行，受字符预算约束截断、标注粒度与 period），与近期 section 时间线并列呈现**——下钻不得只能取到 section 标题链。agent SHALL 自主决定下钻深度、检索查询与何时停止。

#### Scenario: 多级入口按需下钻

- **WHEN** 探索 agent 判断某版块可能与视角相关
- **THEN** agent SHALL 调 `list_lanes` 查该版块泳道，仅对相关泳道调 `get_lane_detail`，无关版块跳过

#### Scenario: 下钻可读历史背景记忆

- **WHEN** agent 调 `get_lane_detail` 查询存在 month/year 档 lifeline 的泳道
- **THEN** 输出含该泳道背景记忆摘要（预算内截断、标注粒度），agent 可将历史记忆用作论据，不得只拿到近期 section 标题时间线

#### Scenario: 无背景记忆时不报错

- **WHEN** 泳道无任何 lifeline 归档行
- **THEN** `get_lane_detail` 正常返回近期 section 详情，背景记忆段如实标注缺失（不静默省略段落标记）

#### Scenario: web_search 与 fetch_page 配合取证

- **WHEN** 探索 agent 需验证事实节点或抓取一手原文支撑深度层
- **THEN** agent SHALL 调 `web_search` 检索，对关键命中调 `fetch_page` 取正文，结果纳入深度层 `evidence_chain`

#### Scenario: 金融工具彻底不可见

- **WHEN** 话题为任意形态（含金融相关）
- **THEN** 系统 SHALL NOT 注册金融行情工具，agent 全程不可见

### Requirement: 分析前素材新鲜度门

分析编排入口（版块级与单泳道）在装配背景素材前 SHALL 对各活跃泳道 **month / year 档**生命线执行**补全门**（并非仅保鲜）：

1. **缺失补建**：有 section 数据的周期无行时 MUST 先补建（含无任何记录时的首份——首建归分析路径，不再留给定时任务）；
2. **截断重算**：已有行但最后写于 72h 前 MUST 重算覆盖（周期已结束的得到完整版，进行中的得到至今快照）；
3. **限流**：单次分析补全调用设全局限额，溢出降级用旧档继续分析并留结构化日志，不得阻塞；
4. **钳制**：任何写入路径的 `as_of_date` MUST 钳制到不超过当前时刻（周期边界未来日期属脏数据）；
5. week 档退出分析路径检查集（近期记忆由 14 天窗口详情承担，长期记忆由 month/year 承担；存量 week 行保留可被消费）。

补全 MUST 串行执行；失败降级不阻塞分析。

#### Scenario: 截断档分析前重算

- **WHEN** 泳道某已结束周期存在行但该行写于周期结束前（半月档），触发分析
- **THEN** 装配前该周期被重算为完整版，分析素材基于补全后的档案

#### Scenario: 无记录首建

- **WHEN** 泳道无任何 lifeline 行（新孵化泳道），触发分析
- **THEN** 装配前为其 month/year 当前期建首份，而非跳过留给定时任务

#### Scenario: 限额溢出降级

- **WHEN** 补全需求超过单次分析限额
- **THEN** 超出部分留结构化日志并继续分析（用旧档），不阻塞不报错

#### Scenario: 补齐幂等

- **WHEN** 同一板块同一天内连续两次触发分析
- **THEN** 第二次触发不重复补（已补档最后写入时间新鲜）

#### Scenario: 补齐失败降级

- **WHEN** 补齐调用失败
- **THEN** 分析继续（用旧档），失败写入结构化日志可查

#### Scenario: 无数据周期跳过

- **WHEN** 某粒度没有任何 section 数据可形成周期
- **THEN** 系统跳过该粒度，不为无数据创建空档案
