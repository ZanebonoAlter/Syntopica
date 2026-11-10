# Design — fix-board-health-triage-degrade

> 触发来源：2026-11-03 21:05 那轮体检 12 板 6 板整轮降级的实证复盘（见 proposal Why）。
> 本 change 只修「体检依据凭空消失」与「触达不到」，不改提示词、不改模型、不动前端。

## D0. 归档顺序依赖（必读）

`board-health-triage-advice` 的 triage 能力**代码已并入但 change 尚未归档**，`openspec/specs/board-health-checkup/spec.md` 目前仍是只有「评分+评语」的旧文本。本 change 的 MODIFIED delta 写的是包含 triage 的**最终态文本**。

**归档顺序 MUST 为：先归档 `board-health-triage-advice`，再归档本 change。** 反序会把 triage 语义从主 spec 里挤掉（MODIFIED 是整段替换）。tasks 中留一条显式检查项。

## D1. 覆盖不全 ⇒ 补齐 ⚪ 证据不足，而非作废整轮

**现状**：`llm.go:334 validateTriage` 第一行 `len(triage) != len(labels)` 直接返回 error；`run.go:86` 拿到 err 就把 `score/comment/triage` 全留空 —— 一条漏判 = 整块板没依据。11-03 实证：模型在 47/50/54/58/96/98 标签板上分别只回 45/40/51/48/≥68/≥72 条，六块板全废。

**决策**：把校验拆成「致命」与「可补齐」两级。

| 情形 | 处置 | 理由 |
| --- | --- | --- |
| 漏判部分标签（覆盖率 ≥ 50%） | 缺失标签补 `insufficient`，保留 score/comment/已给分诊项 | 漏判是模型稳定性问题，不是证据；补「证据不足」既不伪造结论也不丢已得信息 |
| 覆盖率 < 50% | 整轮降级 | 只回几条说明输出崩了（截断/格式坏），此时已给的分诊项也不可信；补齐会变成满屏「证据不足」的假象 |
| category 不在闭集 | 整轮降级 | 结构性非法，无法判断模型在想什么 |
| 未知 aux_id / 重复 aux_id | 整轮降级 | 分诊项无法映射到构成标签，保留会污染清单 |
| evidence 为空 | 整轮降级 | spec 约定每条必附证据，「无证据建议」比没有建议更危险 |
| rewrite 缺 suggestion | 整轮降级 | 界面上会是空白可编辑框 |
| LLM 调用失败/超时 | 整轮降级（沿用现状） | 与漏判区分：那是接口层失败 |

补齐项文案固定为 `本轮 LLM 未覆盖该标签（返回 N/M 条），下轮再判`，类别恒为 `insufficient`，**绝不猜测** keep/remove —— 与既有「不伪造补齐」的初衷不冲突：原决策防止的是「伪造可执行建议」，补「证据不足」是承认本轮没证据。

**被否方案**：「漏判就重试一次 LLM」——单板 LLM 已占 30–60s，重试直接翻倍预算，且本地 4B 的漏判是稳定性问题（同一板 10-30 全过、11-03 全漏），重试大概率同样漏。

**被否方案**：「提高覆盖率要求到 100% 但只降 triage、保评分」（用户未选）——补齐方案已让 mismatch 路径消失，剩余非法项属真异常，整轮降级可接受；拆粒度增加的语义分叉不值得。

## D2. 日志口径

现状：体检**成功时零日志**，降级只有 `Warnf` 一句话，事后只能翻 `board_health_snapshots` 才知道哪轮废了。

新增（每板一行，`[board-health]` 前缀，可 grep）：

```
[board-health] board=1974 labels=47 covered=45 filled=2 degraded=false elapsed=38s
[board-health] board=1980 labels=96 covered=68 filled=0 degraded=true reason="llm triage invalid: triage covers 68/96 (70.8%) below min coverage 50%" elapsed=52s
```

整轮结束一行：`round finished boards=<scope 总数> processed=<实际处理> degraded=0 filled=5 skipped_fresh=0 elapsed=7m48s`。

口径要点：成功路径也要有行（原来只有失败有）；`filled` 让「模型漏判」变成可量化趋势；**降级行也要回传真实 `covered`**（覆盖率不足时是 68/96 而不是 0，否则与同一行 reason 自相矛盾）；被 recover 的 panic/超时也要尽量带上已知 `labels`（列为 0 说明连采集都没跑完）。

