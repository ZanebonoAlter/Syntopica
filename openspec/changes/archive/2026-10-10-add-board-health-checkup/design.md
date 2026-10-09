## Context

现状锚点（探索阶段已核实）：

- 匹配结果 `topic_tag_board_labels` 只存规则名不存交集明细，无法精确归因「某 tag 是被哪个构成标签拉进版块的」→ 标签级统计采用**贴合度口径**（见 D4 修正记录）：量「构成标签语义与版块实际内容的相似度分布」，不做功劳归因。初版「单 aux 纯净样本」口径经真库实测结构性失效（样本占比 2%-11% 且永不增长），已废弃，实测数据与修正决策见 D4。
- `BoardComposition` 无分数字段、方向校验只存 bool → cosine 需体检时重算；向量为内存计算模式（`CosineSimilarity`，`semantic_board_matching.go:724`）。
- 扩充召回接入点：`semantic_board_expand.go` `recallExpandAuxCandidates`（相似路+共现路，上限 40/路，排除已挂载与 disabled）。
- 日报 job：`job_daily_report.go` `DailyReportJob`，有进程级 gate + 队列感知窗口等待 + 成功/失败终止裁决点（`adjudicateDailyReportTerminal`）+ 手动触发路径（`TriggerNowWithDate`）。
- 通知基础设施：`internal/platform/notification`（type/LinkType/LinkID，落库即达前端铃铛）。
- sourcestats 先例（红线 17）：统计口径独立包唯一实现 + 只读端点。

## Goals / Non-Goals

**Goals:**

- 体检统计口径唯一实现（对标 sourcestats 纪律）；纯读统计 + 单表快照写入。
- 体检自动挂日报成功路径之后，当日日报优先；体检失败不影响日报 job 结果。
- member-derived 扩充候选以快照为载体，手动扩充生成时作为主路、无快照回退既有双路。
- 每活跃版块每轮一次 LLM 调用（评分+评语+打扰裁决合并产出）。

**Non-Goals:**

- 不修改匹配管道写入（不存交集明细、不改 match_reason/score 语义）。
- 不做「功劳归因」（不改匹配管道写入交集明细）；标签级只量贴合度，不做多 aux 近似归因（样本不足就显示不足，不硬凑）。
- 不改组合路扩充候选（member-derived 只覆盖 expand×aux 的单标签候选）。
- 不做体检快照 GC / 不做趋势图以外的历史分析（单用户量级：几十版块 × 每日一条）。

## Decisions

### D1: 体检包布局 — `internal/tagmanagement/service/boardhealth/`

对标 sourcestats：统计口径（版块级指标、标签级贴合度分布、cosine 重算、事实画像 diff）唯一实现在该包；只读查询端点消费同一实现。快照写入（LLM 评分 + 落库 + 通知裁决）同包的 job 编排文件。备选「并入 board 包」被否：board 包已 2000+ 行多文件，体检是独立演进域。

### D2: 快照表 `board_health_snapshots`

```
id                bigserial PK
semantic_board_id bigint FK→semantic_labels(id) ON DELETE CASCADE, index
checked_at        timestamptz NOT NULL          -- 体检轮次时间
metrics           jsonb NOT NULL                -- 版块级+标签级统计（见 D4）
uncaptured_candidates jsonb NOT NULL DEFAULT '[]' -- 未收编候选 [{aux_id,label,member_tags,member_articles}]
llm_score         double precision NULL          -- LLM 评分（0-100），降级为 NULL
llm_comment       text NULL                     -- LLM 评语
created_at        timestamptz
```

- 趋势 = `ORDER BY checked_at DESC LIMIT N`（同日多条保留不去重：手动补跑日报触发的体检是真实数据点）。
- 通知冷却不靠本表字段：直接查 notification 表最近一条（LinkType='board', LinkID=board_id）的时间，冷却期内跳过（见 D6）。
- member-derived 候选不建独立表：候选是快照附属产物、每轮全量重算、无跨轮状态；扩充召回读「最新一条快照」的该字段。

### D3: 体检挂载点 — 日报 job 成功路径尾步（链式同步）

`DailyReportJob` 成功终止裁决后同步调用体检入口；体检 panic/error 仅记日志，不改变日报 job 返回值（日报优先产出）。手动日报路径（`TriggerNowWithDate`）同样触发。

- 否决「独立 scheduler job 定时排在日报后」：日报时长不定（30min 预算+回填段），时间拍不准，可能重叠或空跑。
- 体检不是独立调度 job（无自身 NextTime），不进 Registry 清单，不违反红线 1；手动触发复用手动日报路径，不单开端点（YAGNI）。

### D4: metrics jsonb 结构与统计实现

