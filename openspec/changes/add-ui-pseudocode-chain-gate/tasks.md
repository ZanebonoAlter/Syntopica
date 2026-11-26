<!-- stage: implement -->

# Tasks

## 1. 根因确认（先查数据再设计）

- [x] 1.1 查 `events.db`：`ui-stage-missing` 38 次 vs `ui-stage-scope-out` 0 次 → 分层规则从未被触发
- [x] 1.2 定位原因：`checkStageScope` 判定逻辑完整，但只 warn 不 block，且依赖 agent 自报 marker
- [x] 1.3 结论：任何「自报状态再检查」的方案都有同一个漏洞——可以不申报

## 2. 实现（`lib/ui-design-gate.ts`）

- [x] 2.1 `UI_REASON_CODES` 加 `ui-pseudocode-missing` / `ui-chain-out-of-order`（9 → 11）
- [x] 2.2 `UiContractInput` 加 `designMd?: string | null`
- [x] 2.3 `readPseudocodeSection()`：三种结果 `ok` / `na-empty` / `missing`；`N/A：` 空理由判 `na-empty`
- [x] 2.4 `evaluateUiContract()` 在 major + approved 后查伪代码（pending 优先，不跑到这一步）
- [x] 2.5 `checkPrototypeChain()`：L1 骨架 → L2 原型 → L3 数据；只对 major
- [x] 2.6 `hasSkipMarker()`：`<!-- ui-skip: <环>(理由) -->`，理由非空才豁免
- [x] 2.7 `decideUiToolGate()` 的 pass 分支加依赖链检查
- [x] 2.8 `.pi/extensions/ui-design-gate.ts` 传 `designMd`

## 3. 测试

- [x] 3.1 §A' 伪代码 6 条（缺节 / 无 design.md / N/A 空理由 / N/A 有理由 / 有内容 / pending 优先）
- [x] 3.2 §B' 依赖链 8 条（有骨架放行 / 无骨架 block / 骨架自身不设前置 / 缺 index 写 fixture block / 有 index 放行 / ui-skip 豁免 / ui-skip 空理由不豁免 / 非原型路径不受影响）
- [x] 3.3 D7 / F5 白名单断言改 11 值
- [x] 3.4 既有 fixture 补 `designMd` + `skeleton/`（否则 C3/D3 全线红）
- [x] 3.5 `run-harness-smoke.sh` 除既有红外全绿
- [x] 3.6 既有红测试登记欠账：`spec-gate-smoke:block-target-only-names`（改动前就红，stash 基线验证）

## 4. 文档

- [x] 4.1 `开发执行规范.md` §3.1.1 补「业务先定」前置 + 依赖链顺序
- [x] 4.2 `standard/frontend/interaction.md` 补决策权划线表（影响对外产出 vs 只影响界面）
- [x] 4.3 修订 §3.1.1 的 L0 三节名（与实际八节不符——本 change 已发现，但作为独立文档清理）

## 5. change 制品

- [x] 5.1 proposal / design / design-diagram / ui-design / stage
- [x] 5.2 delta：`ui-design-workflow` MODIFIED 1 条（白名单 9→11）+ ADDED 2 条
- [x] 5.3 `openspec validate` → valid

## 6. review

- [x] 6.1 派 review（`minimax-cn/MiniMax-M3.1-Flash:max`）
- [x] 6.2 处置 review 发现

## 7. 归档

- [x] 7.1 八项门禁全绿
- [x] 7.2 `openspec archive add-ui-pseudocode-chain-gate --yes`
- [x] 7.3 `openspec validate --specs` 全过

## 8. 测试

| 验证点 | 命令 | 期望 |
| --- | --- | --- |
| 伪代码三态 | smoke §A' | 6/6 通过 |
| 依赖链两环 | smoke §B' | 8/8 通过 |
| 白名单 11 值 | smoke D7 / F5 | 11 |
| 既有回归 | `run-harness-smoke.sh` | 除已登记红外全绿 |
| legacy 不追溯 | 唯一 major active change 无 `.openspec.yaml` | 走 legacy 分支不进入依赖链 |

## 9. 文档

<!-- doc-impact: dev -->

- `docs/reference/开发执行规范.md` §3.1.1（4.1 / 4.3）
- `docs/reference/standard/frontend/interaction.md`（4.2）
- `docs/research/process-arch-rework/issues.md`：ISS-02 回填（见 10.2）

## 10. 验证

- [x] 10.1 伪代码三态 §A' 6/6、依赖链 §B' 8/8、路径触发 §B'' 4/4
- [x] 10.2 白名单 11 值（D7 / F5 断言）
- [x] 10.3 既有红已登记欠账，本 change 零新增红

## 11. 验收

- [x] A 伪代码在 major approved 时强制、pending 期不查、minor/none 不查
- [x] B 依赖链只判文件存在性，不依赖 marker；`ui-skip` 为显式留痕出口
- [x] C 唯一在途 major change 不受影响（legacy 分支）
- [x] D reasonCode 白名单 11 值，spec 与代码一致
- [x] E 既有红测试已登记欠账，本 change 零新增红
