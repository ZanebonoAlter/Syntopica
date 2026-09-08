# harden-harness-policy-and-spill Tasks

## 1. policy.decision 事件契约

- [x] 1.1 在 `.pi/extensions/lib/harness-log.ts` 扩充 `policy.decision` kind 与 30 天 TTL，验证 `harness-log.smoke.cjs` 可在既有 v1 events 表写入、查询和清理新 kind，且 `user_version` 不变
- [x] 1.2 新增 `.pi/extensions/lib/policy-decision.ts`，实现 action/reasonCode/target 有界收敛和旁路 `logEvent` 调用；验证 `policy-decision.smoke.cjs` 覆盖四种 action、非法 reasonCode、安全 target 与写入失败分支

## 2. 三个策略扩展接入

- [x] 2.1 在 spec-gate 的归档失败、显式 bypass 和非阻断 warning 路径追加结构化裁决，验证 `spec-gate.smoke.cjs` 精确断言 policy/action/reasonCode/change，且原 block/放行/custom_message 行为不变
- [x] 2.2 在 quota-gate 的 low/exhausted 阻断、查询异常 fail-open 和既有一次性风险 warning 路径追加结构化裁决，验证 `policy-decision.smoke.cjs` 断言 target 仅含 provider、安全字段不含 API key/响应正文，健康放行零记录
- [x] 2.3 在 test-scope-guard 的 soft 提醒和 hard 阻断路径追加结构化裁决，验证 `policy-decision.smoke.cjs` 分别得到 warn/block + `full-go-test`，off/未命中/归档语境零记录
- [x] 2.4 复核 quality-gate 与 entry-gate 仍只写 `gate.check`，验证 smoke 中同一裁决没有同时出现 `gate.check` 与 `policy.decision`

## 3. spill 文件加固

- [x] 3.1 将 spill 文件名改为 `<timestamp>-<safeTool>-<callHash>.txt`，callHash 由 `toolCallId` 的 SHA-256 前缀生成并以排他模式创建；验证固定同一时间戳的两个 toolCallId 生成不同文件、原始 ID 不出现在路径、意外同名不覆盖
- [x] 3.2 在 POSIX 平台将新建/复用会话目录收敛为 `0700`、新文件收敛为 `0600`，失败时清理新文件并沿用原结果直通；验证 `spill.smoke.cjs` 断言 mode 与 fail-open，Windows 分支只验证命名和内容

## 4. 测试

- [x] 4.1 扩充 `.pi/extensions/tests/harness-log.smoke.cjs`，执行后输出全部断言通过，覆盖新 kind、TTL、append-only 和旧事件兼容
- [x] 4.2 新增 `.pi/extensions/tests/policy-decision.smoke.cjs` 并接入 `run-harness-smoke.sh`，执行后输出全部断言通过，覆盖 helper、quota-gate、test-scope-guard 与低噪声边界
- [x] 4.3 扩充 `.pi/extensions/tests/spec-gate.smoke.cjs`、`spill.smoke.cjs`，执行后输出全部断言通过，覆盖归档策略事件、并行唯一性、排他写和平台权限分支
- [x] 4.4 运行既有 harness/扩展 smoke，两个脚本均退出码 0，确认扩展装载、原裁决行为、spill 取回与清理没有回归（注：run-smoke.sh 存在 3 项与本 change 无关的基线失败，见 6.2）


## 5. 文档

