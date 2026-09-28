## 1. 指纹归一化（纯函数层）

- [x] 1.1 `.pi/extensions/lib/failure-classify.ts` 新增 `normalizeDiagFingerprint(text)`（design D1 五规则：日志时间戳 / IP:port / 耗时括号与毫秒 / 连续数字序列压平 / 仓库根前缀剥除），不改动 `truncateDiagGate` 记账口径；新增 smoke 断言文件（node --experimental-strip-types 直跑 import 断言），用例含 2026-09-28 23 轮实账 diag 样本（`2026/09/28 22:33:46 ...` 系列逐条断言归一化后相同）与「错误文本主体不同则指纹不同」反例，验证 `node --experimental-strip-types` 断言全绿
- [x] 1.2 归一化函数确定性断言：相同输入字节两次调用输出相同、规则序无关输入稳定性，进同一断言文件

## 2. quality-gate 接线与记账

- [x] 2.1 `.pi/extensions/quality-gate.ts` failureReports 比较点切换为归一化指纹（`prev.diag === diag` 的 diag 改经 normalizeDiagFingerprint；failureReports 存储仍可存原文，比较时双归一或存归一值，取实现最小路径），完整块展示与 gate.check 记账 diag 保留原文，验证方式见 4.1 smoke 行为断言
- [x] 2.2 gateLog 失败分支 payload 新增 `ownership` 字段（mine/foreign/mixed 透传 classify 结果；catch 回退路径记 attribution-degraded），验证 fixture 侧四态各落一条
- [x] 2.3 归因降级记账：classify catch 路径与 baselinePaths 为空的会话级场景记 `policy.decision(policy=quality-gate, action=warn, reasonCode=attribution-degraded, target=cmd)`，同 (cmd, fingerprint) 去重口径对齐 foreign-breakage，验证降级不与 gate.check 失败事实双写

## 3. mine 误判根因定位与修复（design D4：先记后修）

- [x] 3.1 构造复现场景（新会话启动前另一会话改某包文件、新会话零编辑触发该包失败）观察 ownership 实际取值，按嫌疑清单（baselinePaths 启动快照时序 / expandPkgAnchorDirs 展开误吸 / expandSideForms 形态错配）定位断点，结论留痕 docs/research/harness-gate-steer-attribution/explore-findings.md
- [x] 3.2 按定位结果修复（目标：同等场景判 foreign 或 mixed；若根因在 pi 扩展宿主时序不可修，降级目标为确保 attribution-degraded 可考古并在 research 留痕移交后续），复现场景固化为回归断言

## 4. 测试

- 本 change 为 harness 工具链改造，交付验证 = smoke（归一化断言文件 + 词汇稳定集断言 + 存量兼容 fixture）+ 复现场景人工验证留痕；测试设计见 test-cases.md（纯函数层表驱动 + 记账契约层 fixture），用例故事与 smoke 对齐 aggregate-concurrent-gate-warns 先例（test-cases.md 故事编号被 smoke 引用）。

## 5. 文档

<!-- doc-impact: none（harness 层，无业务域） -->

- [x] 5.1 `.agents/skills/harness-facts/SKILL.md`：gate.check payload 字段表补 `ownership`（四态 + 存量无键回退）、policy.decision 词汇表补 `attribution-degraded`
- [x] 5.2 `docs/reference/harness/pi-extensions.md`：quality-gate 节同步指纹归一化口径与降级记账
- [x] 5.3 test-cases.md 用例与 smoke 断言双向引用核对（故事编号 ↔ smoke 注释）

## 6. 验证

本项为 harness 工具链改造，交付验证 = smoke（归一化断言文件 + 词汇稳定集断言 + 存量兼容 fixture）+ 复现场景人工验证留痕；测试设计见 test-cases.md（纯函数层表驱动 + 记账契约层 fixture），用例故事与 smoke 对齐 aggregate-concurrent-gate-warns 先例（test-cases.md 故事编号被 smoke 引用）。

<!-- doc-impact-excuse: flow=edit.map 存量归属混入他会话 overhaul 前端文件（Fix A 落地前的 trigger 未过滤旧账，见 docs/research/harness-gate-steer-attribution/explore-findings.md「跨会话噪声污染」），启发式反向命中为误报；本 change 纯 .pi/harness 层无 front/业务域文件 -->

