<!-- complexity: simple -->
<!-- ui-impact: none -->

## Why

2026-09-28 实测：一个零代码编辑的探索会话被另一活跃 change（ai-provider-model-overhaul）的 reader/service 中间态失败连续注入 23 轮几乎逐字相同的门禁 steer，每轮约 2KB。门禁的失败重复抑制（failureReports 同指纹静默）与并发归属判定（[外部]/[并发] 分支）机制均已存在，但被两处失效击穿：①指纹取"首个失败特征行"做精确相等比较，测试运行期输出该行是带秒级时间戳的日志行，每轮必不同 → 抑制永不触发；②本应判 [外部] 的失败（本会话零编辑、失败路径 ⊆ 启动基线）全部被判 mine（[回归]/[中间态] 全量催修，账本零 foreign-breakage 记账）。AGENTS.md「[回归] 必须修」的语义在并发场景下把修复压力持续落在无责会话一侧，且归因异常被静默吞掉、事后无法考古。

## What Changes

- **指纹归一化比较**：failureReports 的同指纹判定改用归一化 diag（剥时间戳、端口号、耗时值、绝对路径前缀、数字序列等动态值）后再精确比较；展示与 gate.check 记账保留原文，仅比较路径归一化。同指纹连续回合零注入的既有契约从"依赖输出逐字节稳定"变为"对运行时动态值免疫"。
- **归属判定可观测性**：gate.check 失败记账 payload 新增 `ownership` 字段（mine/foreign/mixed）；归因过程异常（classifyFailureOwnership 的 catch 路径、baselinePaths 快照失败、包锚点展开空交）时显式记一条归因降级 policy.decision，不再静默回退 mine。先可观测，再修 mine 误判根因。
- **mine 误判根因定位**：以 2026-09-28 会话（session_id 01a0e7f1-21eb-76de-825c-5279cb5c5ea6，23 轮全判 mine）为样本，定位 baselinePaths 启动快照（compaction/reload 时序、ESM 模块缓存残留、session_start git 异常→空集）或 expandPkgAnchorDirs 展开侧的具体断点并修复。修复验收：同等场景（零编辑会话 + 失败路径全部位于启动基线）判 [外部] 或至少 mixed，不再进 [回归]/[中间态] 全量催修。

## Capabilities

### New Capabilities

（无）

### Modified Capabilities

- `gate-failure-reporting`: 同指纹静默的指纹定义从原始 diag 精确相等改为归一化 diag 相等（时间戳/端口/耗时等动态值不参与指纹）；归因异常不再静默回退 mine，须显式记账降级原因。
- `harness-fact-log`: gate.check 失败记账 payload 新增 `ownership` 字段；policy.decision 新增归因降级事件（policy=quality-gate，reasonCode 表达"归因信号缺席/异常"语义，具体词汇进 design 决策）。

## Impact

- `.pi/extensions/quality-gate.ts`：failureReports 比较逻辑、gateLog 记账、归因异常路径。
- `.pi/extensions/lib/failure-classify.ts`：新增归一化函数（truncateDiagGate 旁路，不改记账口径）。
- `.pi/harness/events.db`（运行时数据）：gate.check payload 增字段向后兼容（旧记录无 ownership 字段，消费侧 sqlite json_extract 判空回退现状）；harness-facts skill 词汇表文档同步。
- `docs/reference/harness/pi-extensions.md`、`.agents/skills/harness-facts/SKILL.md`：gate.check payload 字段与 policy.decision 词汇表描述同步。
- 不涉及业务代码与前端。
