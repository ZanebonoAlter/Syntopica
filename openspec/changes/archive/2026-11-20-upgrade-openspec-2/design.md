## Context

本地 CLI 原为 `@fission-ai/openspec@1.13.2`（2026-09-23 发布），上游最新 `2.1.1`（2026-11-19），跨 2.0 大版本。2.0 的破坏性变化集中在安装形态：skills-only（不再生成 `/opsx:*` 命令文件）、技能集重组（`new`/`continue`/`verify`/`onboard` 移出默认 core profile，新增 `update`）、`openspec update` 取消 `--tools` 参数；2.1 增加「项目内 skills / 全局 skills」选择（非 TTY / agent 环境下不提问，保持原位）。Change 制品文体（Requirement/Scenario delta）从 1.13 到 2.1 未变，自定义 schema `syntopica-ui` 结构也未变。

## Goals / Non-Goals

**Goals:**
- CLI 与项目 skills 迁到 2.1.1，且项目既有流程能力（逐步建档 new/continue、归档验证 verify）不缩水。
- 失效约定同步改写，避免 `--tools pi` 这类死条款继续误导后续会话。

**Non-Goals:**
- 不改变 change 制品文体、不引入图表表达（另开 change 处理「表达现代化」）。
- 不动业务代码、数据库与自定义 schema 文件。

## Decisions

### D1. 升级到 2.1.1 而非停留在 1.13.2

- **结论**：全局 npm 包升到 2.1.1，项目执行 `openspec update`。
- **落点**：`.agents/skills/openspec-update-change/SKILL.md`（2.x 才有的技能，文件存在即证明新版本生效）
- **取舍**：留在 1.13.2（不用迁移，但 `--tools` 约定已失去上游支持，且每过一个月迁移面只增不减）

### D2. skills 保留在仓库内，不用全局 skills

- **结论**：接受非 TTY 下 update 的默认行为，skills 落在 `.agents/skills/` 并随仓库提交。
- **落点**：`.agents/skills/.openspec-target`
- **取舍**：全局 skills（多项目共享一套，但 CI 与 GitHub Copilot 读不到仓库内技能，团队不可见、不可审计）

### D3. 用 `openspec config set workflows` 加回 new / continue / verify

- **结论**：workflow 集设为 `propose, explore, apply, update, sync, archive, new, continue, verify`（不装 onboard/ff/bulk-archive）。
- **落点**：`.agents/skills/openspec-new-change/SKILL.md`
- **取舍**：接受 2.x 默认 core profile（少维护，但只剩 propose 一步生成——会绕过 `syntopica-ui` 的 ui-design 审批门禁，且失去归档前 verify 入口）

### D4. 失效文档与 spec 同步改写

- **结论**：根 `AGENTS.md` 的 `--tools pi` 段改成 2.x 口径；`agent-guide-slim` 的「指令单一权威源」requirement 用 delta MODIFIED 重写。
- **落点**：`AGENTS.md`
- **取舍**：只改 CLI 不改文档（下一轮会话会看到 `--tools pi` 而执行失败，属于会重复踩的死条款）

### D5. `.pi/constraint-injection.json` 的阶段关键词不改

- **结论**：保留 `/opsx-*` 与 skill 名作为**输入别名**，不做删除。
- **落点**：`.pi/constraint-injection.json`
- **取舍**：删掉 `/opsx-*` 关键词（语义更干净，但老习惯输入会静默失去阶段绑定；关键词只是匹配词表，不构成指令副本，不违反 agent-guide-slim）

## Risks / Trade-offs

- [全局 workflows 配置影响本机其它 openspec 项目] → 本机仅此一个 openspec 项目；还原用 `openspec config set workflows '["propose","explore","apply","update","sync","archive"]'`。
- [`.pi/prompts/opsx-*` 9 个手动入口消失，用户肌肉记忆失效] → 同能力仍可用 `/openspec-new-change`、`/openspec-apply-change`、`/openspec-verify-change` 或自然语言触发；AGENTS.md 已写明。
- [2.x 校验变严可能让既有 change 红] → 已用 1.13.2 逐条对比：3 个红项（2 个无 delta、1 个 MODIFIED 丢场景）在两版下**完全一致**，属既有欠账，非升级引入。

## Migration Plan

- 回滚：`npm i -g @fission-ai/openspec@1.13.2` + `git checkout -- .agents/skills .pi/prompts .codex`。
- 旧 change 目录、`.openspec.yaml`、自定义 schema 无需迁移。

## Open Questions

（无）
