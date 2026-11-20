## 1. CLI 与技能安装

- [x] 1.1 全局升级 CLI：`npm install -g @fission-ai/openspec@2.1.1`，验证 `openspec --version` 输出 `2.1.1`
- [x] 1.2 迁移前评估：`openspec update --dry-run --json`，记录 29 项（.agents 11 / .codex 9 / .pi/prompts 9）
- [x] 1.3 执行 `openspec update`，核对实际改动与 dry-run 一致（9 个命令文件删除、4 个 skill 移出 core）
- [x] 1.4 加回项目依赖的 workflow：`openspec config set workflows '["propose","explore","apply","update","sync","archive","new","continue","verify"]'`，再跑一次 `openspec update` 安装，验证 `ls .agents/skills | grep -c openspec` 输出 9

## 2. 文档与 spec 同步

- [x] 2.1 改写根 `AGENTS.md` 的 `--tools pi` 段为 2.x 口径（直接 update + 全局 profile 约定）
- [x] 2.2 写 delta spec：`specs/agent-guide-slim/spec.md` 的 MODIFIED requirement（保留原两个 Scenario 名，新增 skill 集覆盖场景）
- [x] 2.3 复核 `.pi/constraint-injection.json` 后决定不改（关键词是输入别名，不是指令副本；理由记 design D5）

## 3. 测试

- [x] 3.1 `openspec validate --specs` 期望 `Totals: 173 passed, 0 failed`
- [x] 3.2 `openspec validate --changes` 并与 `npx @fission-ai/openspec@1.13.2 validate` 对比，确认红项集合一致（3 个既有欠账：add-home-pipeline-status-page、lane-compose-topic-event-tags、topic-lane-auxiliary-label-constraint）
- [x] 3.3 `openspec schemas` 期望列出 `syntopica-ui (project)`；`openspec status --change upgrade-openspec-2` 期望 schema 为 syntopica-ui 且 DAG 为 proposal → ui-design → specs/design → tasks
- [x] 3.4 `bash scripts/harness/check-tasks-tail.sh openspec/changes/upgrade-openspec-2` 期望退出码 0

## 4. 文档

<!-- doc-impact: none(只改 CLI/skill 安装形态与根 AGENTS.md，不涉 docs/reference 八域) -->
<!-- doc-impact-excuse: flow=本会话未改 docs/reference/flow，命中来自并发脏树（edit.map 快照混入 backend-go/internal/reader 其他 change 文件）; database=同上，命中来自 backend-go/internal/platform/database 多个其他 change 的迁移脏文件 -->

- [x] 4.1 proposal 的 Impact 段与实际改动对齐（`.pi/constraint-injection.json` 实际未改，已由上条说明）

## 5. 验证

- [x] 5.1 `openspec validate upgrade-openspec-2 --type change` 期望 `1 passed, 0 failed`
- [x] 5.2 `openspec status --change upgrade-openspec-2` 期望 `5/5 artifacts complete`
- [x] 5.3 `ls .pi/prompts/ 2>/dev/null` 期望为空（无 `opsx-*` 残留）
- [x] 5.4 `grep -rn "tools pi" AGENTS.md` 期望 0 命中
- [x] 5.5 `bash scripts/harness/archive-readiness.sh upgrade-openspec-2` 期望门禁全绿
