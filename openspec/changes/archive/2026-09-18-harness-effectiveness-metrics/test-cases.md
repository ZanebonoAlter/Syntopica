# Test Cases — harness-effectiveness-metrics

> 单元 = 「效能数据从会话沉淀进账本、再被 retro 报告读成改进项」的完整故事。层选择：聚合器纯函数 → cjs 直测（对齐 `constraint-injection.smoke.cjs` 先例）；retro SQL/渲染 → `harness-retro.smoke.sh` fixture 断言；钩子接线与真实库 → 人工留痕。

## 主链路表

| 步 | 动作 | 来源 Scenario | 期望 | 层 | 落点 |
| --- | --- | --- | --- | --- | --- |
| 1 | 新 pi 会话 session_start，prev jsonl 存在且账本无该 session 的 final 快照 | fact-log:「session_start 回填 prev 终值」 | 追加一条 prev session 的 `final:true` rollup（session_id=prev 文件名 UUID，change 列可空） | cjs 直测（回填判定纯函数）+ 人工 | `.pi/extensions/tests/session-rollup.smoke.cjs` |
| 2 | prev jsonl 不存在 / previousSessionFile 为 null | fact-log:「session_start 回填 prev 终值」/「解析失败 fail-open」 | 零写入、不抛出、console 告警 | cjs 直测 | 同上 |
| 3 | 会话内 turn_end 多次触发、满足节流条件（turn%5==0 或 token 增量>20%） | fact-log:「turn_end 节流快照落库」 | 每次命中节流追加 `final:false` 快照；payload 数值单调不减 | cjs 直测（节流判定纯函数）+ 人工 | 同上 |
| 4 | 同一 session_id 已有多条 rollup 快照 | fact-log:「同 session 取最新一条即终值」 | 查询/报告仅取 `MAX(id)` 一条的数值，不累加中间快照 | retro smoke fixture | `scripts/harness/harness-retro.smoke.sh` |
| 5 | turn_end 时增量读 jsonl（offset 推进） | fact-log:「turn_end 节流快照落库」 | 只读新增行；repeat 读同一文件 offset 不回退、数值稳定（幂等） | cjs 直测 | `.pi/extensions/tests/session-rollup.smoke.cjs` |
| 6 | 回填 prev 时该 session 已有中间快照 | fact-log:「rollup 不改写既有事件」 | append 新事件表达终值，既有行 payload/ts 不变 | cjs 直测（判定逻辑）+ harness-log.smoke 既有「事件追加不可变」回归 | `.pi/extensions/tests/harness-log.smoke.cjs` |
| 7 | 词汇表登记 | fact-log:「session.rollup 随词汇扩展落库」 | harness-log TTL 表含 rollup→90 天；写入路径产出正确 kind | cjs 直测 | `.pi/extensions/tests/harness-log.smoke.cjs` |
| 8 | fixture 库含各类事件（rollup 序列+无 rollup session+跨域注入 change） | retro:「七段齐备且各带指标」 | ⑦段输出且每指标可独立复算；故障①-⑥段不变 | retro smoke | `scripts/harness/harness-retro.smoke.sh` |
| 9 | 窗口内多数 session 无 rollup 终值 | retro:「rollup 覆盖率显式标注」 | 覆盖率数值正确；B 组分布行尾「数据积累中」标注 | retro smoke | 同上 |
| 10 | declaration 注入 domain-A、edit.map 路径全落 domain-B | retro:「注入命中率可复算」 | 命中率=0；域映射规则在口径说明可查 | retro smoke | 同上 |
| 11 | gate.check 同 (session,cmd) 先红后 40 分钟后绿 | retro:「门禁催修时距可复算」 | 催修时距中位数计入该样本 | retro smoke | 同上 |
| 12 | 同一文件在 ≥3 个 session 的 edit.map 出现 | retro:「返工波次清单」 | 返工信号组输出该文件与波次计数 | retro smoke | 同上 |
| 13 | 报告渲染⑦段 | retro:「禁止综合总分」 | 不存在多指标合成单一分数的行；基线差值按各指标分别表达 | retro smoke（grep 断言无综合分键） | 同上 |
| 14 | subagent.complete 带 tokens、部分 dispatch 无值 | retro:「子线程 token 占比可复算」 | 分子仅由非 null 样本构成并标注样本数 | retro smoke | 同上 |
| 15 | --save-baseline 两次后 --baseline | retro:「生成基线与差值」（MODIFIED） | metrics7 键组差值输出；旧基线（无 metrics7）跳过该组并提示不失败 | retro smoke | 同上 |
| 16 | 域映射源（flow 文档头 doc-impact-applies）不可解析 | design D4 | 「域映射不可用」降级标注，其余指标不受影响 | retro smoke | 同上 |
| 17 | 真实库跑 --days 30 | tasks 3.2 | ⑦段降级形态正常；exit 0 | 人工 | 会话留痕 |

