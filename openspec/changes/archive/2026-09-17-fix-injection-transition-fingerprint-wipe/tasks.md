# Tasks — fix-injection-transition-fingerprint-wipe

<!-- doc-impact: none(纯工具链：pi 扩展内部状态机修复，不触前后端业务代码与 reference 文档) -->

## 1. 状态结构与写入点

- [x] 1.1 `ChannelState` 新增 `fingerprintRegime: string | null` 字段（注释：当前 sentFingerprint 各条目投递时的 stableSnapshotKey；null = 尚无动态投递）
- [x] 1.2 `newSessionState` / `resetSessionState` 初始化 `fingerprintRegime: null`
- [x] 1.3 `deliverDynamic` 在写 `state.channel.sentFingerprint = fingerprint` 处同步写 `state.channel.fingerprintRegime = stableSnapshotKey(state.mode, state.boundChange)`（仅在确有投递路径上写；force 且零增量时重写同值，无害）
- [x] 1.4 `inheritFromParent` 的 channel 拷贝带上 `fingerprintRegime`（浅拷贝值即可，string）

## 2. 清空条件收紧

- [x] 2.1 `before_agent_start` 快照重建分支：`if (isTransition)` 清空改为 `if (isTransition && state.channel.fingerprintRegime !== key)`，并更新 :2038 附近注释（说明 regime 判定与 mid-turn 切档窗口；「首次建快照不重置」护栏保留）
- [x] 2.2 代码头注释「混合通道状态」节补一行：指纹按 regime 标记、清空条件、2026-09-17 取证摘要（事件 29644-29652）

## 3. smoke 回归（constraint-injection.smoke.cjs）

- [x] 3.1 场景 A（bug 复刻）：stub 会话序列 `before_agent_start`（未激活，建 index 快照）→ `tool_execution_start` read skill 路径（requirements 激活，mid-turn）→ `tool_execution_start` edit 命中 fixture doc 的 doc-impact-applies（JIT 投递 1 条，markMessages）→ `before_agent_start`（下一 turn）：断言 system prompt 含新档位 header（稳定层重建发生）且 `dynamicText()` 为空（零重投）
- [x] 3.2 场景 B（反向护栏）：接 A，`tool_execution_start` edit `openspec/changes/<fixture>/`（edit-dir 绑定）→ `before_agent_start`：断言动态层全量重投（含已投递过的 JIT 节）
- [x] 3.3 场景 C（fork 变体，纯函数面或链路面择一）：构造继承后状态（快照 key 陈旧 + 指纹 regime 为新 key）→ 首个 `before_agent_start` 断言零重投；若走纯函数面则导出最小判定辅助并直跑断言
- [x] 3.4 `bash .pi/extensions/tests/run-smoke.sh` 全绿；`node .pi/extensions/tests/constraint-injection.smoke.cjs`（经 bundle）单文件亦绿（另做变异验证：恢复无条件清空 → A/C 红；永不清空 → B 红）

## 4. 验证与收尾

- [x] 4.1 影响面命令：本 change 只触 `.pi/extensions/`（TypeScript 扩展，无 go/前端包），质量门禁不命中；smoke 即验证（另经临时 tsconfig A/B 对比：tsc 错误集与改前完全一致，无新增类型错误）
- [ ] 4.2 回检：部署后 ≥3 个工作日跑 proposal 所引复验 SQL，代内重复组数归零（design.md §5）
- [x] 4.3 文档一致性：AGENTS.md 扩展表 constraint-injection 行为描述不受影响（修复使其符合既有描述），无需改动；若有表述漂移在归档前一并核对（已核对：「指纹 diff 驱动，稳态零投递」描述与修复后行为一致）
