## MODIFIED Requirements

### Requirement: 影子接入与既有判定零改动

- 系统 SHALL 提供 `off`（默认）/ `shadow` / `active` 三种模式；本期 SHALL 只实现 `off` 与 `shadow`；`active` SHALL 被显式拒绝并返回明确错误，SHALL NOT 静默按影子执行。（默认值变更：2026-11-10 决策，影子实验结束、实体层不接入事件线判定，默认回到 `off`——避免实体注册表只增不减成为黑洞；`shadow` 保留为实验态，需显式配置开启；`off` 仍保留为完全关闭开关。）
- `off` SHALL 保持第一轮提示与 schema 与调用数**逐字不变**，SHALL 零第二轮模型调用、SHALL 零实体层写入、SHALL NOT 写入 `topic_tags.metadata.entity_subject` 键。
- `shadow` SHALL 在第一轮提示中启用稳定短身份说明，并 SHALL 只写三处：① 实体层；② 正常 AI 审计（`ai_call_logs`）；③ `topic_tags.metadata.entity_subject` 这一**独立惰性键**（仅影子态第一轮产出、无读路径）。SHALL NOT 改写 `event_fingerprint`、标签的业务字段与归属，SHALL NOT 写 story / report / topic 任何字段与任何界面可见字段。
- **两个入口 SHALL 区分**：应用常规打标在 `shadow` 下 SHALL 持久化上述惰性 metadata 键；**评测 CLI SHALL NOT 写标签业务表**，其第一轮原始结果 SHALL 存放于评测产物与实体层证据。
- 事件线身份判定已随事件线退役整体下线（退役边界见 `event-story` 的退役 delta）：在线链路 SHALL NOT 再执行任何事件线身份判定、事件线归属或人工归属覆盖，实体层 SHALL NOT 参与任何在线身份判定。事件标签的指纹（`event_fingerprint` 与其中的 `subject_key`）仍由打标链产出，但 `subject_key` SHALL 只作指纹字段、SHALL NOT 被读作在线判定依据，SHALL NOT 参与任何热度或归属计算；组合标签 SHALL NOT 产生实体提及；存量实体身份 SHALL 保持为空（SHALL NOT 迁移或回填推断）。
- 「双侧都有实体 ID 时按 ID、单侧回退、两侧无 ID 走既有路径」等接线规则 SHALL NOT 作为现行规则写入本期规格（属后续 change）。

#### Scenario: 默认关闭态逐字不变

- **WHEN** 系统以默认配置运行打标
- **THEN** 第一轮提示与 schema 与调用数 SHALL 与上线前逐字一致，SHALL NOT 新增字段输出或模型请求
- **AND** SHALL NOT 发起第二轮模型调用、SHALL NOT 写入实体层、SHALL NOT 写入 `topic_tags.metadata.entity_subject` 键

#### Scenario: 影子态写入限定三处

- **WHEN** 系统以 `shadow` 模式处理一批事件标签
- **THEN** 实体层、正常 AI 审计、`topic_tags.metadata.entity_subject` 独立惰性键 SHALL 有新增
- **AND** `event_fingerprint`、标签的业务字段与归属、日报、话题与界面可见字段 SHALL 不变
- **AND** 评测 CLI 运行时 SHALL 不写标签业务表，第一轮原始结果 SHALL 只落评测产物与实体层证据

#### Scenario: 事件线身份判定不读实体 ID

- **WHEN** 某事件标签已被归一到某个全局实体 ID
- **THEN** 在线身份判定 SHALL 不存在（事件线已退役），实体层 SHALL NOT 产生任何在线判定输入或输出
- **AND** 存量 `subject_key` 指纹值 SHALL 与上线前一致，SHALL NOT 由 `subject_key` 或辅助标签行反推实体身份
- **AND** 人工归属 SHALL NOT 被归一结果覆盖，组合标签 SHALL NOT 产生实体提及
