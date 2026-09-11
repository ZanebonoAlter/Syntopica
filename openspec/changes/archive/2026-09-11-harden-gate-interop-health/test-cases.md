# Test Cases: harden-gate-interop-health

主链路故事（agent 视角）：改后端代码 → turn_end 门禁前先探测 interop → 健康 → 照常跑门禁（行为与引入前一致）｜WSL 故障期 → 探测超时 → 整轮 cmd 链路门禁跳过 + steer 提示"WSL interop 环境故障，非代码问题" + 账本记 policy.decision(interop-down) 且零 gate.check 假失败 → 下 turn 纯对话不再被粘性重跑轰炸 → 探测通过但命令中途挂 → diag 特征命中 → 不进粘性 + 环境归因 → 真实代码失败照旧 [回归] 催修 + 全部 cmd 链路失败提示带（wsl环境）标注。

## 0. 契约继承与调整（⓪ 改契约了吗）

本 change MODIFIED `harness-fact-log` 的「策略显著裁决统一记账」Requirement。旧资产反查：`bash scripts/test-assets.sh harness-fact-log`（无自动化测试资产，词汇契约靠 sqlite 断言 + skill 文档锚定）。

| 旧 Scenario | 处置 | 旧测试 | 动作 |
|---|---|---|---|
| spec-gate 阻断归档被记录 | 保留原文 | 无（人工 sqlite 断言） | 无动作 |
| spec-gate 显式豁免被记录 | 保留原文 | 无 | 无动作 |
| quota-gate 阻断与 fail-open 被区分 | 保留原文 | 无 | 无动作 |
| test-scope 软硬模式被记录 | 保留原文 | 无 | 无动作 |
| 正常放行零记录 | 语义收窄（补"探测健康本身零记录"） | 无 | 实现 1.1 验证时顺带断言探测健康零 policy.decision |
| 记账故障不改变裁决 | 保留原文 | 无 | 无动作 |

新增 gate-interop-health capability 全部 ADDED，无旧测试遗留。

## 1. 主链路表（节拍）

| 步 | 动作 | 来源 Scenario | 期望 | 层 | 落点 |
|---|---|---|---|---|---|
| 1 | 改后端代码触发 turn_end（健康环境） | 探测成功不改变既有门禁行为 | 探测通过（~40ms），lint/vet/build/域测试照常执行，gate.check 落库，无 policy.decision | 真实触发 + sqlite 断言 | tasks 1.1 |
| 2 | 故障期触发 turn_end | 探测失败短路整轮 cmd 链路门禁 | 后端+pnpm lint 全跳过，steer 提示环境故障与重启建议，无门禁命令失败记录 | 故障注入（探测超时调 1ms）+ sqlite | tasks 1.2 |
| 3 | 短路后查账本 | interop 探测短路被记账且不双写 | 一条 policy=quality-gate、action=fail-open、reasonCode=interop-down 的 policy.decision，durationMs 非负；本轮零新增 gate.check | sqlite 断言 | tasks 1.3 |
| 4 | 故障后下 turn 纯对话 | 探测失败短路（steer 含恢复建议） | 回合正常结束不被阻断；环境故障未进粘性 → 下 turn 不重跑 | 故障注入 + 观察 | tasks 1.2 |
| 5 | 探测通过但命令执行中挂（diag 含 UtilAcceptVsock） | 环境故障失败不触发粘性重跑 | 该命令失败不进粘性，下 turn 纯对话不重跑，steer 归因环境 | 手工构造 diag（node 正则单验） | tasks 2.1 |
| 6 | 真实编译错误 | 真实代码失败保持既有语义 | 进粘性、[回归]/[中间态] 分级、下 turn 重跑催修 | 真实触发（引入临时编译错误） | tasks 2.2 |
| 7 | 观察任一 cmd 链路失败 steer | cmd 链路失败提示含链路标注 | 提示行含（wsl环境）标注；真实失败修复义务不受弱化 | 观察 1/6 场景 steer | tasks 3.1 |

## 2. 变体走查（五组固定清单）

- **输入变体**：diag 为空串（无特征，走既有失败路径，不误判环境）｜diag 仅含 `UtilAcceptVsock` 无 `<3>WSL` 前缀（双关键字锚定下仍命中——关键字一：`UtilAcceptVsock`，关键字二：`<N>WSL (… - ) ERROR` 前缀形态；实现按"任一命中即环境故障"还是"同时命中"须与 design D3 一致：双锚定意为两形态任一即中，取并集防单形态漂移）→ node 正则单验；diag 正常输出含 "WSL" 字样但非 interop 前缀形态（如错误消息引用 WSL 路径）→ 不命中，node 正则单验
- **前置变体**：纯对话 turn 无粘性（不探测，零开销 return）｜纯对话 turn 有粘性（真实失败遗留 → 仍探测并重跑催修）｜故障期后恢复（探测通过 → 门禁恢复执行，无需人工复位）→ 故障注入观察
- **时间窗口**：探测恰在 2s 边界（1.9s 慢但成功 → 放行执行；2.1s → 超时短路）→ 由 pi.exec timeout 语义保证，理论值不实测留痕；故障持续 40min 期间每有代码改动的 turn 探测各失败一次（每 turn ≤1 次探测，不叠加）
- **幂等**：连续多 turn 故障短路（每 turn 各记一条 policy.decision，不合并不漏）→ sqlite 计数；同一故障期探测不产生 gate.check（跳过的命令零记账，重复验证不双写）→ sqlite 断言；恢复后同 turn 内探测只跑一次（后端前端共享）→ 代码审查确认单次调用点
- **可用性（UI 前三必检，本 change 无 UI，划除误输入/空态）**：错误态（探测抛异常 → fail-open 短路不抛错给 agent）→ 故障注入（mock throw）验证；加载态（探测期间 turn_end 等待 ≤2s 上界）→ 代码确认 timeout 参数。超长文本（diag 超 truncateDiagGate 截断后特征仍可识别——特征识别须在截断前做或特征位于 stderr 头部天然保留）→ 代码审查 + node 单验

