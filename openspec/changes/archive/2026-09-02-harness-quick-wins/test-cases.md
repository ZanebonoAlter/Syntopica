# harness-quick-wins 白盒用例文档

> 复杂度声明：complex（依据：成功采样记账为状态机——`{consecOk, lastWasFail, everGreen}` 三字段、多分支转移，见白盒附加节）。

## 主链路：一个后端开发会话的门禁记账生命周期

| 步 | 动作 | 来源 Scenario | 期望 | 层 | 落点 |
| --- | --- | --- | --- | --- | --- |
| 1 | 会话内首次改后端文件，回合末门禁跑通（lint 无编译错 → vet/build/test 并行全绿） | 门禁命令失败记账（改造面） | 各命令落库 1 条 ok=1 且 payload `flip:true`（会话锚点，会话内该 cmd 首个成功必记） | smoke | quality-gate.smoke.cjs |
| 2 | 连续成功第 2~4 次 | 转绿翻转锚点必记（采样面） | 不落库 | smoke | quality-gate.smoke.cjs |
| 3 | 连续成功第 5 次 | 转绿翻转锚点必记 | 落库 1 条 `sampled:true, n:5`，计数回绕重起 | smoke | quality-gate.smoke.cjs |
| 4 | 某次命令失败 | 门禁命令失败记账 / 失败全量记账不采样 | 落库 ok=false + 512B 内 diag，consecOk 清零 | smoke | failure-classify.smoke.cjs（回归）+ quality-gate.smoke.cjs（清零断言） |
| 5 | 失败后修复成功 | 转绿翻转锚点必记 | 落库 `flip:true`（转绿锚点与步 1 会话锚点同规则） | smoke | quality-gate.smoke.cjs |
| 6 | 曾绿命令再失败（everGreen=true） | （D4 新契约，AGENTS.md 锚） | steer 文案含 `[回归]`，强语气 | 人工 | 会话内观察 |
| 7 | 从未绿的命令失败（everGreen=false） | （D4 新契约） | steer 文案含 `[中间态]`，轻提示 | 人工 | 会话内观察 |
| 8 | lint 输出含 typechecking error | 同根因短路未执行不记账 | 仅 1 条 golangci-lint ok=false 事件；vet/test 未执行零记账，其状态不动 | smoke | quality-gate.smoke.cjs |
| 9 | 回合只改 docs/ markdown | 未运行不记账（既有） | 零事件 | 人工 | sqlite3 复查 |
| 10 | 会话内 git diff 命中 front/eslint.config.* 后跑前端门禁 | （D1 兜底） | 该会话去 --cache 全量 lint | 人工 | 二次门禁时长对比 |

前端侧对应步 1'（pnpm lint 走 eslint --cache 门禁调用，成功同样按采样记账）落 quality-gate.smoke.cjs。

## 变体走查（五组固定清单）

- **输入变体**（门禁输入=命令输出）：diag 特征表既有覆盖（FAIL/error/#pkg/exit、超长截断 512B、控制字符剥离）；新增边界——lint 退出码 0 但含 warning 输出 → 不属编译失败特征，**不短路**，正常路径（防把 warning 误判编译错）。纯 typechecking error 无行号 → 命中特征照常短路。
- **前置变体**：会话前残留脏文件不触发首轮门禁（既有用例回归）；eslint 缓存文件不存在/被删 → 等价冷启动全量一次，行为不变；node_modules/.cache 目录不存在 → eslint 自建。
- **时间窗口变体**：~~不适用~~（无日期/窗口语义，采样"连续"仅按回合序不按时间）——划除。
- **幂等/并发变体**：短路判定确定性（同输出字节同判定，纯字符串匹配）；per-cmd 状态为会话内 Map，多 pi 窗口天然隔离；events.db 并发写有既有 WAL + busy_timeout 兜底。同一会话 reload（扩展重载）→ 会话内状态丢失，采样计数从会话锚点重新起算（可接受：锚点机制保证至少 1 条成功记录）。
- **可用性变体**（无 UI 面）：steer 文案两档可读性入主链路步 6/7 人工验证；超长 diag 已有 512B 截断。~~空态/加载态~~——无 UI，划除。

## 效果核对（量化）

| 指标 | 方法 | 基线（实施前实测） | 目标 |
| --- | --- | --- | --- |
| ok=1 事件量 | sqlite3 按 ts 日聚合 kind=gate.check AND ok=1 | ≈600 条/日（08-30/31 实测） | <200 条/日（-70%+） |
| pnpm lint 门禁时长 | 同文件二次门禁计时 | 22.3s 均值 | <10s |
| 短路回合命令数 | 编译错误回合事件数 | 3 条/回合（lint+vet+test 全红） | 1 条/回合 |

## 白盒附加：采样状态机分支表

状态 `s = (lastWasFail, consecOk, everGreen)`，初始 `(false, 0, false)` 但**会话锚点规则将首条成功视同转绿**（实现等价：初始 lastWasFail=true）。输入 = 本轮该命令 ok。

| 当前态 | 输入 | 动作 | 次态 |
| --- | --- | --- | --- |
| 任意, everGreen=false | ok=true | 记 `flip:true`（会话/转绿锚点同规则） | (false, 1, true) |
| 任意, everGreen=true, lastWasFail=true | ok=true | 记 `flip:true` | (false, 1, true) |
| (false, 1..4, true) | ok=true | 不记 | (false, consecOk+1, true) |
| (false, 5, true) | ok=true | 记 `sampled:true, n:5` | (false, 0→下一轮起 1, true)（计数回绕） |
| (false, *, true) | ok=false | 记 fail+diag；steer `[回归]` | (true, 0, true) |
| (true, 0, false) | ok=false | 记 fail+diag；steer `[中间态]` | (true, 0, false) |
| 短路回合 | — | lint 记 fail；vet/test 未执行：无动作无状态转移 | 各自态不变 |

**边界值**：① 连续成功恰第 5 次落库（第 4 次不落）；② 会话首条即失败 → 全程 `[中间态]` 文案、everGreen 恒 false；③ 失败/成功交替（最坏记账量）→ 每次成功都是锚点必记——交替本身即不稳定信号，全记是审计需要，非缺陷；④ N 不整除场景（如段内成功 7 次：锚点 1 + 第 5 次 1 = 落库 2 条，余 2 次进下段计数——**段内计数不跨失败保留**，失败清零后重起）；⑤ 短路回合 vet/test 的 consecOk 保持原值（未执行≠失败）。

## 继承与调整（⓪ 契约变更对账）

| 旧 Scenario（主 spec） | 处置 | 旧测试 | 动作 |
| --- | --- | --- | --- |
| 门禁命令失败记账 | 正文保留，ok=true 语义改采样 | .pi/extensions/tests/failure-classify.smoke.cjs | 回归不动（断言失败侧，兼容）；步 4 验证 |
| 未运行不记账 | 原文保留 | 人工（archive 映射） | 步 9 人工回归 |
| diag 失败特征优先 | 原文保留 | failure-classify.smoke.cjs | 不动 |
| go test 记账不丢 FAIL 行 | 原文保留 | failure-classify.smoke.cjs | 不动 |
| ~~ok=true 全量记账（旧正文条款）~~ | **REMOVED 语义**（改采样） | grep 实测 quality-gate.smoke.cjs / failure-classify.smoke.cjs **无 ok=1 计数断言** | 无旧断言需改；新断言全在 quality-gate.smoke.cjs |
| 修复循环每轮按新变化触发（旧 change 用例） | 粘性语义不动 | quality-gate.smoke.cjs | 回归跑绿（触发与记账解耦，D3） |
