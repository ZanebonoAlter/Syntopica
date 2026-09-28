# test-cases — add-foreign-gate-steer-attribution

主链路故事：一个零编辑会话遭遇 foreign 中间态失败 → 首轮完整提示 → 后续动态值轮次静默 → 归因降级可考古 → 转绿收尾。测试层：纯函数（node strip-types 直跑断言）+ 记账契约（smoke fixture）。

**断言锚（双向引用）**：纯函数层 = `.pi/extensions/tests/failure-fingerprint.smoke.ts`（S1-1~S1-6 / W1~W8+W4b/W4c/W5b / D1~D4，node --experimental-strip-types 直跑）；行为层 = `.pi/extensions/tests/quality-gate.behavior.smoke.cjs` 本 change 新增场景 **FP**（FP1~FP4）/ **OW**（OW-a1~a2 / OW-c1~c3 / OW-b1~b3）/ **T**（T1~T5）；既有回归面 = 同文件场景 L/M/N/O/B4（aggregate-concurrent-gate-warns / attribute-concurrent-gate-noise / tune-quality-gate-concurrency 先例）。

## 主链路表（故事 S1：动态值风暴下的边沿触发）

| 步 | 动作 | 来源 Scenario | 期望 | 层 | 落点与断言锚 |
| --- | --- | --- | --- | --- | --- |
| 1 | 首次失败（diag=时间戳日志行 A） | 首次失败输出完整块 | 完整块注入一次（分级前缀+tail 30） | smoke 行为断言 | quality-gate failureReports → **FP1** |
| 2 | 次轮失败（diag=同源日志行，仅时间戳/端口/毫秒不同） | 运行时动态值不构成新指纹 | 零注入；记账 diag 保留原文 | 纯函数+行为 | normalizeDiagFingerprint → **S1-1/S1-2/W1~W3/W8**；行为零注入 → **FP2**；记账原文 → **FP3** |
| 3 | 第 N 轮失败特征真实变化（错误文本主体不同） | 首次失败输出完整块（指纹变化视同首次） | 重新完整块注入 | 纯函数+行为 | normalizeDiagFingerprint → **S1-3/W6/W4b**；行为重新注入 → **FP4** |
| 4 | 转绿 | 失败转绿输出收尾一行 | 单行「已转绿」，指纹状态清除 | smoke | gateLog ok 分支 → 既有 **L5**（不回归） |
| 5 | 纯对话回合同失败仍在 | 粘性重跑语义不变 | 命令仍重跑、指纹相同零注入 | smoke | stickyFailures → 既有 **L3/L4**（不回归）+ **T4** 粘性重跑零重复注入 |

## 主链路表（故事 S2：归属判定与降级可观测）

| 步 | 动作 | 来源 Scenario | 期望 | 层 | 落点与断言锚 |
| --- | --- | --- | --- | --- | --- |
| 1 | 失败路径 ⊆ 启动基线、mine 为空 | 测试运行期失败按包锚点判外部 | [外部] 一行提示、不进粘性、foreign-breakage 记账、gate.check ownership=foreign | 复现场景+fixture | classifyFailureOwnership/gateLog → 既有 **N1~N6/R1** + 本 change 复现固化 **T1~T3**（含无绑定会话 edit.map 归属 Fix C） |
| 2 | 归属信号缺席（基线空/库不可用） | 归因信号缺席显式记账 | 保守 mine + attribution-degraded 一条 + ownership=attribution-degraded | fixture | quality-gate 归因降级点 → **OW-b1~b3**（基线缺席会话级单发） |
| 3 | 判定过程抛异常 / 解析不出 | 路径不可解析时保守按本会话失败处理 | 视同 mine + 降级记账不静默 | fixture | classify catch 路径（防御层，无自然触发）+ P=∅ → **OW-c1~c3**（(cmd,fp) 边沿去重） |
| 4 | 存量事件（无 ownership 键）被查询 | 存量事件无 ownership 消费兼容 | 判空回退不报错 | fixture | 消费侧查询 → §6 验证 3（临时库 json_extract 判空）+ SKILL.md 存量回退条目 |
| 5 | 四态各落一条 | 失败记账携带归属 / 归属降级显式标注 | mine/foreign/mixed/attribution-degraded 各一 | fixture | **OW-a1**（mine/foreign/mixed 三态）+ **OW-b1**（attribution-degraded） |
| 6 | 修复后同等场景判外部 | （design D4 修复目标） | 零编辑会话 + 物主登记文件失败 → foreign；无人归属调用点 → 保守 mine 单块 | 复现场景固化 | **T1~T5**（修复前观察 mine 留痕 research findings；修复后 foreign + 保守残余锁定） |

## 变体走查

- 输入：空 diag（提取不到行）→ 归一化输出空串，与空串指纹相等（同失败静默）；纯时间戳行（无错误主体）→ 归一化后为空串族，视同同指纹（保守多静默优于逐轮轰炸，红态知情走查询入口）。
- 幂等：同一输出字节重复调用归一化 → 输出恒等（确定性断言）；规则应用两次 → 与一次相同（规则幂等）。
- 时间窗口：时间戳跨日（22:59→23:00）→ 仍同指纹（数字序列压平覆盖）。
- 不适用划除：前置/越界（无外部依赖状态）；并发（归一化纯函数无线程语义）；UI 可用性（无 UI）。

## 白盒附加（normalizeDiagFingerprint 分支表 → failure-fingerprint.smoke.ts W 系列断言）

| 分支 | 输入样本 | 期望 | 断言锚 |
| --- | --- | --- | --- |
| 日志时间戳剥除 | `2026/09/28 22:33:46 INFO icon...` → `2026/09/28 22:35:39 INFO icon...` | 归一化后相等 | W1/W1b |
| IP:port 剥除 | `http://127.0.0.1:34505/favicon.ico` vs `:38117` | 相等 | W2/W2b |
| 耗时/毫秒剥除 | `(0.02s)` vs `(0.03s)`；`[0.029ms]` vs `[0.049ms]` | 相等 | W3 |
| 数字序列压平 | `FAIL pkg 10.793s` vs `FAIL pkg 9.283s` | 相等 | W4 |
| 标识符邻接数字保留（实现修正：错误文本主体不折叠） | `e1` vs `e7`；`models2` vs `models3`；`go1.27` vs `go1.30` | 不相等（宁可多报） | W4b/W4c |
| 仓库根前缀剥除 | `/home/x/Syntopica/backend-go/a.go` vs `backend-go/a.go` | 相等 | W5（非本仓库绝对路径不剥 → W5b） |
| 错误主体保留 | `undefined: modelsA` vs `undefined: modelsB` | 不相等（重新全文注入） | W6 |
| 空输入 | `""` | `""` | W7（纯时间戳行 → S1-6） |
| 确定性 | 同输入两次调用恒等；规则幂等；null/undefined 安全 | 恒等/幂等/空串 | D1~D4 |