## 3. 层选择说明

- 特征正则纯逻辑：node 一次性脚本单验（不建 harness 单测基建，.pi/extensions gitignored，design D6）
- 短路/豁免/记账分支：故障注入（探测超时参数临时调 1ms 或环境变量开关）+ sqlite 断言 events.db
- 既有行为不变：真实触发 turn_end（改一行后端代码 + 临时引入编译错误再修复）
- opencli 端到端：N/A——无前端交互；主链路落点为真实 turn_end 触发，等价覆盖

## 4. 效果核对（效果依赖断言外因素）

环境故障的复现依赖 WSL vsock 状态（不可控）：部署后观察期内（下一次 9/3 型故障期）核对——故障期 gate.check 失败不再成串出现（取而代之 policy.decision interop-down 计数）、故障期 steer 消息无 [回归] 假归因、恢复后门禁自动续跑。日常量化：`sqlite3 events.db "SELECT substr(ts,1,10), COUNT(*) FROM events WHERE kind='policy.decision' AND json_extract(payload,'$.reasonCode')='interop-down' GROUP BY 1;"` 对比同期 gate.check ok=false 且 diag 含 UtilAcceptVsock 的条数（后者应趋零）。

## 5. 展示字段盘点（改数据结构）

无用户可见界面字段。agent 可见的 steer 提示新增两类内容，来源语义锚：（wsl环境）链路标注（specs·cmd 链路失败提示显式标注执行链路）、环境故障归因与恢复建议（specs·探测失败短路/环境故障识别）。policy.decision 新增 reasonCode=interop-down（specs·interop 探测短路被记账）。全部有 Requirement 锚，无隐式契约。

## 6. 白盒附加（复杂档：分支表 + 边界值）

分支表（turn_end 主流程，quality-gate.ts）：

| # | 分支 | 条件 | 期望路径 | 验证 |
|---|---|---|---|---|
| B1 | 无门禁需求 | 无 cmd 链路触发且 sticky=∅ | return，不探测零开销 | 代码审查（既有 early return 语义保持） |
| B2 | 探测成功 | cmd echo exit 0 且 <2s | 既有门禁流程原样 | 主链路步 1 |
| B3 | 探测超时 | pi.exec timeout 2s 触发 | 短路：跳过全部 cmd 链路门禁 + steer 环境提示 + policy.decision(interop-down) + 零 gate.check | 主链路步 2/3 |
| B4 | 探测 exit≠0 | echo 返回非零（未超时） | 同 B3 | 故障注入 |
| B5 | 探测抛异常 | pi.exec reject | 同 B3（catch 后与超时同路径，fail-open 不抛出） | 故障注入（mock throw） |
| B6 | 命令失败·环境特征 | gateLog code≠0 且 diag 命中特征 | 不 add sticky、不推 [回归]/[中间态] failures、单独环境故障提示段、gate.check ok=false 照记 | 主链路步 5 + node 正则单验 |
| B7 | 命令失败·真实 | code≠0 且 diag 未命中 | 既有语义：add sticky + 分级 steer + gate.check | 主链路步 6 |
| B8 | 环境失败后下 turn | sticky 不含该命令且无新触发 | 纯对话不重跑（粘性豁免生效） | 主链路步 4 |
| B9 | 前端侧短路 | 探测失败但仅前端触发 | pnpm lint 也跳过（同一 interop 通道） | 代码审查（探测点位于两侧分派前） |

边界值：

- 探测超时：2000ms（常量 PROBE_TIMEOUT_MS；1.9s/2.1s 理论边界由 pi.exec 保证，不实测留痕）
- 特征正则：真实样本 `<3>WSL (1751 - ) ERROR: UtilAcceptVsock:251: accept4 failed 110` 必中；`WSL` 普通字样（非前缀形态）必不中；空 diag 必不中；截断后 diag（tail 30 行窗口）头部特征仍在（interop stderr 出现在输出最前部）——node 单验覆盖
- durationMs：探测耗时毫秒数 ≥0；记账时若 Date.now() 异常返回 NaN → 序列化前钳 0（事件 payload MUST 数字）
- 不适用划除：并发/线程安全（turn_end 单线程串行，N/A）；重试语义（故障持续期重试无意义，design 明确不做，N/A）
