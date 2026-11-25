<!-- domain: dev -->

# standard-spec-format Specification

## Purpose
TBD - created by archiving change standard-spec-injection. Update Purpose after archive.
## Requirements
### Requirement: Standard 文档 spec 结构

**首期试点范围**：本 capability 首期仅约束 `docs/reference/standard/backend/ai-logging.md`（试点文档）。其余 standard 文档（code-style / lint / package-layout / testing / theming / interaction-conventions / commit-pr …）在后续 change 逐步 spec 化，过渡期 SHALL 不强制、check-standards SHALL 不校验（见 design §8/§9）。

已 spec 化的 standard 文档 SHALL 遵循：包含 `## Requirements` 节，其下以 `### Requirement: <name>` 组织规范条目。每个 Requirement SHALL 标注 `**级别**: MUST` 或 `SHOULD`，并附至少一个 `#### Scenario: WHEN/THEN`。

文档头 SHALL 含 HTML 注释 `doc-impact-applies: <代码路径模式>` 声明适用范围，供 constraint-injection extension 的 jitDocs 配置关联代码改动（配置直接从该标签生成，单一真相源）。路径须为仓库根相对路径，逗号分隔多 token，做路径前缀匹配（见 design §4）。

规范散文（Anti-Patterns、注意事项）SHALL 并入对应 Requirement 的 Scenario（`AND NOT` 句式）或作为 Requirement 正文，而非独立成节，保证注入机制提取 `## Requirements` 节即得全部规范条目。

#### Scenario: spec 结构齐全

- **WHEN** 校验已 spec 化的 standard 文档（首期：ai-logging.md）
- **THEN** 存在 `## Requirements` 节，其下 Requirement 条目含级别 + 至少一个 Scenario

#### Scenario: 过渡期非 spec 文档全文注入

- **WHEN** 某标准文档尚未 spec 化（无 `## Requirements` 节）但 `doc-impact-applies` 命中改动代码
- **THEN** extension 全文注入该文档（standard 定位即约束清单，不报错、不静默跳过）

#### Scenario: 多文档同时命中

- **WHEN** write/edit 路径同时命中多个 standard 文档的 `doc-impact-applies`
- **THEN** extension 执行规范段依次列出每个命中文档内容，各自标文档名标头

