## Purpose

版块扩充方向：用户锁定单一目标版块，系统按相似/共现双路召回相关候选，LLM 以版块画像做二分类裁决，产出挂载建议（单标签 merge / 组合 compose 携带目标）。

## Requirements

### Requirement: 扩充方向锁定单版块
版块扩充方向 SHALL 一次只针对一个用户选定的目标版块运行：用户从活跃版块下拉中单选目标后触发生成，该轮产生的所有扩充建议（单标签 merge 与组合 compose）的 target SHALL 恒等于该锁定版块。系统 SHALL NOT 在一轮生成中跨多个版块产扩充建议。

#### Scenario: 锁定版块生成
- **WHEN** 用户选定版块「美债」触发生成（扩充×单标签）
- **THEN** 该轮所有 merge_into_existing 建议 target_board_id SHALL 等于「美债」的 ID

#### Scenario: 目标版块被禁用后确认失败
- **WHEN** 建议生成后目标版块被禁用（status≠active），用户确认该建议
- **THEN** 确认 SHALL 失败并提示目标版块不可用，建议保持 pending

### Requirement: 扩充候选召回
版块扩充方向 SHALL 按来源召回与目标版块相关的候选：

- **单标签路**：候选为 active 且未挂载进目标版块的辅助标签，召回条件为（1）与目标版块 embedding 相似度达阈值（配置化），或（2）与目标版块现有构成标签在 co-tag 窗口内共现频次达标（配置化）。两种来源 SHALL 去重合并。
- **组合路**：候选为满足 compose 共现门槛的标签对/三元组，且至少一个组件满足上述单标签召回条件（相似或与版块构成共现）。

召回结果 SHALL 按相关性排序后送 LLM 裁决；召回为空时 SHALL 返回空建议列表（正常结果，非错误）。

#### Scenario: 相似标签被召回
- **WHEN** 目标版块「美债」扩充，aux「美债拍卖」与版块 embedding 相似度达阈值且未挂载
- **THEN** 「美债拍卖」SHALL 进入候选列表送 LLM 裁决

#### Scenario: 共现标签被召回
- **WHEN** aux「国债期货」未挂载进「美债」，但与「美债」构成标签「美国国债」在窗口内共现达标
- **THEN** 「国债期货」SHALL 进入候选列表送 LLM 裁决

#### Scenario: 已挂载标签不召回
- **WHEN** aux「美联储」已在目标版块构成中
- **THEN**「美联储」SHALL NOT 出现在扩充候选里

#### Scenario: 无关标签不被召回
- **WHEN** aux「日本央行」与目标版块「美债」相似度未达阈值、且与版块构成标签无达标共现
- **THEN**「日本央行」SHALL NOT 进入候选（省 LLM 成本）

#### Scenario: 组合路候选相关性过滤
- **WHEN** 组合路生成时，共现对「美国国债 × 收益率」中「美国国债」已在目标版块构成中（视为相关）
- **THEN** 该共现对 SHALL 进入候选；而与版块无任何相关组件的共现对 SHALL 被过滤

#### Scenario: 召回为空
- **WHEN** 目标版块无任何相关候选（版块已充分覆盖）
- **THEN** 系统 SHALL 返回空建议列表，不报错

### Requirement: 版块画像上下文与二分类裁决
版块扩充方向的 LLM prompt SHALL 聚焦目标版块完整画像：版块名称与描述、现有构成标签列表（含组合标签）、近期内容标题（该版块近期匹配的 section 标题，数量配置化）。LLM 任务 SHALL 为二分类：每个候选「属于该版块」（单标签路 → merge_into_existing；组合路 → compose 携带目标）或「不属于」（skip）。LLM SHALL NOT 被要求在版块间比较或指定目标。

#### Scenario: 版块画像进 prompt
- **WHEN** 扩充「美债」版块触发 LLM 裁决
- **THEN** prompt SHALL 包含「美债」的描述、构成标签列表与近期内容标题，以及候选列表

#### Scenario: 二分类裁决
- **WHEN** 候选「美联储利率」与「日本央行殖利率」同轮送裁，LLM 判定前者属于「美债」后者不属于
- **THEN** 系统 SHALL 仅对「美联储利率」产出 merge 建议，「日本央行殖利率」skip 不落库
