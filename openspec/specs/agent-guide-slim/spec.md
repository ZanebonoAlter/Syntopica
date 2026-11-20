## Purpose

约束 AI 代理引导文档（根 AGENTS.md、子目录 AGENTS.md、CONTRIBUTING.md）的单一入口与精简形态：规范正文只存在 `docs/reference/`，引导文件只保留项目快照、行为规则与权威文档链接，避免多副本漂移。

## Requirements

### Requirement: Root AGENTS.md as single agent entry point
根目录 `AGENTS.md` SHALL 作为 AI 代理的唯一规范入口，总长度不超过 80 行。内容只包含：项目快照（5 行）、AI 行为规则（20 行）、链接到 `docs/reference/` 下的权威文档、构建命令摘要（10 行）、GitNexus 工作流（保留）。

#### Scenario: Agent reads root AGENTS.md
- **WHEN** AI 代理开始工作会话
- **THEN** 从 `AGENTS.md` 获得项目概要、行为规则和到 `docs/reference/` 的链接，不重复包含完整代码规范

### Requirement: Root AGENTS.md links to reference docs
根 `AGENTS.md` 中关于代码风格、目录约定、测试规范的详细内容 SHALL 被替换为指向 `docs/reference/development.md` 的链接。架构相关内容 SHALL 被替换为指向 `docs/reference/architecture/` 的链接。

#### Scenario: Code convention reference
- **WHEN** 代理需要了解代码风格约定
- **THEN** `AGENTS.md` 指向 `docs/reference/development.md`，代理阅读该文件获取完整规范

### Requirement: Frontend sub-AGENTS.md minimal
`front/AGENTS.md` SHALL 不超过 25 行。内容只包含：声明遵循根 AGENTS.md、前端特有差异点（导入顺序、组件命名、数据映射）、链接到 `docs/reference/`。

#### Scenario: Frontend agent guide
- **WHEN** 代理在 `front/` 目录工作
- **THEN** `front/AGENTS.md` 在 25 行内提供前端特有约定，不重复根 AGENTS.md 已有的通用规范

### Requirement: Backend sub-AGENTS.md minimal and corrected
`backend-go/AGENTS.md` SHALL 不超过 25 行。内容只包含：声明遵循根 AGENTS.md、后端特有差异点（handler 模式、JSON tag、错误包装）、链接到 `docs/reference/`。过时内容（如 "SQLite for persistence"）SHALL 被修正。

#### Scenario: Backend agent guide accuracy
- **WHEN** 代理阅读 `backend-go/AGENTS.md`
- **THEN** 文件正确描述数据库为 PostgreSQL（非 SQLite），且不超过 25 行

### Requirement: Cursorrules minimal
`.cursorrules` SHALL 不超过 5 行。内容只包含：声明遵循 `AGENTS.md`、code-review-graph MCP 工具的简要使用说明。

#### Scenario: Cursor rules
- **WHEN** Cursor IDE 读取 `.cursorrules`
- **THEN** 文件指向 `AGENTS.md` 作为规范源，不重复代码规范内容

### Requirement: CONTRIBUTING.md as link page
`CONTRIBUTING.md` SHALL 精简为纯链接页，指向 `docs/getting-started.md`、`docs/reference/development.md`、`docs/reference/architecture/overview.md`。不包含内联的代码规范内容。

#### Scenario: Contributor reads CONTRIBUTING.md
- **WHEN** 新贡献者打开 `CONTRIBUTING.md`
- **THEN** 看到环境搭建、开发规范、架构文档的链接，无需在 CONTRIBUTING.md 内阅读完整规范

### Requirement: No redundant architecture files in subdirectories
迁移完成后，`front/ARCHITECTURE.md`（851 行）和 `backend-go/ARCHITECTURE.md`（825 行）SHALL 被删除。架构文档的唯一位置为 `docs/reference/architecture/`。

#### Scenario: Subdirectory cleanup
- **WHEN** 搜索项目中的 ARCHITECTURE.md 文件
- **THEN** 只存在 `docs/reference/architecture/` 下的架构文档，不存在子目录级别的 ARCHITECTURE.md

### Requirement: openspec 指令单一权威源（禁止多副本）

openspec 工作流指令 SHALL 仅存在于一个机制层：`.agents/skills/openspec-*/SKILL.md`（2.0 起 skills-only，手动命令文件不再生成）。`.pi/prompts/opsx-*` MUST NOT 存在或手工恢复；`.claude/skills/openspec-*`、`.claude/commands/opsx/`、`.codex/skills/openspec-*` 等 harness 副本 MUST 删除。

`openspec update` SHALL 直接执行（2.x 无 `--tools` 参数）。workflow 集由全局 profile 决定：本仓库依赖逐步建档与归档验证，`new` / `continue` / `verify` MUST 存在于 `openspec config get workflows`，避免被默认 core profile 砍掉。根 AGENTS.md MUST 注明该更新约定。

#### Scenario: 仓库不存在 .claude openspec 副本

- **WHEN** 检查 `.pi/prompts/`、`.claude/skills/`、`.claude/commands/`、`.codex/skills/` 目录
- **THEN** 不存在任何 openspec 工作流指令副本，指令只存在于 `.agents/skills/openspec-*/SKILL.md`

#### Scenario: AGENTS.md 含防再生说明

- **WHEN** 读取根 AGENTS.md 的 openspec 更新约定
- **THEN** 明确写有「直接执行 `openspec update`（2.x 无 `--tools`）」及原因（2.0 skills-only，命令文件不再生成），并说明 workflow 集由全局 profile 管理

#### Scenario: skill 集覆盖项目既有流程

- **WHEN** 执行 `ls .agents/skills | grep openspec` 与 `openspec config get workflows`
- **THEN** 至少包含 `openspec-new-change`、`openspec-continue-change`、`openspec-verify-change` 三个 skill，且 workflows 配置含 `new`、`continue`、`verify`（不因 2.x 默认 core profile 丢失逐步建档与归档验证能力）