```jsonc
{
  "board": {                       // 版块级（全部匹配行）
    "matched_tags": 320,           // 挂载 tag 总数
    "reason_dist": { "direct_hit": 210, "composite_hit": 30, "hit_rate": 40, "max_sim": 20, "weighted": 20 },
    "ghost_matches": 12,           // direction_mismatch=true 计数
    "vector_missing": 3            // embedding 缺失计数（tag identity 或构成标签任一侧）
  },
  "labels": [                      // 标签级（贴合度分布：构成标签 × 全部挂载 tags）
    { "aux_id": 42, "label": "推理",
      "samples": 444,              // 参与比对的挂载 tag 数（有 identity 向量的）
      "cosine_mean": 0.71, "cosine_median": 0.72, "cosine_p25": 0.66, "cosine_p75": 0.77,
      "low_sim_ratio": 0.08,       // 低于板块内相对低分线的样本占比
      "insufficient": false }      // samples < 最小样本量 → true 且分布字段为 null
  ]
}
```

- **标签级贴合度口径（D4 修正，2026-10-08 实测数据驱动）**：每个构成标签 embedding（`semantic_labels.embedding`，创建时即有）× 该版块全部挂载 tags 的 identity embedding（`topic_tag_embeddings`），内存 cosine 循环（沿用匹配管道同款实现）。回答的问题是「该构成标签语义与版块实际内容的贴合分布」——不依赖 tag 的 aux 挂载数量、不声称归因功劳。
- **修正原因（初版口径实测失效）**：初版「单 aux 纯净样本」口径假设挂载集中单 aux 占比可观。真库实测：全库 6874 个单 aux tags 仅 68 个（1%）匹配到任何版块——匹配选择效应使多 aux tag 有多次 direct_hit 机会、单 aux tag 的唯一 aux 多为长尾（不在任何版块构成），几乎进不了挂载集；现挂载集单 aux 占比仅 2%-11% 且集中于 9-28 回填的历史 tags（6 月创建），每日新增 ≈ 0，样本池永不增长 → 标签级统计大面积「数据不足」，体检核心价值落空。这不是实现 bug，是设计假设与真实数据形态不匹配。
- **新口径实测验证（真库 SQL 全量模拟，五个板块）**：样本全覆盖（板块 3030：96 构成标签 × 444 样本/标签）；排序语义可验证（3030「开发者工具与开源生态」top 为 Codex/Coding Agent/MCP/Vibe Coding 等核心主题，bottom 为 MySQL/Spring Boot/Vue3/SQLite 等外围技术栈，cos_mean 极差 0.544~0.730）；判别力与板块规模正相关——大板块（96/57 标签）极差 0.19~0.21 信号强，小板块（18 标签）极差 0.08 信号弱（主题纯，体检价值本就低，可接受）。
- **低分线：板块内相对线**——该板块全部构成标签 cosine_mean 的 P25 分位（自适应板块主题纯度）。固定 0.5 线在 tag×aux 场景失效：实测板块内 cosine min 0.51+（文本 embedding 同域底噪高），固定线低分占比恒 0%。（原 direction threshold 引用一并移除。）
- 向量缺失：tag identity embedding 或构成标签 embedding 任一侧缺失，跳过该比对并计数（不按 0 计入）；构成标签自身缺 embedding → 该标签 insufficient。
- 事实画像 diff（uncaptured_candidates）：`topic_tag_board_labels ⋈ topic_tag_semantic_labels ⋈ article_topic_tags` 聚合，排除已挂载构成与 disabled；按 member_tags 降序、上限存 top 50（前端/扩充召回再取前 N）。
- 量级：单板块 96 构成标签 × 477 挂载 tags ≈ 4.6 万次内存 cosine，毫秒级。

### D5: LLM 评分与打扰裁决合并为一次调用

prompt 模式对标升级建议的单决策空间纪律：输入 = 版块名/描述 + metrics + 未收编候选 top 若干 + 既有构成清单，输出 JSON `{score: 0-100, comment: string, notify: boolean}`。

- `notify` 仅在 score 低于告警阈值（`board_health_alert_threshold`，默认 60）时被尊重；正常分数忽略 notify。
- 坏 JSON / 超时 → 降级：统计照常落快照，`llm_score/llm_comment` 为 NULL，本轮跳过通知，不重试。
- 备选「两阶段调用（评分后再裁决打扰）」被否：多一倍调用成本，LLM 在同一份数据上做两次本质相同的综合判断，合并语义等价。

### D6: 通知

- 触发条件：`llm_score < board_health_alert_threshold`（或 score 为 NULL 的降级轮不通知——异常通知只对有评分的轮次生效）且 LLM `notify=true`。
- 冷却：`board_health_notify_cooldown_days`（默认 3），查 notification 表最近一条同 LinkType/LinkID 的时间。
- 落通知：`type=warning` 语义沿现有 success/error 之外新增枚举值（若前端样式仅二态则复用 error 档，实现时定，不新增前端工作），LinkType='board'，LinkID=board_id。

### D7: 扩充召回接入 — `recallExpandAuxCandidates` 前置 member-derived 主路

```
manualGenerate(expand, aux, target)
  → 读目标版块最新体检快照
  → 快照存在且 uncaptured_candidates 非空：
      候选 = 清单前 board_health_member_candidates_top（默认 20）
      排除已挂载（生成时已排除，保险再滤）
      days>0 时按近 days 天文章引用过滤（批量 EXISTS，同现有相似路实现）
  → 无快照或清单为空：回退现有相似路+共现路（行为不变）
```

