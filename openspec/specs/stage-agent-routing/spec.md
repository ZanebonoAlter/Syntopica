# stage-agent-routing Specification

<!-- domain: harness -->

## Purpose

TBD - created by archiving change stage-agent-routing. Update Purpose after archive.

## Requirements

### Requirement: 每个开发阶段有专属 agent 档

系统 SHALL 为 6 个开发阶段（explore / propose / design / implement / verify / archive）各提供一个 `.pi/agents/stage-<阶段>.md` 定义的 agent 档，每个档 SHALL 在 frontmatter 里钉死 `model` 与 `tools`。

#### Scenario: 六档齐全

- **WHEN** 列举 `.pi/agents/` 下的文件
- **THEN** 存在 `stage-explore.md` / `stage-propose.md` / `stage-design.md` / `stage-implement.md` / `stage-verify.md` / `stage-archive.md`
- **AND** 每个文件的 frontmatter 含 `name`（等于文件名去扩展名）、`description`、`model`、`thinking`、`tools`

#### Scenario: 权限按阶段收窄

- **WHEN** 读取 `stage-explore.md` 或 `stage-verify.md` 的 `tools`
- **THEN** 不含 `write` / `edit`
- **AND** `stage-implement.md` 的 `tools` 含 `write` 与 `edit`

#### Scenario: 跨供应商并存

- **WHEN** 汇总六档的 `model` 前缀
- **THEN** 至少出现两个不同的 provider 前缀
- **AND** 不要求与主会话 provider 一致

### Requirement: 派发时按 stage.md 自动选档

系统 SHALL 提供 `scripts/harness/stage-agent.sh <change>`，按该 change 的 `stage.md` 头部 `<!-- stage: <值> -->` 输出应派发的 agent 名。

#### Scenario: 合法阶段

- **WHEN** `stage.md` 声明 `<!-- stage: implement -->`
- **THEN** 脚本输出 `stage-implement`

#### Scenario: 六个阶段全覆盖

- **WHEN** 依次对 6 个合法阶段值运行脚本
- **THEN** 输出依次为 `stage-explore` / `stage-propose` / `stage-design` / `stage-implement` / `stage-verify` / `stage-archive`

#### Scenario: legacy 兜底

- **WHEN** change 目录无 `stage.md`，或 marker 缺失/取值非法
- **THEN** 脚本输出 `general-purpose` 且退出码为 0（不阻断）

#### Scenario: 不阻断流程

- **WHEN** 脚本遇到任何读取失败
- **THEN** 输出去向后仍以 0 退出（fail-open），不让路由问题挡住开发

### Requirement: 旧档保留为无阶段兜底

系统 SHALL 保留 `explore` / `plan` / `general-purpose` / `change-review` / `ui-design` 五个既有档，并在文档中说明其适用场景为「change 流程外或无 stage.md 的任务」。

#### Scenario: 旧档仍可派发

- **WHEN** 以 `subagent_type: change-review` 派发
- **THEN** 派发成功，行为与本次改动前一致

#### Scenario: ui-design 门禁不受影响

- **WHEN** `ui-design-gate` 判定当前档是否为 `ui-design`
- **THEN** 判定逻辑不变（档名未改）
