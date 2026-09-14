# Design: offline-catchup（停机恢复补全：标签边保留窗 + 日报自动补档）

## Context

前置 `ai-health-heartbeat-reprobe`（已实现）解决"AI 断⇒暂停 / 回⇒自动恢复"。本 change 补齐恢复之后的两处数据缺口。关键现状（见 proposal - Why）：归档删边与时序运气造成归档文章标签边状态随机（实测 1.5 千带边 / 1.4 万无边）；聚合消费方（日报候选 collectBoardTags、cotag 30 天窗、升级建议）全部不筛 `articles.archived`，仅靠时间窗界定范围；日报 21:00 只生成当天，缺档永不自动补。

侦察确认的落点：`aux_label_cleanup` job（小时级，tag 域卫生，`job_aux_label_cleanup.go`）；孤儿回收 `tagmanagement/service/core/article_tagger.go:364 CleanupOrphanedTags(tagIDs)`；数值窗口配置先例 `persistent_topic_*`（cfg struct + 默认值 + 解析失败回退，`daily_report_topic_repository.go:60-177`）；日报按 (board, period_date) upsert（`daily_report_repository.go:190`）。

## Goals / Non-Goals

**Goals**

- 归档变纯生命周期标志（不删边）；边按 7 天（可配）窗口统一回收，消除随机状态。
- 停机 ≤ 窗口天数时，恢复 → 队列清空后自动补齐缺档日报，零手动。
- 超窗日期重建被拒绝，杜绝"空报告覆盖好报告"。

**Non-Goals**

- 不做配置设置 UI（键走既有 ai_settings 写入途径）。
- 不改日报生成算法、`reading_behaviors`/`search_vector` 归档处理、队列不筛 archived 的语义。
- 不做"报告存在但不满意"的自动重生成（手动重建入口语义不变，仅受窗口守卫约束）。

## Decisions

### D1 · 边 GC 收编 `aux_label_cleanup`，不新增 scheduler job

`AuxLabelCleanupJob` 在既有 aux label GC 之后串第二步：调用 tagging 包新增的 `EdgeGC(ctx, EdgeGCRequest{RetentionDays})`。理由：同域（标签衍生数据卫生）、同节奏（小时级足够，窗口天级）、复用 registry 注册与运维可见性；用户新增 job 无收益。备选 `log_cleanup`（保留期模式更像但域是日志）已否决。scheduler 约束（Registry 自动发现、无第二份清单）不涉及。

### D2 · 配置键 `tag_edge_retention_days`，默认 7，双语义复用

`ai_settings` 单键驱动三处：边 GC 窗口、日报补档扫描窗口、重建守卫下界。语义绑定是刻意的：**边在 = 候选可信 = 可重建；边回收 = 不可信 = 拒绝重建**——一个键保证三处口径永不漂移。读取照 `persistent_topic_*` 模式（缺失/非法/≤0 → 回退默认 7 + warn 日志）。窗口从**边创建时刻**起算（打标落库时刻），天然覆盖"停机 N 天 → 恢复日 drain → 边生成 → 7 天内补档"链路，不从文章发布日起算（会错位）。

### D3 · GC 实现：先删边、后收孤儿，不加索引

`DELETE FROM article_topic_tags WHERE created_at < now()-N days`，删除前收集受影响 `topic_tag_id`，删后调 `CleanupOrphanedTags`（与现归档路径同一函数，行为一致）。`created_at` 无索引——稳态 ~1.5 万行（550/天 × ~4 边 × 7 天）seq scan 毫秒级，不值得为此加索引与迁移；若未来入库量级变化再补（记录于此，届时是参数级决策）。

### D4 · `CleanupOldArticles` 仅移除删边段

删除「affectedTagIDs 收集 → 删 ArticleTopicTag → CleanupOrphanedTags」三步；`reading_behaviors` 删除、`search_vector` 置 NULL、`archived` 翻标志、活跃窗口计数全部不动。孤儿 tag 清理职责整体移交 D1 的窗口 GC（边不再随归档消失，孤儿只能由 GC 产生）。

### D5 · 补档挂 `DailyReportJob` 尾部，队列空为前置，顺延不丢

21:00 生成完当天报告后：若 tag_jobs 无 pending/leased（积压已 drain）→ 对窗口内（不含今天）每个日期 d 逐板检查 `(board, d)` 报告存在性，缺失则 `GenerateAndSaveReport(board, d)`（幂等 upsert，只补缺不重建已有，避免每晚重算烧 LLM）；若队列非空 → 本轮跳过补档，次日 21:00 再试（缺档仍在窗口内，最迟窗口末端补上；配合心跳 change，用户停机 ≤7 天场景下恢复日次日即补齐）。备选（heartbeat 恢复钩子即时补档）已否决：恢复时刻积压必然未清，补了也不全。

### D6 · 超窗守卫：两条入口同口径拒绝

`POST /api/daily-reports/generate`（handler 校验 date）与调度器 `TriggerNowWithDate`：`date < today - retentionDays` → 拒绝（HTTP 4xx / accepted=false），错误消息明示"标签边已按 N 天窗口回收，超窗日期候选不全，拒绝重建"。既有好报告因此不会被空报告覆盖（同日重建是整份覆盖语义）。

## Risks / Trade-offs

- **[补档当晚队列未清 → 顺延一天]** 停机多日后恢复，drain 需数小时，当晚 21:00 若未清则次日才补。可接受（窗口 7 天 >> 停机 ≤7 天 + 1 天顺延）；急性子用户可 drain 完后手动重建（窗口内允许）。
- **[窗口是硬边界]** 停机超过 7 天 → 早期日期的边已回收 → 无法可信补档，守卫拒绝。语义诚实（与其补空报告不如明说不可补）；需要更长窗口改一个配置值。
- **[孤儿清理延迟]** 原先归档即时收孤儿 → 现在最长延迟到下一轮 GC（小时级）。tag 短暂多活几小时无消费影响（aux_label_cleanup 本就是 disable 语义 + 衰减机制兜底）。
- **[边表持续小规模波动]** 万级行、小时级 DELETE+count，锁窗口毫秒级，无并发冲突面（与归档路径不再共享删除职责）。

## Migration Plan

1. 合入即生效；无 schema 迁移（无新列，配置键 lazy 读取缺失走默认）。
2. 部署后第一轮 GC 会把存量"泄漏边"（>7 天的 1542 篇归档带边文章）清掉——属预期收敛，这些边的使命（当日报告已生成或永不再生成）均已完成。
3. 回滚 = 回退二进制；边表已回收的行不可恢复（语义等同旧行为的归档删边，无数据降级）。

## Open Questions

（无——窗口默认 7、可配置、收编既有 job、守卫拒绝语义均经用户确认。）
