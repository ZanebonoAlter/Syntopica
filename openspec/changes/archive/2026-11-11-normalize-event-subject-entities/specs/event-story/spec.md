## MODIFIED Requirements

### Requirement: 主体归一化

`subject_key` SHALL 按固定顺序匹配，前一层命中即终止：① `subject_key` 精确匹配（归一化为小写、下划线分隔）；② `subject_key` embedding 余弦相似度 ≥ 0.92；③ `subject` 字面归一（小写 / NFKC / 去空格 / 去「公司」「集团」等后缀）后精确匹配。三层均未命中的 SHALL 登记为新的主体候选。

`subject_key` embedding SHALL 仅用于兜同义拼写差异，SHALL NOT 用于事件判定。实体归并的相似度阈值 SHALL 显著高于事件召回阈值（0.92 vs 0.5），因为误归并两个不同主体的代价高于漏归并。

主体键 SHALL 存在于事件线上（`board_stories.subject_key`）；`subject` 原文 SHALL 保留用于展示。

系统另有一套**全局实体身份层**（见 `entity-subject-resolution` 能力：全局稳定实体 ID + 名称证据层）。本期该实体身份层以影子模式落地，SHALL NOT 参与事件线身份判定：既有 `subject_key` 三层判定与 `subjectPreFilter` 的 subject_key 一票否决 SHALL 逐字不变；事件线判定路径 SHALL NOT 读取实体 ID；事件线 SHALL NOT 因实体身份层而改写 `subject_key`；人工归属 SHALL NOT 被实体归一结果覆盖；热度口径 SHALL NOT 变更；SHALL NOT 通过迁移或回填推断存量数据的实体身份。实体身份如何接入事件线判定属**后续 change** 的范围，本期 SHALL NOT 把该接线规则作为现行规则。

#### Scenario: 跨语言别名归并
- **WHEN** 两次提取分别产出 subject=`特朗普` / `Trump`，subject_key 均为 `person:trump`
- **THEN** 两者 SHALL 判为同一主体，SHALL NOT 因原文字面不同而分开

#### Scenario: 同公司不同写法归并
- **WHEN** 两次提取分别产出 subject=`字节跳动` / `字节` / `ByteDance`
- **THEN** 三者 SHALL 归并为同一 `subject_key`（如 `company:bytedance`）

#### Scenario: 不同主体不因向量相近而合并
- **WHEN** 两个 subject 的 embedding 相似度为 0.85（低于 0.92），且字面不同
- **THEN** 系统 SHALL 判为不同主体，SHALL NOT 归并

#### Scenario: 三层均不命中时新建
- **WHEN** 新主体在精确键、embedding、字面三层均未命中
- **THEN** 系统 SHALL 登记新的 subject_key，SHALL NOT 复用相似度最高的已有键

#### Scenario: 影子态不改变事件线身份判定

- **WHEN** 某事件标签已在影子态被归一到某个全局实体 ID
- **THEN** 事件线身份判定 SHALL 仍只使用 `subject_key` 三层匹配与 subject_key 一票否决，判定路径 SHALL NOT 读取实体 ID、SHALL NOT 放宽一票否决
- **AND** 存量 `subject_key` 与事件线归属 SHALL 与上线前一致，SHALL NOT 由 `subject_key` 或辅助标签行反推实体身份
- **AND** 人工归属 SHALL NOT 被归一结果覆盖