## D3. 触发面：三条产出路径 + 新鲜度守卫

**现状**：`RunCheckup` 全仓只有两个调用点 —— 日报定时主轮尾步、`admin board-checkup` CLI。回填与手动端点都不触发。用户 11-03 手动/回填补出 11-01、11-02 日报后，体检数据自然纹丝不动。

**决策**：

- 抽出带参数的入口 `boardhealth.RunCheckupWithOptions(ctx, db, llm, Options{BoardIDs, MinInterval})`，`RunCheckup` 保留为全量零守卫包装（CLI/现有尾步不受影响）。
- 三条触发：
  1. 定时主轮尾步（现状不变，零守卫，但仍走新预算）——**主轮是当夜首轮，MUST NOT 被守卫跳过**（否则白天手动生成过一次，当晚就没有当日依据）；
  2. `job_daily_report.go` 回填阶段 `backfilled > 0` 后补一次（**异步 goroutine**：主轮有失败版面时尾步被跳过，这一轮会真跑满 30 分钟，不能算进 job 时长；通常是守卫全跳 ≈ 一次 SQL）；
  3. `topicgraph/handler` 三个 `generate*` 函数中 `saved > 0` 时异步触发，scope=本次板块集合。
- **新鲜度守卫**（仅回填与手动两条路径）：`Options.MinInterval` 默认 6h。批量查 `MAX(checked_at) GROUP BY semantic_board_id` **且只统计 `llm_score IS NOT NULL` 的快照**——降级轮也会写 checked_at，但它恰恰是「本轮没有依据」，若算新鲜则该板 6h 内无法重跑（与 change 目的相拗）。命中即跳过该板（不调 LLM、不写快照、记 skip 数）。
- **语义边界**：守卫保证的是「主轮之后，回填与手动不会重复跑同一块板」；「同一晚主轮 + 回填 + 手动叠加最多一轮 LLM」只在主轮先于手动时才成立（主轮零守卫是有意取舍：保有当日依据优先于省一轮 LLM）。
- **异步与上下文**：手动与回填两条异步路径都用 `context.Background()` 派生自己的 30 分钟预算（生成 goroutine / job ctx 此时可能已结束）；panic 在 goroutine 边界 recover（回填沿用 `launchLifelineHealAsync` 现有写法，gosec G118 已标注理由）。

**被否方案**：体检拆成独立 cron（如 06:30）。多一个调度面 + 与日报数据新鲜度脱钩，且用户预期是「日报出来就能看体检」，改动面也更大。

## D4. 两级时间预算

现状：`job_daily_report.go:55` 硬编 10 分钟整轮无单板超时 —— 11-03 12 板耗 7.8 分钟已贴近上限，10-30 出现 `board 246030 checkup failed: context deadline exceeded`（前面的板吃光预算，后面的板直接被砍）。

**决策**：

- 整轮预算 10min → 30min（实测 12 板 7.8min，留 4x 余量给板数增长）。
- 单板子超时 6min（`context.WithTimeout` 包在 `checkupOneBoard` 调用外）：一个慢板只把自己降级，兄弟板预算不受影响。
- 超时按单板失败处理，日志带 `elapsed`；整轮 MUST NOT 因单板超时中断。

## D5. 新的参数默认值（常量，不进配置面）

| 参数 | 默认 | 位置 |
| --- | --- | --- |
| 最小覆盖率下限 `minTriageCoverage` | 0.5 | `boardhealth/llm.go` 包级常量 |
| 单板子超时 | 6min | `boardhealth/run.go` |
| 整轮预算 | 30min | `admin/scheduler/job_daily_report.go` 尾步 + 手动触发处 |
| 体检新鲜度间隔 | 6h | `boardhealth` 包级默认 + `Options.MinInterval` 覆盖 |

不把这些塞进 `ai_settings`：它们描述的是 harness 自保行为，不是用户可调业务参数；多一个配置项就多一处能配错的地方。

## Impact 面复核

- 纯读约束不变：除 `board_health_snapshots` 与（罕见）通知外无写库。
- 无迁移：`metrics.triage` 结构未变，补齐项就是普通的 `insufficient` 条目。
- 前端零改动：降级态（「本轮未生成评分与分诊」）与 `insufficient` 展示分支已存在（`boardHealthWorkspace.ts:281`、`boardHealthTriage.ts:131`）。