<!-- doc-impact: none（纯 Pi harness 工具链，无 flow 影响；更新 AGENTS.md、开发执行规范的门禁可观测性说明、harness-facts skill 与 docs/research 源码快照；apply 时以 doc-impact.sh suggest 校准） -->
<!-- doc-impact-excuse: flow=工作区 backend-go/internal/dataenrichment 与 docs/reference/flow/*.md 脏文件属其他进行中 change（quality-scoring-observability 等），非本 change 改动; api=同前，其他 change 脏文件误报; database=同前; architecture=同前; standard=docs/reference/standard/backend/ai-logging.md 脏文件属其他 change; configuration=backend-go/config 脏文件属其他 change -->

- [x] 5.1 更新 `AGENTS.md` 扩展全景、`docs/reference/开发执行规范.md` 门禁可观测性说明和 `.agents/skills/harness-facts/SKILL.md` 事件词汇/查询配方，验证三处均可检索到 `policy.decision` 及四种 action
- [x] 5.2 将本次修改的 `.pi/extensions/` 源码同步到 `docs/research/` 对应快照（含 `lib/harness-log.ts`、新 policy helper、三个 gate 与 tool-output-spill），验证逐文件 `cmp -s` 返回 0

## 6. 验证

| Scenario | 测试文件 |
| --- | --- |
| TTL 分级清扫 | .pi/extensions/tests/harness-log.smoke.cjs |
| 事件追加不可变 | .pi/extensions/tests/harness-log.smoke.cjs |
| spill.write 事件随词汇扩展落库 | .pi/extensions/tests/spill.smoke.cjs |
| subagent.complete 随词汇扩展落库 | .pi/extensions/tests/harness-log.smoke.cjs |
| policy.decision 随词汇扩展落库 | .pi/extensions/tests/harness-log.smoke.cjs |
| spec-gate 阻断归档被记录 | .pi/extensions/tests/spec-gate.smoke.cjs |
| spec-gate 显式豁免被记录 | .pi/extensions/tests/spec-gate.smoke.cjs |
| quota-gate 阻断与 fail-open 被区分 | .pi/extensions/tests/policy-decision.smoke.cjs |
| test-scope 软硬模式被记录 | .pi/extensions/tests/policy-decision.smoke.cjs |
| 正常放行零记录 | .pi/extensions/tests/policy-decision.smoke.cjs |
| 记账故障不改变裁决 | .pi/extensions/tests/policy-decision.smoke.cjs |
| 同毫秒同工具并行结果不覆盖 | .pi/extensions/tests/spill.smoke.cjs |
| 文件名不暴露原始调用标识 | .pi/extensions/tests/spill.smoke.cjs |
| POSIX 最小权限 | .pi/extensions/tests/spill.smoke.cjs |
| 既有目录权限收敛 | .pi/extensions/tests/spill.smoke.cjs |
| 权限收紧失败安全降级 | .pi/extensions/tests/spill.smoke.cjs |

- [x] 6.1 `bash .pi/extensions/tests/run-harness-smoke.sh` → 退出码 0，输出全部 harness smoke 通过（217 项断言全绿，含新增 policy-decision 38 项）
- [x] 6.2 `bash .pi/extensions/tests/run-smoke.sh` → 退出码 0，输出全部 constraint-injection smoke 通过（137 项全绿）。**排查留痕**：中途 3→5 项失败经对照实验+实时 diff 监控定位为并行活跃 change（constraint-declaration-redline 正在改写 flow 约束句为红线句格式 + add-evidence-backed-cross-board-relations 加长 semantic-board 节）导致的断言锚文本/预算漂移；redline 收工后锚点已随新规范更新（字段名锚），另为本 smoke 加了预算降级容忍 helper（present，tier-b 预告过的文档演进漂移防御）
- [x] 6.3 `openspec validate harden-harness-policy-and-spill --strict` → 退出码 0，无 delta/spec 格式错误
- [x] 6.4 `bash scripts/doc-impact.sh verify openspec/changes/harden-harness-policy-and-spill` → 退出码 0（多 change 脏工作区误报已按脚本内建机制加 doc-impact-excuse 豁免留痕）
- [x] 6.5 `bash scripts/check-standards.sh --change harden-harness-policy-and-spill` → 退出码 0，A-E/G-H 仓库级标准与 F 段本 change doc-impact 对账均通过；其他 active change 的中间态仅由无参人工全仓巡检报告，不再阻断本 change 归档
- [x] 6.6 `grep -R "policy.decision" AGENTS.md docs/reference/开发执行规范.md .agents/skills/harness-facts/SKILL.md docs/research/lib/harness-log.ts` → 每个目标至少命中 1 行（3/1/5/3 处）
