# lane-trend-overview 设计

## Context

泳道态势现状（见 proposal Why）：结算产物单句 ≤100 字（`lane_snapshot.go`，`laneSnapshotMaxRunes=100` rune 硬截断，实测句子被切半）；月/年周期归档（`topic_lifeline_context`，granularity week/month/year/all，Weekly/Monthly/Yearly 定时任务维护，前端 `boardEnrichment.ts` 已封装 contexts 端点）从未上人看 UI；日报阅读视图泳道展开体（`DailyReportTopicSection`）只有当日明细 + 7 天节点图。目标形态已与用户对齐：泳道展开体顶部趋势区（14 天长版/月/年切换 + 全文 + 逐日事件折叠），板块内容卡片不动。

## Goals / Non-Goals

**Goals**

- 长短两版结算产物与存储/聚合链路（一次 LLM 调用出两版）
- 日报泳道趋势区组件与挂载（三档切换、全文、降级、逐日事件折叠）
- 月/年档零新增后端（复用既有 contexts 端点与前端 API 封装）

**Non-Goals**

- 不改板块内容 tab 卡片（短版渲染、布局、交互一律不动）
- 不改 lifeline_context 的生成口径/定时任务/表结构（纯读取消费）
- 不做富文本/markdown 渲染（月/年 content 按纯文本换行展示，富文本化留待后续）
- 不做"报告期当时"的历史态势回放（趋势区锚定板块最新报告期，与阅读的报告日期解耦——趋势就是"现在怎么样"）

## Decisions

### D1 两版生成协议：单次调用 + JSON 输出 + 解析降级

结算仍为一次 LLM 调用（`daily_report.lane_snapshot` / digest_polish，纪律不变）。system prompt 要求输出 JSON：`{"summary": "≤100字一句话", "detail": "≤500字成段叙述"}`，两版同一事实集。解析与降级规则：

- JSON 解析成功 → `truncateRunes(summary, 100)` + `truncateRunes(detail, 500)` 各自 rune 安全截断后入库（机械截断保护与现状同构）；
- 解析失败或字段缺失 → 降级：整段输出截 100 字作短版、长版置空、记 warn 日志；结算不算失败（不重试，下个日报日自愈）——守住"结算失败不阻塞"红线；
- `maxTokens` 512 → 768（两版合计 ≤600 字 + JSON 结构开销），温度不变。

**备选与否决**：分隔符分段文本协议（解析更简单但边界歧义大，长文里出现分隔符即坏）；两次独立调用（多一倍成本且两版可能失一致，违背"同窗同素材一次生成" spec）。

### D2 存储：`topic_lane_snapshots` 加可空列 `rolling_detail`

```go
RollingDetail string `gorm:"type:text" json:"rolling_detail"` // ≤500字长版叙述；空=缺失（存量/生成失败）
```

列名对齐既有 `rolling_summary`。AutoMigrate 直加（无 FK/索引/回填——纯派生缓存，下个日报日自然覆盖补齐）。JSON 序列化上"缺失"以空字符串表达（text 列可空与否不敏感，统一空串=缺失；聚合响应层转 `*string` 区分语义，见 D3）。

**备选与否决**：独立新表（过度设计——生命周期与快照完全同构，每日报覆盖 upsert）；`rolling_summary` 存 JSON 双字段（破坏既有短版消费者的读取契约）。

### D3 聚合响应：snapshot 增 `detail` 可空字段

`LaneDynamicsSnapshot` 增 `Detail *string \`json:"detail,omitempty"\``——`nil`=长版缺失（存量），非 nil=长版全文。快照整体缺失仍是 `snapshot=null`（既有语义不变），两级缺失可区分（对应 spec「长版缺失明示」「无态势快照降级」两个场景）。handler 零改动（透传）。

### D4 日报页取数路径：复用板块级 lane-dynamics + 按需 contexts