| Scenario | 测试文件 |
| --- | --- |
| 首次失败输出完整块 | .pi/extensions/tests/quality-gate.behavior.smoke.cjs |
| 同指纹持续零注入 | .pi/extensions/tests/quality-gate.behavior.smoke.cjs |
| 运行时动态值不构成新指纹 | .pi/extensions/tests/failure-fingerprint.smoke.ts, .pi/extensions/tests/quality-gate.behavior.smoke.cjs |
| 红状态可经查询入口获知 | scripts/harness/gate-status.smoke.sh |
| 失败转绿输出收尾一行 | .pi/extensions/tests/quality-gate.behavior.smoke.cjs |
| 粘性重跑语义不变 | .pi/extensions/tests/quality-gate.behavior.smoke.cjs |
| 失败路径全落在会话启动基线时判外部 | .pi/extensions/tests/quality-gate.behavior.smoke.cjs |
| 测试运行期失败按包锚点判外部 | .pi/extensions/tests/quality-gate.behavior.smoke.cjs |
| 失败路径含本会话触发文件时维持代码失败语义 | .pi/extensions/tests/quality-gate.behavior.smoke.cjs |
| 路径不可解析时保守按本会话失败处理 | .pi/extensions/tests/quality-gate.behavior.smoke.cjs |
| 归因信号缺席显式记账 | .pi/extensions/tests/quality-gate.behavior.smoke.cjs |
| 归属其他 active change 的文件被识别为外部 | .pi/extensions/tests/quality-gate.behavior.smoke.cjs |
| 外部失败不进粘性且记 warn 归因 | .pi/extensions/tests/policy-decision.smoke.cjs |
| 同指纹外部失败不重复提示 | .pi/extensions/tests/quality-gate.behavior.smoke.cjs |
| 门禁命令失败记账 | .pi/extensions/tests/quality-gate.behavior.smoke.cjs |
| 失败记账携带归属 | .pi/extensions/tests/quality-gate.behavior.smoke.cjs |
| 归属降级显式标注 | .pi/extensions/tests/quality-gate.behavior.smoke.cjs |
| 存量事件无 ownership 消费兼容 | .pi/extensions/tests/quality-gate.behavior.smoke.cjs |
| 失败全量记账不采样 | .pi/extensions/tests/policy-decision.smoke.cjs |
| 转绿翻转锚点必记 | .pi/extensions/tests/policy-decision.smoke.cjs |
| 分母按采样口径还原 | .pi/extensions/tests/policy-decision.smoke.cjs |
| 同根因短路未执行不记账 | .pi/extensions/tests/quality-gate.behavior.smoke.cjs |
| 未运行不记账 | .pi/extensions/tests/quality-gate.behavior.smoke.cjs |
| diag 失败特征优先（stdout 噪声行不掩盖真实错误） | .pi/extensions/tests/failure-classify.smoke.cjs |
| go test 记账不丢 FAIL 行 | .pi/extensions/tests/run-harness-smoke.sh |
| spec-gate 阻断归档被记录 | .pi/extensions/tests/spec-gate.smoke.cjs |
| spec-gate 显式豁免被记录 | .pi/extensions/tests/spec-gate.smoke.cjs |
| quota-gate 阻断与 fail-open 被区分 | .pi/extensions/tests/policy-decision.smoke.cjs |
| test-scope 软硬模式被记录 | .pi/extensions/tests/policy-decision.smoke.cjs |
| dev-process-guard 孤儿治理事件按豁免形状记账 | .pi/extensions/tests/dev-process-guard.smoke.cjs |
| interop 探测短路被记账且不双写 | .pi/extensions/tests/policy-decision.smoke.cjs |
| 归因降级被记账且不双写失败事实 | .pi/extensions/tests/quality-gate.behavior.smoke.cjs |
| 正常放行零记录 | .pi/extensions/tests/policy-decision.smoke.cjs |
| 记账故障不改变裁决 | .pi/extensions/tests/policy-decision.smoke.cjs |

补充验证记录（2026-09-29 归档前复检，tasks 原验证清单 1~5 全绿）：

1. 归一化断言全绿：run-harness-smoke.sh 直跑 failure-fingerprint.smoke.ts → 23 轮实账样本归一化互相同（S1-1~S1-2）+ 确定性/幂等（D 系）。
2. smoke 回归：harness-retro.smoke.sh 67/67（attribution-degraded 登记情报型不吸入④段失效名单，L2b）。
3. 记账 fixture 四态：behavior smoke OW-a1（mine/foreign/mixed）+ OW-b1（attribution-degraded）；存量无键 json_extract NULL 回退不报错。
4. 词汇稳定：真库 policy.decision DISTINCT reasonCode 枚举与 harness-facts 词汇表一致，无漂移（attribution-degraded 暂无自然触发记录，属预期）。
5. 复现场景留痕：docs/research/harness-gate-steer-attribution/explore-findings.md 含 3.1 观察结论与 3.2 修复/降级决策记录（docs/research/ 被 .gitignore，本地留痕不入库）。
