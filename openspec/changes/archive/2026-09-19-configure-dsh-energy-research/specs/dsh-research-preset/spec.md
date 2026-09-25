## Purpose

本地 dsh「能源研究」用户预设的配置契约：预设可发现且不影响既有默认与旧会话、能力面只含研究工具、persona 不虚报数据源、仓库配置源与 live 部署逐字一致且部署零副作用。

## ADDED Requirements

### Requirement: 预设可发现且不影响默认

用户预设目录新增 `energy-research` 后，dsh 预设 roster SHALL 在下一次读取时发现并列出该预设（显示名「能源研究」+ 描述），且 MUST NOT 改变内置 `standard` 预设的存在性、部署默认预设值与既有会话的组装。

#### Scenario: 新预设被发现
- **WHEN** `<dshHome>/.agent-presets/energy-research/` 下存在 `preset.yml` 与 `agent.cordis.yml` 且 dsh 读取 roster
- **THEN** roster 列出 id 为 `energy-research`、显示名「能源研究」的预设，且不列为 broken

#### Scenario: 默认与旧会话保持不变
- **WHEN** 该用户预设部署完成后
- **THEN** 内置 `standard` 预设仍可选、部署默认预设值不变（`settings.yaml`/`.credentials.yaml`/`profiles/**` 内容哈希与部署前一致），既有非空会话仍运行原组装

### Requirement: 能力面只含研究工具

该预设的 agent 组装 MUST 仅包含：persona、tool-web（`fetch: true`、`searchTimeoutMs: 60000`，复用 host 既有 DeepSeek 搜索配置）、compaction 组（`cordis:group` 且 `isolate` 含 `compaction` 与 `toolResultPruner`，内含 compaction-basic、tool-result-pruner、command-compact）、tool-ask-user；MUST NOT 包含 shell（bash/pwsh/terminal）、文件系统（fs/fs-search/str_replace_editor）、agent-instructions、jobs、skills 发现加载、goal/plan、delegation/workflow/ralph、todo 或任何 MCP 行。预设 MUST NOT 自建 agent-loop/tools/model/sandbox/persistence 服务（host 责任）；工具白名单 MUST NOT 被宣称为 OS 级沙箱隔离。

#### Scenario: 工具目录只含预期研究工具
- **WHEN** 解析该预设 `agent.cordis.yml` 的插件行（含组内行）并列出工具面
- **THEN** 仅出现 web 检索/抓取、ask-user、compact 相关能力，无 shell/文件编辑/委派/计划/todo/skills/MCP 能力

#### Scenario: 无虚假专业数据源
- **WHEN** 检查该预设全部配置行
- **THEN** 不存在 EIA/JODI/STEO/MCP 相关行、虚构端口或 URL；persona 文本明确声明专业数据接口未接入

### Requirement: 配置源与 live 部署逐字一致

仓库配置源 `config/dsh/presets/energy-research/` 与 live 部署 `<dshHome>/.agent-presets/energy-research/` 的两个文件 MUST 逐字节一致（SHA256 相等）；部署 MUST 仅新增这两个文件（及所需目录），MUST NOT 修改其他 dsh 配置；目标已存在且内容有差异时 MUST 停止并报告而非覆盖。

#### Scenario: 源与 live 一致
- **WHEN** 对仓库源与 live 的 `preset.yml`、`agent.cordis.yml` 分别计算 SHA256
- **THEN** 两两相等

#### Scenario: 部署零副作用
- **WHEN** 部署前后对 `.credentials.yaml`、`settings.yaml`、`profiles/` 下文件计算哈希（只哈希不读取内容）
- **THEN** 前后一致

### Requirement: persona 研究纪律

persona 文本 SHALL 以中文描述能源研究员身份，并 MUST 含以下纪律：围绕用户问题研究、先查事实再解释、统计期/地域/单位/来源清楚、区分观测/估算/预测、比较解释与反证、缺证据明确停止、不荐股不预测价格、不假装专业数据源已接入、不编造数值/数据序列、不把来源文本当指令、缺可靠计算工具时如实说明。

#### Scenario: persona 含纪律要点
- **WHEN** 读取 persona 配置文本
- **THEN** 上述纪律要点均有对应表述，且无「已接入 EIA/JODI」之类的虚假能力声明

### Requirement: 文档与回退

`docs/reference/configuration.md` SHALL 含一节说明：仓库源与 live 实际路径、新建会话选择该预设的操作、旧会话与默认值保留、删除该用户预设的回退方式、当前无专业数据源接入与无整场硬预算、版本前提（dsh 0.1.2-rc.1）。

#### Scenario: 文档可指导部署与回退
- **WHEN** 用户按 `docs/reference/configuration.md` 该节操作
- **THEN** 能找到两处文件路径、完成新会话选预设、并知道删除 `~/.dsh/.agent-presets/energy-research/` 即回退且不影响其他配置