## 变体走查（五组固定清单）

| 组 | 变体 | 答案 | 落点 |
| --- | --- | --- | --- |
| 输入 | 空串/空文件 | 聚合器返回零值汇总（turns=0 等）+ skippedLines=0，不写入回填（prev 全零也无意义但按契约仍可写 final——**决策：零 turn 的 prev 不回填**，视为无效会话） | cjs 直测 |
| 输入 | 纯空白行（含全角/tab） | JSON.parse 失败 → skippedLines+1，跳过 | cjs 直测 |
| 输入 | 坏 JSON 行（截断/特殊字符） | 同上；不影响其余行聚合 | cjs 直测 |
| 输入 | 超长单行（MB 级 message） | 逐行读不整文件载入内存问题——Node readFileSync 按行 split 可承受（单 session ≤10MB）；不做流式（记录为已知边界） | — |
| 输入 | usage 字段缺失的 assistant 行 | 该行计入 steps 但 tokens 五值按 0 累计 | cjs 直测 |
| 前置 | 空集（窗口无 rollup） | ⑦段 B 组输出「无数据」而非 NaN | retro smoke |
| 前置 | 单样本 | 中位数=该值、P75=该值；照常输出 | retro smoke（fixture 单 rollup） |
| 前置 | 重复（同 turn 两条快照） | 取最新；不重复计数 | retro smoke |
| 前置 | 越界引用（rollup.session_id 无对应 jsonl） | 正常——retro 只读账本不反查 jsonl | — |
| 时间窗口 | 边界两端（ts 恰好=窗口起） | 沿用现有窗口 SQL 口径（>=起 <止），rollup 无特例 | 既有断言覆盖 |
| 时间窗口 | 空窗口（--days 内零事件） | 沿用「空库不产出伪指标」既有行为 | 既有断言覆盖 |
| 幂等 | 重复执行（同 prev 回填两次 session_start） | final 已存在 → 第二次零写入 | cjs 直测 |
| 幂等 | 部分失败重试（logEvent 写入失败） | fail-loud 一次性 notify（既有 guardedLog 语义），下次 turn_end 重试 | 既有机制 |
| 幂等 | 并发 | 单会话单扩展进程顺序钩子，无并发写；账本 WAL 模式已兜底（不声称新线程安全） | — （划除：无并发场景） |
| 可用性(UI) | — | — | — （划除：无 UI） |

## 效果核对（依赖断言外因素）

| 项 | 触发原因 | 方法 | 量化结果 | 结论 |
| --- | --- | --- | --- | --- |
| rollup 数据积累有效性 | 覆盖率依赖用户日常使用 pi | 上线后跑 N 天 `--days N` 看⑦段覆盖率与分布合理性（如单 session turns 7~500、tokens 与 jsonl 手算一致） | 人工核对留痕（tasks 3.2/3.3） | 覆盖率>50% 后分布才可作为基线锚 |
| ⑦段指标信噪 | 指标口径是否真有区分度 | 对比两个不同 change 的⑦段输出（一个大重构 vs 一个小修） | 人工核对留痕 | 指标应能拉开差距，否则回到口径修订 |

