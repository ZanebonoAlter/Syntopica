# scope-archive-standards-check Tasks

## 1. 目标范围校验

- [x] 1.1 先新增 `scripts/check-standards.smoke.sh` fixture，覆盖无参全仓 F 段、`--change` 仅目标 F 段、目标自身失败与不存在目标四种情形；脚本在实现前应能复现当前全仓耦合，完成后 `bash scripts/check-standards.smoke.sh` 输出通过。
- [x] 1.2 为 `scripts/check-standards.sh` 实现可选 `--change <name>` 参数处理：仅收窄 F 段至目标 change，A-E/G-H 与无参全仓行为不变；未知参数、缺参和不存在目标明确非零失败。验证：`bash scripts/check-standards.smoke.sh` 通过。

## 2. 归档门禁接入

- [x] 2.1 先扩充 `.pi/extensions/tests/spec-gate.smoke.cjs`，断言非 bypass 归档预检调用 standards 脚本时携带从命令取得的 `--change <change>`，且现有 bypass 与目标自身失败阻断断言不变。
- [x] 2.2 修改 `.pi/extensions/spec-gate.ts` 将从归档命令取得的 change 传给 `check-standards.sh --change`；同步 `.pi/extensions/` 源码快照。验证：`bash .pi/extensions/tests/run-harness-smoke.sh` 通过。

## 3. 测试

- [x] 3.1 `bash scripts/check-standards.smoke.sh` → 退出码 0，目标范围、全仓、目标失败和非法参数场景均通过。
- [x] 3.2 `bash .pi/extensions/tests/run-harness-smoke.sh` → 退出码 0，spec-gate smoke 断言归档预检携带 `--change`。

## 4. 文档

<!-- doc-impact: standard -->
<!-- flow-impact: 无 flow 影响（仅归档门禁脚本与规范改动） -->
<!-- doc-impact-excuse: flow=其他 active change 的 service 改动; api=其他 active change 的 handler 改动; database=其他 active change 的 model 或迁移改动; architecture=其他 active change 的 app 或 tracing 改动; configuration=其他 active change 的配置改动 -->

- [x] 4.1 更新 `docs/reference/开发执行规范.md` 与 `openspec/specs/doc-impact-gate/spec.md`：明确归档门禁的 standards F 段按目标 change 对账、无参手工检查仍全仓巡检。验证：`grep -n "check-standards.sh --change" docs/reference/开发执行规范.md openspec/specs/doc-impact-gate/spec.md` 命中两处。

## 5. 验证

| Scenario | 测试文件 |
| --- | --- |
| 门禁未过时归档被拦截 | .pi/extensions/tests/spec-gate.smoke.cjs |
| 三项全过时放行 | .pi/extensions/tests/spec-gate.smoke.cjs |
| 无关 active change 不阻断目标归档 | scripts/check-standards.smoke.sh |
| 目标 change 对账失败仍阻断归档 | scripts/check-standards.smoke.sh |
| 手动全仓巡检保持全量语义 | scripts/check-standards.smoke.sh |
| 归档目标不存在 | scripts/check-standards.smoke.sh |
| 归档门禁使用目标范围 | .pi/extensions/tests/spec-gate.smoke.cjs |
| 显式豁免留痕 | .pi/extensions/tests/spec-gate.smoke.cjs |

- [x] 5.1 `bash scripts/check-standards.smoke.sh` → 退出码 0，输出目标范围、全仓、目标失败和不存在目标场景均通过。
- [x] 5.2 `bash .pi/extensions/tests/run-harness-smoke.sh` → 退出码 0，spec-gate smoke 含目标范围参数断言。
- [x] 5.3 `bash scripts/doc-impact.sh verify openspec/changes/scope-archive-standards-check`、`bash scripts/scenario-trace.sh openspec/changes/scope-archive-standards-check`、`openspec validate scope-archive-standards-check --strict` → 均退出码 0。
