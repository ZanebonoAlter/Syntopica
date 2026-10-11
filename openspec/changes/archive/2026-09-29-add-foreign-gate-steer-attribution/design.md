## Context

2026-09-28 会话 01a0e7f1-21eb-76de-825c-5279cb5c5ea6（零代码编辑的探索会话）被另一活跃 change 的 reader/service 中间态失败连续注入 23 轮近似逐字相同的门禁 steer。源码排查（.pi/extensions/quality-gate.ts + lib/failure-classify.ts）确认：边沿触发抑制与三态归属判定机制均已存在（aggregate-concurrent-gate-warns / tune-quality-gate-concurrency / attribute-concurrent-gate-noise 三 change 落地），但被两处失效击穿。事实存档：docs/research/harness-gate-steer-attribution/explore-findings.md。

失效实证：
- **指纹击穿**：failureReports 同指纹静默用 `prev.diag === diag` 精确相等；diag 取 truncateDiagGate 的首个失败特征行，测试运行期输出该行为带秒级时间戳的日志行（账本实测 `2026/09/28 22:33:46 ...` → `22:35:39` 逐轮不同），比较永假 → 每轮视同指纹变化 → 每轮完整块注入。
- **mine 误判**：该会话零编辑、无 boundChange、失败包目录 ⊆ 启动基线（overhaul 自 06:52 开工，早于会话 12:15 启动），按 classifyFailureOwnership 应判 foreign；实际 23 轮全判 mine（该会话 policy.decision 仅 1 条 gate-lock-held，零 foreign-breakage / concurrent-mixed）。断点未定位——归因过程异常被 catch 静默吞掉，账本无任何降级痕迹，这正是本 change 要先修的可观测性缺口。

## Goals / Non-Goals

**Goals:**
- 同指纹静默对运行时动态值（时间戳/端口/耗时/数字序列/绝对路径前缀）免疫。
- 归属判定的每次保守回退在账本可考古（ownership 字段 + 归因降级事件），mine 误判根因可定位并修复。

**Non-Goals:**
- 不改变粘性重跑语义、分级阈值、门禁命令集。
- 不引入会话级门禁开关（isChildSession 之外无旁路——本次会话侧规避无正解的结论维持）。
- 不重构归属判定架构；三态语义与既有 spec 契约不变，只修实现与可观测性。

## Decisions

### D1 指纹归一化：新增独立函数，不触碰记账 diag

`lib/failure-classify.ts` 新增 `normalizeDiagFingerprint(text: string): string`，仅在 quality-gate 的 failureReports 比较点使用；`truncateDiagGate`（gate.check 记账与展示）不动。归一化规则按序应用：

1. 剥日志行首时间戳：`^\d{4}[-/]\d{2}[-/]\d{2}[T ]\d{2}:\d{2}:\d{2}(\.\d+)?`
2. 剥 IP:port：`\b\d{1,3}(\.\d{1,3}){3}:\d{2,5}\b` 及 `127.0.0.1:\d+` / `http://...:\d+` 中的端口段
3. 剥耗时/毫秒值：`\(\d+(\.\d+)?(ms|s)\)`、`\[\d+(\.\d+)?ms\]`
4. 数字序列压平：连续 `\d+` → `<N>`（覆盖剩余计数、端口号、毫秒残段）
5. 仓库根绝对路径前缀 → 相对路径（`/home/.../Syntopica/` 剥除）

保守取向：归一化只剥"必然逐轮变化"的形态；不确定的保留原文进指纹（宁可多报不漏报新指纹——与既有"宁可多报不误判外部"同向）。

*替代方案否决*：在 truncateDiagGate 内归一化——会把记账 diag 也改掉，破坏"相同输出字节产生相同 diag"的既有确定性契约与事后考古能力。

### D2 ownership 字段：gateLog 失败分支顺手写入，值域四态

gate.check ok=false 的 payload 新增 `ownership: "mine" | "foreign" | "mixed" | "attribution-degraded"`。写入点在 gateLog 的失败分支（classifyFailureOwnership 返回值原样透传；try/catch 回退 mine 时改记 attribution-degraded）。ok=true 不加该字段（成功无归属语义）。存量事件无键——消费侧（harness-retro / harness-facts 查询配方）判空回退，不迁移不回填。

### D3 归因降级词汇：`attribution-degraded`

policy=quality-gate、action=warn、reasonCode=`attribution-degraded`、target=cmd。语义：归属信号缺席/判定异常导致保守回退 mine。记点位：①classify 的 catch 路径；②baselinePaths 为空且失败本可判外部信号的场景（session_start git 快照失败时显式记一条会话级降级，避免逐命令重复）。同指纹降级回合去重口径与 foreign-breakage 对齐（同 (cmd, fingerprint) 至多一条）。词汇登记 harness-facts SKILL.md 词汇表并进 smoke 断言稳定集。

### D4 mine 误判根因定位：可观测性先行，复现样本回归

修复顺序强制"先记后修"：D2/D3 落地后，以本 change 自己的实现会话 + 一个构造的复现场景（新会话启动前由另一会话改某包文件、新会话零编辑触发该包失败）观察 ownership 实际取值。嫌疑清单按优先级排查：
1. baselinePaths 启动快照时序（session_start 事件 cwd 异常 / compaction reload 重拍时机 / ESM 模块缓存跨会话残留导致 ownerSessionId 误清）；
2. expandPkgAnchorDirs 展开后 P 含 mine 侧误吸（accumulatedTriggerPaths 是否被非编辑 bash 命令污染）；
3. expandSideForms 双前缀形态与锚点形态错配。
定位后修复 + 回归场景固化进测试。**在 ownership 记账落地前不盲修**——无观测就无归因，这是本次事故的方法论教训。

### D5 测试设计（复杂度 simple，收敛两层）

- **纯函数层**（表驱动单测）：normalizeDiagFingerprint（23 轮轰炸的实账 diag 样本逐条断言归一化后相同）；classifyFailureOwnership 既有用例不回归 + attribution-degraded 新路径。
- **记账契约层**（smoke/fixture）：ownership 字段四态落库、存量无键兼容、attribution-degraded 不与 gate.check 失败事实双写。

## Risks / Trade-offs

- [归一化过度折叠两个真实不同失败为同指纹 → 新问题被静默] → 规则只剥时间/端口/耗时/数字形态，错误文本主体保留；指纹变化仍重新全文注入（既有第 4 条契约不变）；gate-status-query 可随时查当前红态。
- [D4 根因定位可能发现是 pi 扩展宿主时序问题（不可修）] → 降级目标：确保该场景至少判 attribution-degraded 且账本可考古，[外部] 判定修复移交后续 change；不阻塞本 change 其余交付。
- [两个 change 并发改同一 Requirement（策略显著裁决统一记账）] → 归档顺序约束：本 change 先归档 sync；add-change-lifecycle-retro 的 delta 已按合并后状态书写，其归档时 openspec 会校验冲突。

## Migration Plan

1. lib 归一化函数 + 单测先行（无行为变化，可独立合入）。
2. quality-gate 指纹比较点切换 + ownership 记账 + 归因降级记账（一个提交，行为变化点）。
3. 扩展热加载验证（新会话生效；无进程内迁移需求——failureReports 本就会话级清零）。
4. D4 复现定位与修复（独立提交，可能延伸后续 change）。
回滚：revert 提交 2 即回现状（记账新字段对旧消费方无影响，无需数据回滚）。

## Open Questions

- attribution-degraded 会话级去重的粒度（session vs session+cmd）——实现时按 foreign-breakage 的 (cmd, fingerprint) 对齐即可，不足虑。
