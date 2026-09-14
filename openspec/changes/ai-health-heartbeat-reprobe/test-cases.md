# Test Cases: ai-health-heartbeat-reprobe

主链路故事（常驻拓扑用户故事）：树莓派常驻后端，PC 上的 AI 按需启停——
「PC 关机 → 分析自动暂停不烧重试；PC 开机 → 自动恢复补处理；全程零手动」。

## 主链路表（步/动作/来源 Scenario/期望/层/落点）

| # | 步/动作 | 来源 Scenario | 期望 | 层 | 落点 |
|---|---|---|---|---|---|
| 1 | Pi 启动，PC 关机，启动探测 | ai-model-health「启动时探测每条路由主 provider」+「ListRoutes 瞬态失败时重试」 | 探测每路由主 provider；全拒 → 快照 not healthy | 单测 | `aihealth_test.go` 既有启动探测用例（保持绿） |
| 2 | 门禁联动 | （隐含，IsPaused 公式） | not healthy → IsPaused=true → 分析 job 不跑 | 集成(testcontainer) | `health_gate_compose_test.go::TestHealthGate_ProbeUnhealthy_PauseAwareSkips`（保持绿） |
| 3 | PC 开机，心跳探通 | 「降级后自动恢复」/「快照不健康时定时复检直至自愈」 | 单次成功 → healthy → IsPaused=false | 单测 | `reprobe_test.go::TestStartPeriodicReprobe_RetriesWhileUnhealthy_*` 前半段（改造保留）+ compose happy path（既有） |
| 4 | healthy 持续心跳 | 「健康态心跳持续探测」（ai-health-reprobe）+「快照健康后仍按心跳周期复检且可降级」（ai-model-health） | healthy 后探测计数**继续增长**（反转旧断言） | 单测 | `reprobe_test.go::TestStartPeriodicReprobe_HeartbeatContinuesWhenHealthy`（改写自旧 StopsWhenHealthy 后半段） |
| 5 | PC 中途关机，秒拒 ×1 | 「端点秒拒连续两次即降级」（去抖窗口） | 第 1 次失败后快照**仍 healthy**（去抖） | 单测 | `aihealth` 新增降级测试（failStreak=1 分支） |
| 6 | 秒拒 ×2 | 同上 | 第 2 次失败 → 降级 not healthy → IsPaused=true | 单测 + 集成 | 单测降级测试 + `health_gate_compose_test.go::TestHealthGate_HeartbeatDegrade_PauseAwareSkips`（新） |
| 7 | 降级后 PC 回来 | 「降级后自动恢复」 | 单次探通 → 恢复 healthy | 单测 | 新增恢复测试（fake probe 翻转） |
| 8 | 慢而活的服务器 | 「忙服务器不被误降级」 | 响应慢但在 provider timeout 内 → 计成功，不触发降级 | 集成 | `health_gate_compose_test.go::TestHealthGate_SlowProviderWithinTimeout_Healthy`（新，httptest delay server） |
| 9 | 慢加载模型自愈 | 「慢加载模型加载完成后自动自愈」 | not healthy 期间持续重探直至探通（60s 节奏不变） | 单测 | 既有重探测试前半段（保持） |
| 10 | 探测 in-flight | 「探测 in-flight 时定时触发被跳过」 | tick 被跳过不并发 | 单测 | `TestStartPeriodicReprobe_InFlightTickSkipped`（既有，保持绿） |
| 11 | 心跳失败触发拉起 | 「心跳失败触发拉起且遵守冷却」 | 复用 TryStartProbe 同一入口 → 拉起/冷却链路行为不变 | 单测 | 既有拉起/冷却用例（保持绿）；心跳与手动共用入口由 #10 结构保证 |
| 12 | 仅探主 provider / 无路由跳过 / 宽松口径边界 | ai-model-health 既有三场景 | 语义不变 | 单测/集成 | 既有用例保持绿（`TestHealthGate_EmbeddingUp_LLMDown_NotHealthy` 等） |

## 继承与调整（⓪ 改契约了吗 → 改了，REMOVED+ADDED 重立）

