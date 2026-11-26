## MODIFIED Requirements

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
