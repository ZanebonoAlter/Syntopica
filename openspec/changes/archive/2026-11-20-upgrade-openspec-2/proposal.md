<!-- complexity: simple -->
<!-- ui-impact: none -->

## Why

本地 `@fission-ai/openspec` 停留在 **1.13.2（2026-09-23）**，上游最新为 **2.1.1（2026-11-19）**，相差 8 个版本且跨 2.0 大版本。2.0 的破坏性变化落在**安装形态**而非 spec 文体：

- **skills-only**：不再生成 `/opsx:*` 命令文件，全部收敛为 `.agents/skills/openspec-*` 一套 skill；`openspec update` 的 `--tools` 参数取消。
- 技能集重组：删除 `openspec-new-change` / `openspec-continue-change` / `openspec-verify-change` / `openspec-onboard`，新增 `openspec-update-change`。
- 2.1 支持全局 skills（问一次落位，选项见升级向导）。

这直接让项目现有约定失效：`openspec/specs/agent-guide-slim/spec.md` 硬性规定「工作流指令仅存在于 `.pi/prompts/opsx-*` 与 `.agents/skills/openspec-*/` 两层」以及「`openspec update` 必须带 `--tools pi`」，根 `AGENTS.md` 同款说明。2.x 下 `.pi/prompts/opsx-*` 不再生成、`--tools` 不存在，该 requirement 与文档说明都会变成死条款。

不含：本次不改变 change 制品文体（spec 的 Requirement/Scenario 范式 1.13 → 2.1 未变），也不引入图表表达（另开 change）。

## What Changes

- 全局 CLI 由 `1.13.2` 升到 `2.1.1`（npm 全局包，`@fission-ai/openspec`）。
- 在仓库根执行 `openspec update`，按 dry-run 计划迁移 29 项：
  - `.agents/skills/`：5 个 SKILL.md 更新、新增 `.openspec-target` 与 `openspec-update-change`、删除 continue/new/onboard/verify 四个 skill。
  - `.pi/prompts/opsx-*.md` 9 个手动命令入口删除（2.x 不再生成）。
  - `.codex/skills/openspec-*` 9 个旧副本删除。
- 同步失效约定：
  - `openspec/specs/agent-guide-slim/spec.md` 的「openspec 指令单一权威源」requirement 改为 2.x 形态（单层 skills + `update` 无 `--tools`）。
  - 根 `AGENTS.md` 的 `--tools pi` 说明段改写。
  - `docs/reference/开发执行规范.md` 与 `docs/reference/harness/pi-extensions.md` 中 `/opsx-*` 命令入口表述改为 skill 触发。
  - `.pi/constraint-injection.json` 的阶段关键词表移除已删 skill 名 / `/opsx-*` 入口（保留能匹配用户口述的别名需逐项判断）。
- 验证：`openspec validate --all`、`openspec status --change <各 active change>`、自定义 schema `syntopica-ui` 可加载、harness smoke 通过。

## Capabilities

### New Capabilities

（无）

### Modified Capabilities

- `agent-guide-slim`: 「openspec 指令单一权威源（禁止多副本）」requirement 从「`.pi/prompts/opsx-*` + `.agents/skills/` 双层」改为「`.agents/skills/` 单层 + 2.x update 无 `--tools`」，场景同步改写。

## Impact

- **改动的仓库文件**：`.agents/skills/**`（11 项）、`.pi/prompts/opsx-*.md`（9 删除）、`.codex/skills/**`（9 删除）、`AGENTS.md`、`docs/reference/开发执行规范.md`、`docs/reference/harness/pi-extensions.md`、`.pi/constraint-injection.json`、`openspec/specs/agent-guide-slim/spec.md`。
- **不碰**：业务代码（`backend-go/`、`front/`）、数据库、自定义 schema `openspec/schemas/syntopica-ui/`（update 不动它，已核对 dry-run 只涉 skills/prompts）、既有 change 制品。
- **部署后用户可见变化**：`/opsx-new`、`/opsx-apply`、`/opsx-verify` 等手动命令入口消失；改用 `/openspec-propose`、`/openspec-apply-change`、`/openspec-update-change` 或直接自然语言触发。`openspec update` 不再接受 `--tools`。
- **需要的操作**：无需数据迁移；旧 change 目录、`.openspec.yaml`、自定义 schema 完全兼容。回滚方式为 `npm i -g @fission-ai/openspec@1.13.2` + `git checkout -- .agents/skills .pi/prompts .codex`。