| 旧资产 | 处置 | 动作 |
|---|---|---|
| `TestStartPeriodicReprobe_RetriesWhileUnhealthy_StopsWhenHealthy` | **改造**：前半段（unhealthy 持续重探）语义保留；后半段"healthy 后计数不增长"断言被 spec 反转 | 拆为「unhealthy 自愈」+「healthy 持续心跳」两个用例，后者断言计数增长 |
| 旧 spec scenario「快照已健康时不再定时探测」「快照健康后不再周期性复检」 | 随 REMOVED requirement 退役 | 无对应旧测试资产（旧断言即上行的后半段，已随改造处置） |
| 其余 aihealth/analysispause 既有用例 | 原样保持 | 回归确认零回退 |

## 变体走查（五组固定清单）

- 输入（空串/空白/分隔符/单token/大小写/特殊字符/超长）：**不适用**——探测无字符串输入，provider 配置校验属 UpsertRoute 既有链路。划除。
- 前置（空集/单元素/重复/越界/部分满足）：
  - 空集（无路由）：既有 spec「未配置任何路由不健康」→ 既有测试保持绿；
  - 单元素（仅 embedding 通）：`TestHealthGate_EmbeddingUp_LLMDown_NotHealthy` 既有；
  - 重复（同 provider 多路由共享）：既有"只探/拉一次"用例保持；
  - 部分满足：同 EmbeddingUp_LLMDown。
- 时间窗口（边界两端/空窗口/跨窗口/归一化）：
  - failStreak 边界：1（不降，去抖窗）vs 2（降级）——核心新用例；
  - **成功重置边界**：fail→success→fail→fail —— 第二个 fail 序列从 streak=0 重计，第 3 步首次 fail 不降级、第 4 步才降级（防"历史失败残留"误降）；新增用例覆盖；
  - **跨触发源累计**：心跳 fail ×1 + 手动 reprobe fail ×1 → 降级（failStreak 归 probeMu 域、不区分触发源）；新增用例；
  - not-ready（CheckedAt=nil）首探失败：不走去抖直接 not healthy（启动竞态 fail-closed，既有语义）；既有测试 + 白盒表行 2。
- 幂等（重复执行/部分失败重试/并发）：
  - 重复 tick：in-flight 跳过（既有）；ListRoutes 部分失败重试（既有 3 次退避）；并发探测互斥（既有 probeMu）——线程安全声称由互斥结构保证。
- 可用性（UI 必检前三：误输入反馈/空态/错误态/加载态/超长/重复提交）：**UI 不在本 change 范围**（ui-impact:none，降级态复用既有 AiHealthBanner 渲染链）。划除留痕。

## 白盒附加（复杂档：状态机分支表 + 边界值）

状态机：`(prev: not-ready|healthy|not-healthy) × (探测: ok|fail|listErr)`，failStreak ∈ {0,1,≥2}：

| # | prev | 探测 | failStreak 动作 | 快照动作 |
|---|---|---|---|---|
| 1 | not-ready | ok | =0 | healthy=true |
| 2 | not-ready | fail | 置 0（基线，不累计） | healthy=false（**无去抖**，启动 fail-closed） |
| 3 | healthy | ok | =0 | healthy=true |
| 4 | healthy | fail（streak→1） | =1（<2） | **healthy=true 保持**，Routes/CheckedAt 更新（去抖窗） |
| 5 | healthy | fail（streak→2） | =2（≥2） | healthy=false（降级） |
| 6 | not-healthy | fail | 保持（无意义） | healthy=false |
| 7 | not-healthy | ok | =0 | healthy=true（恢复） |
| 8 | any | listErr | 不参与去抖（DB 故障保守路径） | healthy=false（现状原样） |

边界值：`degradeFailures=2`（1 不降 / 2 降）；成功清零后重计（见变体-时间窗口）；降级后 streak 对后续 fail 不再影响（行 6）。

不适用划除：failStreak 上溢（int、真实节奏 2 即封顶）；多进程共享 streak（快照/streak 均进程内存态，单实例假设既有）。

## 效果核对（真实环境，任务 3.2）

依赖断言外因素（真实网络断联节奏、前端轮询周期）→ 人工：本地起后端 + provider base_url 指向不监听端口，观察 ≤2 心跳周期内 `/schedulers/status.ai_healthy` 翻 false、任务停租约；恢复端点后 ≤60s（单次探通）翻 true。时间线记录回填本节。
