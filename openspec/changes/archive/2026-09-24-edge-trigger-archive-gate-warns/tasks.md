## 1. 用例先行（先红后绿）

- [x] 1.1 建 `test-cases.md`（本 change 目录）：主链路表串「归档重试循环中 warn 只说一次」完整故事（步/动作/来源 Scenario/期望/层/落点）；⓪ 继承与调整表——先跑 `bash scripts/harness/test-assets.sh concurrent-change-coordination` 与 `bash scripts/harness/test-assets.sh test-case-design` 反查旧资产，旧 Scenario（树上 warn / 冷启动跳过 / 无违例静默 / 缺失 warn / 分层错配 warn / 异常 fail-open / 豁免兼容）逐行处置（全部「继承」为防回归锚点，新增 6+3 个边沿 Scenario 为扩展）
- [x] 1.2 变体走查 + 白盒附加：空清单/单行/重复行/顺序颠倒清单的指纹稳定性；空 sessionId 兜底槽；同会话跨 change 键隔离；LRU 超限淘汰；compact 清空后重发；不适用项（时间窗口/幂等并发等）划除留痕
- [x] 1.3 `spec-gate.smoke.cjs` 新增边沿用例（同会话 mock 两次 `tool_call`）：同指纹第二次尝试零 sendMessage 零 warn 记账（现行全量重发 → 红）；清单变化第二次重新投递并再记账（现行绿，防回归锚点）；转净/清零收尾单行（现行无 → 红）；新 sessionId 首见重投（现行绿，防回归锚点）。验证：`bash .pi/extensions/tests/run-harness-smoke.sh` 中新用例当前红、锚点用例绿
- [x] 1.4 边沿判定纯逻辑用例（deliver/silent/close 三态 × 首见/同指纹/转绿/再犯矩阵，无 DB 依赖）：现行无此函数 → 红

## 2. 指纹状态与投递边沿（design D1/D2/D3）

- [x] 2.1 `spec-gate.ts` 增 per-sessionId 指纹表（二级 Map + 兜底槽 + LRU 上限 32）与 sha256 指纹函数（⑤' 清单行排序去重后哈希；⑤ 文案列表 join 后哈希，取 16 字节前缀）。验证：1.3/1.4 用例转绿
- [x] 2.2 检查⑤ 投递点接入边沿判定：首见全量投递+记账、同指纹静默（console 一行）、违例清零单行收尾删条目。验证：1.3 对应用例绿，既有 ⑤ 用例（无违例静默/缺失 warn/分层错配 warn/异常 fail-open/豁免兼容）保持绿
- [x] 2.3 检查⑤' 投递点同款接入（exit 3 冷启动维持零输出零收尾）。验证：1.3 对应用例绿，既有「树上 warn/冷启动跳过」锚点用例保持绿
- [x] 2.4 `git diff .pi/extensions/spec-gate.ts` 核对：无检查①-④' 失败收集、block reason、`archive-check-failed`/`ui-verification-missing` 记账行变更；bypass / fail-open / 无名 fail-open 的 warning 路径不在指纹表覆盖内（D5）。验证：diff 仅含 warn 投递点与新增状态模块

## 3. compact 与会话边界（design D4）

- [x] 3.1 `pi.on("session_compact")` 清该会话指纹条目；D7 异常 fail-open 包裹（compact 回调与指纹计算异常 → 按无状态全量投递 + console.warn）。验证：smoke 中 compact 模拟（直清条目）后同指纹再试重投一次

## 4. 测试

- [x] 4.1 `bash .pi/extensions/tests/run-harness-smoke.sh` → 退出码 0（含 1.3/1.4 新增用例与全部既有 spec-gate 用例）
- [x] 4.2 `bash .pi/extensions/tests/policy-decision.smoke.cjs` 语义回归由 4.1 覆盖 → 记账词汇表（action/reasonCode 枚举）零变更

## 5. 文档

<!-- doc-impact: none(harness 机制文档不在 doc-impact 七域；pi-extensions.md 与开发执行规范属 harness 机制/流程文档，随本 change 就地同步) -->

- 无 flow 影响（纯 harness change，不涉业务域，§12.2 变更溯源豁免）

- [x] 5.1 `docs/reference/harness/pi-extensions.md`：spec-gate 条目补「归档 warn 边沿触发（同指纹会话内至多一条、转绿收尾、compact 重发）」一段
- [x] 5.2 `docs/reference/开发执行规范.md` §4 门禁分层表中 spec-gate warn 行措辞与新行为对齐（先 grep 核实旧表述，无则记一句核实结论）

## 6. 验证

- [x] V1 `bash .pi/extensions/tests/run-harness-smoke.sh` → 退出码 0
- [x] V2 `bash scripts/harness/doc-impact.sh verify openspec/changes/edge-trigger-archive-gate-warns && bash scripts/harness/scenario-trace.sh openspec/changes/edge-trigger-archive-gate-warns` → 均退出码 0
- [x] V3 `openspec validate edge-trigger-archive-gate-warns --strict` → 通过
- [x] V4 `git status --short` → 本 change 变更仅含 `.pi/extensions/spec-gate.ts`、`.pi/extensions/tests/spec-gate.smoke.cjs`、两份文档、`openspec/changes/edge-trigger-archive-gate-warns/`；其他脏文件为并发改动不纳入
- [x] V5 手工核对一次真实归档 dry-run（任选一个待归档 change 跑 `bash scripts/harness/archive-readiness.sh <name>` 自查，不真归档）→ 门禁脚本链路无回归

### 回检指标（归档后 7 天窗口，`harness-retro` 复查）

| 回检指标 | 基线（2026-09-23） | 期望 | 观察窗口 |
| --- | --- | --- | --- |
| 同 `session_id+change` 的 `concurrent-dirty-tree` warn 多条对数 | 5 对 | 0 | 上线后 7 天 |
| `spec-gate` warn 计数/7天（retro ④段口径 31） | 31 | ≤15 | 上线后 7 天 |
| ⑤/⑤' 转绿收尾行出现次数 | 无此通道 | ≥0（通道存在且用例覆盖） | smoke 断言（归档前） |

### Scenario→测试文件映射（scenario-trace 对账）

| Scenario | 测试文件 |
| --- | --- |
| 树上有其他 change 归属文件时 warn | .pi/extensions/tests/spec-gate.smoke.cjs |
| 冷启动跳过 | .pi/extensions/tests/spec-gate.smoke.cjs |
| 同指纹重试静默 | .pi/extensions/tests/spec-gate.smoke.cjs |
| 清单变化重新输出 | .pi/extensions/tests/spec-gate.smoke.cjs |
| 转净收尾一行 | .pi/extensions/tests/spec-gate.smoke.cjs |
| 新会话首见重新提醒 | .pi/extensions/tests/spec-gate.smoke.cjs |
| 无违例静默放行 | .pi/extensions/tests/spec-gate.smoke.cjs |
| 白盒用例缺失 warn 留痕 | .pi/extensions/tests/spec-gate.smoke.cjs |
| 纯函数任务提 SQLite warn 留痕 | .pi/extensions/tests/spec-gate.smoke.cjs |
| 检查⑤异常不阻断归档 | .pi/extensions/tests/spec-gate.smoke.cjs |
| 豁免通道兼容 | .pi/extensions/tests/spec-gate.smoke.cjs |
| 同指纹重试不重复投递 | .pi/extensions/tests/spec-gate.smoke.cjs |
| 违例集合变化重新投递 | .pi/extensions/tests/spec-gate.smoke.cjs |
| 违例清零收尾一行 | .pi/extensions/tests/spec-gate.smoke.cjs |