- 组合路（expand×composite）不动。
- suggestion 管道（hash 幂等 / dismiss 冷却 / LLM 二分类裁决 / per-row 勾选确认）零改动，仅候选来源变化。

### D8: 配置项（ai_settings，均可缺省）

| key | 默认 | 说明 |
| --- | --- | --- |
| `board_health_min_sample_size` | 5 | 标签级归因最小样本量，低于标 insufficient |
| `board_health_alert_threshold` | 60 | 评分告警线（0-100） |
| `board_health_notify_cooldown_days` | 3 | 同版块异常通知冷却天数 |
| `board_health_member_candidates_top` | 20 | 扩充召回取未收编候选前 N |

### D9: API 端点（只读）

`GET /api/semantic-boards/:id/health-checkup?trend=N`（N 默认 7，1-90 白名单校验）：返回 `{ latest: 快照|null, trend: [{checked_at, llm_score}] }`。版块不存在 404；disabled/非 board 返回 latest=null 空态（对标红线 17 端点纪律：disabled 不是 404）。MUST NOT 写库或触发体检。收编/添加/移除复用既有 composition 接口，不新开端点。

### D11: 前端形态 A —— 体检治理面板吸收构成管理，BoardCompositionPanel 退役

用户决策：原「板块内容」tab 的构成面板默认展示（aux chips 纯列表 + ref_count）信息价值低，与体检评价合并为单一治理主面板。

- 平移：picker 添加（搜索/多选/确认 + 不回填提示）、行内移除（confirm 文案不变）、CompositeCreateDialog、组合挂载移除——全部走既有接口，后端零变化。
- 废弃：aux chips 纯展示与 ref_count 展示（质量表每行含 label，质量信息取代引用计数）。
- 新增交互：未收编候选行「收编」= 直接调 addComposition（手动挂载特权，不走 LLM 建议管道，不碰红线 10）；toast 反馈 + 刷新构成与候选。
- 无快照降级：徽章「等待首轮体检」占位、质量列「待体检」、构成管理照常（面板不依赖体检数据可用）。
- LaneDynamicsPanel 从 BoardCompositionPanel 内迁出，TagsPage 直接渲染（位置视觉不变）。
- 影响面：ui-impact 升 major（信息架构变化），ui-design 八节重写 + 原型升级四态；后端接口零变化，specs 仅补收编动作 scenario。

## Risks / Trade-offs

- [LLM 评分轮间漂移 → 趋势锯齿] → prompt 锚定「基于给定指标打分」，同一组指标应得同一档分；趋势看趋势线不看单点。
- [小板块贴合度信号弱（主题纯，构成标签挤在同一簇，极差实测 0.08）] → 接受：小板块体检价值本就低；UI 质量表按 cosine_mean 排序展示，弱排序不误导（不做过细的档位划分）；min_sample_size 兜底极小板块。
- [贴合度口径与外部标签基线有重叠（板块外随机标签 cos_mean 实测 0.571 vs 板块内均值 0.653）] → 接受：体检是板块内构成标签互比排序，不做板块内外判别；LLM 评分输入为板块内相对值。
- [口径切换后历史快照 labels 字段语义变化（samples 从单 aux 行数变为挂载 tag 数）] → 接受：趋势只画 llm_score 不涉及；latest 展示永远来自最新口径快照；不做历史快照迁移。
- [手动补跑日报产生同日多条快照] → 接受（真实数据点）；趋势序列原样展示。
- [体检重算 cosine 对大版块的性能] → 单版块毫秒级内存计算 + 每日一轮，无热点；若未来量级恶化再考虑增量/缓存（Non-Goal 现在不做）。
- [member-derived 候选因快照滞后（最新快照是昨日）漏掉新标签] → 接受：扩充生成本就是用户低频手动操作，昨日候选足够；用户也可先手动跑日报刷新。
- [合并 LLM 调用中 notify 裁决质量] → notify 仅低分轮生效 + 通知冷却兜底，最坏情况是多/少一条通知，无数据风险。

## Migration Plan

1. 迁移：建 `board_health_snapshots` 表（单表，无回填需求——首轮体检自动产生数据）。
2. 部署顺序：后端（表+job 尾步+端点+召回接入）→ 前端（体检卡）。无前后端兼容问题（端点新开，前端新组件）。
3. 回滚：下线前端组件 + 忽略 job 尾步即可；表留存无害（纯日志性数据），如需彻底回滚 drop 表（无 FK 被依赖）。
4. 首轮体检：部署后首次日报生成（或手动触发日报）即产生首批快照；此前端点返回空态（前端已设计空态文案）。

## Open Questions

- 通知 type 枚举是否新增 warning 档（D6）：实现时看前端通知中心现有样式档位，二选一（新增枚举 or 复用 error 档），不影响 spec 行为。
- 趋势默认条数 N 的前端默认值（spec 定 7，实现时可视面板高度微调，1-90 白名单内）。