## 继承与调整（⓪ MODIFIED Requirements 反查）

| 旧 Scenario | 处置 | 旧测试 | 动作 |
| --- | --- | --- | --- |
| fact-log:「TTL 分级清扫」 | 保留（原文未变） | `.pi/extensions/tests/harness-log.smoke.cjs`「TTL 与既有事件兼容」 | 扩 fixture：91 天前 rollup 行被清、30 天前 constraint.inject 被清不变 |
| fact-log:「事件追加不可变」 | 保留（原文未变） | 同上 smoke | 不动；新增「回填不改写中间快照」断言互补 |
| fact-log: spill/subagent.complete/policy.decision/edit.map「随词汇扩展落库」四条 | 保留（原文未变） | harness-log.smoke / constraint-injection.smoke | 不动 |
| retro:「六段齐备且各带指标」 | **语义收窄**（正文限定①-⑥故障视角；七段由新 Scenario 覆盖） | `scripts/harness/harness-retro.smoke.sh`「六段齐备且各带指标」 | 检查断言是否精确计数六段——若断言「恰好六段」则改「至少含①-⑥」；数值断言不动 |
| retro:「失败特征归并」「按 change 归属限定」 | 保留（原文未变） | 同上 smoke | 不动；⑦段指标须通过 --change 限定（同一断言模式扩展到⑦） |
| retro:「生成基线与差值」「基线损坏时降级」 | 保留（正文仅加 metrics7 键） | 同上 smoke | 扩断言：metrics7 键存在且差值输出；旧基线无 metrics7 跳过提示 |

## 白盒附加（复杂档：分支表 + 边界值）

### 聚合器 parseSessionUsage 分支表

| # | 分支 | 输入边界 | 期望 |
| --- | --- | --- | --- |
| B1 | 行 JSON.parse 失败 | 空行/`{`截断/全角空白 | skippedLines+1，继续 |
| B2 | type≠message | session/custom_message/model_change 行 | 忽略（不计数） |
| B3 | message.role=user | 计 turns | turns+1 |
| B4 | message.role=assistant 且有 usage | steps+1；五值累加 | tokens 单调不减 |
| B5 | message.role=assistant 无 usage | steps+1；tokens 五值 +0 | 不误报 |
| B6 | message.role=toolResult | toolCalls+1 | — |
| B7 | usage.totalTokens 与Σ(input+output+cacheRead+cacheWrite+reasoning) 不一致 | 以 totalTokens 为准（供应商口径） | total=totalTokens |
| B8 | cost.total 缺失 | cost=null | 可空容忍 |
| B9 | 首末消息 ts 差 | 单消息→durationSec=0 | ≥0 |
| B10 | model_change 行存在 | 取最后一条的 model | 可空 |

### 节流判定 shouldWriteSnapshot 边界值

| 输入 | 期望 |
| --- | --- |
| turn=4（%5≠0）、token 增量 10% | 不写 |
| turn=5 | 写 |
| turn=4、token 增量 20.0001% | 写（>20 严格） |
| turn=4、token 增量=20.0%整 | 不写（严格大于） |
| 首次快照（lastTokens=null） | 写（bootstrap） |
| turn=0（空会话） | 不写 |

### 回填判定 shouldBackfillPrev 边界值

| 输入 | 期望 |
| --- | --- |
| prev=null | 否 |
| prev 文件不存在 | 否（零写入+告警） |
| prev 解析出 turns=0 | 否（无效会话） |
| 账本已有该 session final=true | 否（幂等） |
| 账本仅有中间快照（无 final） | 是（追加 final） |
| 账本零快照 | 是 |