- **14 天长版 + 逐日事件**：阅读视图宿主（`BoardDailyReportTimeline`）在首个泳道展开时发一次板块级 `GET /semantic-boards/:id/lane-dynamics?days=14`（既有端点，现成聚合含 snapshot.detail 与逐日 timeline），页面级缓存、切报告日期不重拉（数据锚定板块最新报告期 `MAX(period_date)`，与翻看哪期报告无关）。`DailyReportTopicSection` 接收该缓存，泳道展开体按 `topic_id` 从 `lanes` 取本泳道数据传入趋势区。
- **月/年**：趋势区切到对应档时按 topicId 调既有 `fetchContexts(topicId, 'month'|'year')`，宿主级 `Map<topicId, ContextRow[]>` 缓存（周期归档与报告期无关，翻报告不重拉）；展示 `period` 最大一条的 `content` 与 `as_of_date`（前端自取 max，不依赖响应排序）。
- **泳道不在 lanes 里**（沉寂/新锚定当期未进窗口）：14 天档与逐日事件按"快照缺失/无事件"降级占位，月/年档照常。

**备选与否决**：新增单 topic 趋势端点（多一个 handler+repo 查询，聚合数据现成没必要）；逐日事件走 `getTopicLifeline`（7 天窗口不够 14 天）。

### D5 前端组件：新增 `LaneTrendOverview`，泳道体顶部挂载

`features/tags/components/daily-report/LaneTrendOverview.vue`，props：`topicId`、`topicColor`（泳道左缘线复用）、`lane`（该泳道的聚合数据或 null）。内部状态：当前档（`'14d' | 'month' | 'year'`，默认 14d，每泳道独立）、逐日事件展开开关（默认收起）、单日"还有 N 条"就地展开。分段切换复用既有分段按钮模式；折叠复用「还有 N 条」模式；状态条复用 `--bg-sunken` 占位/内联错误条。分组逻辑（日期→事件、折叠计数）在组件内实现（~30 行，不抽共享 util、不动 `LaneDynamicsCard`——守住"卡片不改"承诺，避免顺手重构波及既有测试）。月/年 content 渲染用 `white-space: pre-line` 纯文本（见 Non-Goals）。

挂载条件与 `DailyReportMiniLifeline` 对齐：`zone.key === 'active' && group.topicId != null`。挂载位置在泳道展开体最顶（今日 section 卡片与节点图之前）。

### D6 降级矩阵（spec 场景 → UI 行为）

| 数据态 | 14 天档 | 月/年档 | 逐日事件 |
|---|---|---|---|
| 一切就绪 | 长版全文 + as_of | 最新归档全文 + as_of | 可展开，5 条/日折叠 + 如实计数 |
| 长版缺失（存量） | 短版 + 「长版随下次日报结算生成」提示 | 不受影响 | 照常（数据来自 timeline 非快照） |
| 快照缺失 | 「态势待结算」占位 | 不受影响 | 照常 |
| 月/年无归档 | 不受影响 | 「该周期暂无归档摘要」占位 | — |
| 请求失败 | 内联错误条 + 重试 | 内联错误条 + 重试 | 随 14 天档错误态 |
| 加载中 | 加载提示 | 加载提示 | — |

## Risks / Trade-offs

- [LLM 输出 JSON 解析失败率未知] → 降级路径保底（短版截断全文 + 长版空），warn 日志观察 `ai_call_logs` 若干日报周期，失败率高再考虑分隔符协议或重试一次。
- [单次调用输出预算翻倍，个别泳道结算变慢] → maxTokens 768 上限封顶；每泳道 60s 超时与 20 泳道/板块 clamp 均不变。
- [日报页多一次板块级请求] → 单请求聚合、页面级一次、只读轻查询；加载态不阻塞既有内容渲染。
- [contexts content 是 AI 生成的结构化长文，纯文本渲染观感朴素] → 接受（Non-Goals）；换行分段已可读，富文本化后续按需。
- [趋势区锚定最新报告期而阅读视图可翻旧报告，as_of 可能晚于所看报告日期] → 这是刻意的语义（趋势=现在）；UI 标注「汇总截止」日期自解释。

## Migration Plan

1. 后端合入（模型加列 + AutoMigrate，启动自动生效；旧快照 detail 空，降级路径生效）。
2. 前端合入 + 静态部署。
3. 可选：手动触发一次当日日报生成，全部活跃泳道当日成立长版。
4. 回滚：前端回滚即可隐藏 UI；后端回滚丢弃 detail 列（纯派生缓存无数据损失）。

## Open Questions

- 14 天档在长版就绪后是否需要"重新结算"手动入口（现在只能等下个日报日）——观察使用频率再定，不阻塞本期。
