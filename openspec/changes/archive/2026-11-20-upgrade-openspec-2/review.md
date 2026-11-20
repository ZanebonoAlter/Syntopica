### R-1 2026-11-20【Medium】主 spec 旧标题级别导致 2.x 归档拒绝

- **状态**：已修复
- **发现**：`openspec validate upgrade-openspec-2` 报 INFO「MODIFIED failed for header … not found」。根因不是文本差异，而是 `openspec/specs/agent-guide-slim/spec.md` 全篇用 `## Requirement:` / `### Scenario:` 二级旧格式（全仓 1069 个 requirement 里只有 11 个是旧的，该 spec 独占 8 个），2.x 归档按 `### Requirement:` 三级解析，匹配不到就拒。
- **处置**：把该主 spec 的 8 个 requirement 与对应 scenario 提升一级（`###` / `####`），并把 `# Purpose` + `TBD` 补成真实 Purpose。改后 `openspec validate agent-guide-slim --type spec` 通过，change 的 INFO 消失。

### R-2 2026-11-20【High】默认 core profile 会砍掉项目流程依赖

- **状态**：已修复
- **发现**：2.1.1 `openspec update` 按默认 core profile 删除 `openspec-new-change` / `openspec-continue-change` / `openspec-verify-change`。本仓库 `syntopica-ui` schema 需要逐步建档（proposal → ui-design 审批 → specs），`propose` 一步生成会绕过 major UI 原型审批门禁；`verify` 是归档前验证入口。
- **处置**：`openspec config set workflows` 加回 `new` / `continue` / `verify` 后二次 `update`，skill 集从 6 个恢复到 9 个；`openspec config get workflows` 与 `ls .agents/skills` 双向核对。

### R-3 2026-11-20【Low】doc-impact 归属轨被并发脏树污染

- **状态**：已采纳
- **发现**：`archive-readiness` 报「声明 none 但启发式命中 flow/database」。查 `events.db` 中本 change 最新 `edit.map`（id 148724）：17 个路径里混入了 `backend-go/internal/platform/database/*`、`backend-go/internal/reader/service/ai_detection_service.go` 等其他 change 的脏文件，本会话实际只改了 `AGENTS.md`、`openspec/changes/upgrade-openspec-2/**`、`openspec/specs/agent-guide-slim/spec.md`。
- **处置**：在 tasks.md 增加 `<!-- doc-impact-excuse: flow=…; database=… -->` 留痕（脚本为多 change 脏工作树内置的兜底通道），不修改事实库、不伪造域声明。

### R-4 2026-11-20【Medium】2.x 校验变严的误判风险

- **状态**：已驳回
- **发现**：`validate --changes` 有 3 个红项，初步怀疑是升级引入的严格化。
- **处置**：用 `npx @fission-ai/openspec@1.13.2 validate` 逐条复跑对比，3 个红项（2 个无 delta、1 个 MODIFIED 丢场景）在两版下**输出完全一致**，判定为既有欠账，不计入本 change。结论写入 design「Risks / Trade-offs」。
