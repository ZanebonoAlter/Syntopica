<!-- complexity: complex -->
<!-- ui-impact: none -->
<!-- constraint-domains: topic-graph, daily-report, semantic-board -->
<!-- doc-impact: flow -->

# 开启事件线跨天延续（拆成两层判定）

## Why

用户原话（2026-11-07，叙事工坊事件线系列）：

> 「事件线比 embedding 关联线索更靠谱」
> 「今日升温应该指事件线」

上一条 change（`surface-story-events-in-radar`）把雷达右栏改成事件线粒度时留下一笔硬账：**事件线实际上不是「跨天同一事件的聚合」，而是「当天的一条线索」**。三条实测证据（真库只读复核 + 读码，2026-11-08）：

1. **跨天事件线 0 条**。`board_stories` 活跃 **1262** 条（2026-11-10 复核；2026-11-08 原测为 910 条），**全部** `first_seen_date = last_seen_date`；`daily_report_threads` 共 13,491 条、其中 960 条已归属。spec 定义「事件线 = 跨天同一事件的聚合」（`openspec/specs/event-story/spec.md`「事件线数据模型」），当前实现**不满足**。
2. **「今天可能升温」结构性为空**。事件线级升温要求该线自身有近 3 日基线；今日 135 条有报道证据的事件线里**只有 2 条**有历史基线，「今日新起」档承担 120 条。升温和「今日该读」因此都退化成「今天新冒出来的东西」。
3. **召回没问题，是闸门拒了**。同版块相邻日同 `subject_key` 的事件线对 147 对，其中 105 对落在召回半径（cosine distance ≤ 0.5）内、34 对连 `object` 都完全相同。样例：线 4571（11-03）与线 4581（11-04）指纹**一字不差**，召回时向量距离 **0.104**，仍然没有接上。

拒它的原因在代码里，不在数据里：

- **根因一：信任判据选错了锚点**（`backend-go/internal/topicgraph/service/story_fingerprint.go:ValidateEventFingerprint`）。校验要求 `subject` 与 `object` **各自**都能在 `event_keywords` + `aliases` 里双向命中。但 `object` 槽位装的是**事件参数**（事件里被测量的那个值），关键词装的是**标签名**的词 —— 数值永远命不中标签名的词。实测：标签 `id=215382`「人民币兑美元中间价下调」`subject_key=other:人民币兑美元中间价`、`action=下调`、**`object=6.7253`**，关键词 `["人民币","美元","中间价","下调","汇率"]` → 永久 `fingerprint_trusted=false`，进而被「指纹不可信一律不合并」的合并闸门挡住。有指纹的 7,910 条标签里（2026-11-10 复核；信任值按判据现算、不落库），其中 **765 条** 的 `object` 以数字开头 —— 这一类**在现行判据下永远不可能通过校验**。
- **根因二：同日去重与跨天延续共用一套「object 严格相等」判据**（`backend-go/internal/topicgraph/service/story_assign.go:classifyStoryCandidate`）。R-14 的严格相等是**为同日去重**设计的（同一篇文章切出的多段线索，误合并代价极高），对跨天延续是错的：汇率中间价每天数值不同，**正因为数值在变才需要一条线串起来**。同一函数里，阶段推进（action 不同）的日期判据又拿**事件线的原点日**（`board_stories.occurred_at`，建线那天）当基准，于是「这条线在第 4 天有新动作」永远超不过「≤1 天」的窗口。
- **根因三：召回盲区（2026-11-10 复核新增）**。召回 SQL 硬性要求 `board_stories.embedding IS NOT NULL`（`story_repository.go:199`），而建线时 `Embedding: thread.Embedding`（`story_attribute.go:509`）在线索无向量时写 NULL → 该事件线**永久退出召回池**。真库活跃线 1262 条里 **111 条无向量（8.8%）**；explore-findings 的 DeepSeek 样本（4333/4353/4378）正是因此各自单日成线。召回层不补，判据再对也没有候选。
- **根因四：`subject_key` 漂移（实体层已能归一）**。同一主体被模型产出成多种键：`product:deepseekharness` / `product:deepseek_harness` / `person:deepseek_harness_团队` / `other:deepseek_harness_插件机制详解`。类型前缀不同 → `NormalizeSubjectKey` 归不了一 → subject 一票否决直接挡掉本可同线的候选。实体层（`normalize-event-subject-entities`，2026-11-10 起默认 shadow）已能跨叫法归一（实测：`泸州市中级人民法院`→`四川省泸州市中级人民法院`、`PAW3955`→`PAW Next I`），但红线 15 当前禁止事件线判定路径读实体 ID —— 本 change 显式解禁并接线（见 design D14）。

用户已拍板：**拆成两层判定** —— 同日去重保持现状（严格），跨天延续另立判据。本 change 就是把那第二层判据补上，并修掉让数值型事件**永久不可信**的信任判据。

## What Changes

- **归属判定拆成两层**（`service/story_assign.go`）：
  - **层一 · 同一次发生（same occurrence）**：候选事件线**当天**已有证据（= 同一篇日报、同批文章切出的多段线索）→ 判据**逐字不变**：subject_key 精确相等一票否决 → action 相容 → object 严格相等 → 同日阶段。误合并代价最高的一档**一点不放松**。
  - **层二 · 跨天延续（cross-day continuation）**：候选事件线的**最近证据日早于**本次报告日 → 判据 = 层一判据 **加两处放宽**（见 design D3）：① 「阶段推进」的日期基准从**线原点日**换成**线最近证据日**（只放宽不收紧，窗口仍是 ≤1 天）；② 新增「**参数型 object 值可变**」的延续（两侧 object 同为参数型时，值不同也允许挂到同一条线）。
- **事件指纹新增结构化字段 `object_type`**（枚举 `entity` / `value`，**同一次提取调用**产出，零新增 LLM 调用）：`entity` = 被作用的对象（产品/政策/资产/事项），`value` = 事件参数/度量值（数值、金额、比率、时间点）。它是「数值型 object 与实体型 object」的**唯一判据来源**（确定性语法只做必要条件质检，不用关键词黑名单、不用长度阈值）。
- **修复召回盲区**（`service/story_attribute.go` + `repository/story_repository.go` + `cmd/backfill-stories`）：建线时若线索无向量，SHALL 用事件标题现算 embedding 写入（SHALL NOT 将 NULL 写入后不再处理）；`cmd/backfill-stories` 新增 `--fill-story-embeddings` 回填存量无向量事件线（只补 `embedding IS NULL`，不覆盖已有向量）。召回 SHALL NOT 因缺向量而把事件线永久排除。
- **主体身份接入实体层**（`story_assign.go` + `story_attribute.go` + 新增 `story_entity_identity.go`）：`board_stories` 新增可空列 `subject_entity_id`（建线时取自事件标签 `metadata.entity_subject.entity_id`，缺失/技术失败写 NULL）；判定时主体身份 SHALL 按「`subject_key` 精确相等 **或**（两侧 `subject_entity_id` 相同且均非空 **且** subject 归一化后精确相等）」判同一主体，两者都不满足即否决；不得用包含关系（会把配偶/国家/机构当同一主体）；裸 `subject_key`（仅类型前缀、无规范名）不参与实体越过。实体 ID 只做**加法**（能把漂移的键判成同一主体），SHALL NOT 推翻既有 `subject_key` 相等；任一侧缺失实体 ID 时回退既有的 `subject_key` 三层口径。**红线 14「`subject_key` 一票否决」随之修订为「主体身份一票否决」**（红线 15 的「不参与事件线判定」由本 change 解禁）。
- **信任判据修法**（`service/story_fingerprint.go:ValidateEventFingerprint`）：`subject` 的关键词命中**仍是硬要求**；`object` 的关键词命中要求**只在 `object_type=entity` 时适用**；`object_type=value` 时 object 免关键词命中，但必须通过「以数字开头」的确定性质检（过不了就按 entity 回落，保守）。存量没有 `object_type` 的标签**按 entity 处理**（保守，等价于现状）。
- **新增跨天延续的合并证据与可复核标记**：经层二放宽（阶段推进 / 参数型值变化）的挂载标 `is_development=true`（延续 = 该事件线的新阶段，跨期去重因此不把延续事件降级成快讯）；判定理由进日志枚举；跨天线可由 `first_seen_date < last_seen_date` 直接识别。
- **矩阵日格颜色的参与者数改为按当日窗口计算**（`repository/lane_snapshot_repository.go`）：跨天线一旦出现，生命周期累计的 `board_stories.participant_count` 会把**未来证据泄漏到历史日格**，违反红线 28 的「当日最热事件线」口径 → 改为当日窗口内去重 `participant_key` 数。
- **回填与回滚路径**：`cmd/backfill-stories` 新增 `--reattribute`（重算指定范围内的事件身份）、`--snapshot <file>` / `--restore <file>`（写前快照与还原）；**默认不重写历史**，新日报从部署日起自然产生跨天线。`cmd/backfill-fingerprints` 新增 `--fill-object-type`（只补存量标签缺失的 `object_type`，不覆盖既有五字段）。
- **改动能力**：`event-story`（主：指纹字段与信任判据、事件线数据模型、两层归属判定、跨天延续的证据与可复核、存量重算与快照回滚）、`story-heat-evidence`（矩阵日格颜色的「当日」口径）。无新能力、无新页面、无新路由、无前端改动。

## Capabilities

### New Capabilities

（无 —— 本 change 只修订既有能力，不新建能力，避免出现重复 spec。）

### Modified Capabilities

- `event-story`（`openspec/specs/event-story/spec.md`）：
  - MODIFIED「事件指纹结构化产出」：指纹新增 `object_type`；`object` 的关键词一致性命中的适用范围按角色区分（数值型 object 不再永久不可信）。
  - MODIFIED「事件线数据模型」：`board_stories` 新增可空列 `object_type`（候选侧的角色判据）。
  - MODIFIED「事件归属判定顺序」：改为**两层判定**（层一同日严格不变 / 层二跨天延续两处放宽）；主体身份改为「`subject_key` 相等 **或** `subject_entity_id` 相同」一票否决。
  - MODIFIED「事件线数据模型」：`board_stories` 除 `object_type` 外新增可空列 `subject_entity_id`（实体层主体身份，缺失回退 `subject_key`）。
  - ADDED「事件线召回可用性」：建线 SHALL 保证 `embedding` 非空（缺则用标题现算），存量缺向量由 `--fill-story-embeddings` 回填；回填前召回行为不变。
  - ADDED「跨天延续的合并证据与可复核标记」：层二的证据门槛、`is_development` 标记、跨天线识别口径。
  - ADDED「跨天延续的存量重算与快照回滚」：`--reattribute` / `--snapshot` / `--restore` 契约与「默认不重写历史」。
- `story-heat-evidence`（`openspec/specs/story-heat-evidence/spec.md`）：
  - MODIFIED「热度消费口径一致」：矩阵日格颜色的参与者数 SHALL 取**当日窗口**内去重参与者数；一条跨天事件线在其有事件的每一天各计一次，SHALL NOT 用生命周期累计值污染历史日格。

## Impact

- **后端（判定与身份）**：
  - `backend-go/internal/topicgraph/service/story_assign.go`（两层判据；`classifyStoryCandidate` 拆成层一/层二两条路径；判定理由枚举新增）。
  - `backend-go/internal/topicgraph/service/story_object_type.go`（**新增**：object 角色判定的唯一实现 —— 指纹 `object_type` 解析 + 「以数字开头」质检）。
  - `backend-go/internal/topicgraph/service/story_fingerprint.go`（信任判据按角色修订）。
  - `backend-go/internal/topicgraph/service/story_attribute.go`（建线时写 `object_type` 与 `subject_entity_id`；线索无向量时用标题现算 embedding）。
  - `backend-go/internal/topicgraph/service/story_entity_identity.go`（**新增**：story ↔ entity 的只读解析，实体层接口的唯一消费点；不得写实体三表）。
  - `backend-go/internal/topicgraph/service/story_backfill.go`（`--reattribute` 重算身份、快照/还原的编排）。
  - `backend-go/internal/topicgraph/repository/story_repository.go` / `story_models.go`（候选查询带出 `object_type` 与 `subject_entity_id`；`BoardStory.ObjectType` / `SubjectEntityID`）。
  - `backend-go/internal/topicgraph/repository/lane_snapshot_repository.go`（矩阵日格参与者数按当日窗口计算）。
- **后端（提取与迁移）**：
  - `backend-go/internal/tagmanagement/service/core/extractor_section.go` + `extractor_enhanced.go`（提取 schema/prompt 增加 `object_type`，**同一次调用**产出）。
  - `backend-go/internal/tagmanagement/service/core/types.go` + `tagger.go`（`EventFingerprint` 结构与 metadata 持久化）。
  - `backend-go/internal/platform/database/postgres_migrations.go`（**一条**新迁移：`board_stories` 增可空列 `object_type` 与 `subject_entity_id`；无表重写、无回填）。
- **CLI**：`backend-go/cmd/backfill-stories/main.go`（`--reattribute` / `--snapshot` / `--restore` / `--fill-story-embeddings`）、`backend-go/cmd/backfill-fingerprints/main.go`（`--fill-object-type`）。
- **前端**：**无改动**。跨天延续对用户可见的表现是「雷达今日该读 / 今日升温的条目来自跨天线」——那是**既有界面的数据变化**，不是界面变化（详见 ui-design.md「N/A Reason」）。人工复核入口（人工拆分/指派事件线）**另开 change**（`event-story` spec 早有「人工纠正优先」约束，但仓库至今**没有**对应端点与 UI，本 change 不假装它存在）。
- **存储**：一条加列迁移（`board_stories.object_type`、`board_stories.subject_entity_id`，均可空）；`topic_tags.metadata.event_fingerprint` 增一个嵌套键（JSONB，无 schema 变更）；**无表重写、无历史数据自动改写**。
- **红线**：**修订**红线 14 的主体口径为「主体身份一票否决：`subject_key` 精确相等 **或** `subject_entity_id` 相同且均非空」，向量仍只用于召回、不得单独作判定；**解禁**红线 15 的「实体 ID 不参与事件线判定」（接线范围严格限 `subject_entity_id` 的主体身份，实体层三表只读）；不改红线 2/3/7（候选生命周期、手动建话题）；不改话题与事件身份正交；不改分区只是展示容器；不改热度公式（`heat-v1-48h-hl24h`）；**层一（同日去重）除主体身份外判据逐字不变**。
- **文档（实现阶段执行，本阶段不改）**：`docs/reference/flow/topic-graph.md`（红线 14 主体身份口径、红线 15 解禁与接线边界、红线 13 的「本期不建独立实体表」已按现状改为引用红线 15）、`docs/reference/flow/daily-report.md`（事件线归属节的两层判据与实体身份）、`docs/reference/flow/semantic-board.md`（红线 28 的「当日参与者数」口径）。

## 部署后影响与需要的操作

- **部署后用户可见行为**：从部署日起，新生成的日报里**同一件事的连续报道会收进同一条事件线**（此前每天各是一条线）。用户能直接看到的变化：
  - 雷达右栏「今日该读」里出现的条目，其事件线**可能跨多天**（此前每条都是当天的）；
  - 「今天可能升温」档**不再结构性为空** —— 跨天线有了近 3 日基线，升温倍率才有分母（此前几乎全靠「今日新起」兜底）；
  - 日报正文里同一事件的跨天续报**不再被跨期去重降级成快讯**（经放宽的跨天延续标为进展）；
  - 版块态势 14 日矩阵里，跨天线在其有事件的**每一天**各占一格（此前每条线只占一天）。
- **需要用户手动执行的操作**：
  0. **确认实体层在跑**：主体身份接线依赖 `entity_subject.mode` 为 `shadow`（默认）；`off` 下标签没有 `entity_id`，判定自动回退 `subject_key`（不报错，但跨叫法仍会漏）。部署前用只读 SQL 确认 `entity_registry` 有行、`topic_tags.metadata ? 'entity_subject'` 有数据。
  1. **必须跑一次存量重算才会看到历史跨天线**：`cd backend-go && go run ./cmd/backfill-stories --reattribute --dry-run`（先看统计，确认后再去掉 `--dry-run`）。**不加这个开关，存量事件线一律不动**（历史保持单日线的样子）。
  2. **跑重算前必须先快照**：`--snapshot /tmp/story-identity-YYYYMMDD.jsonl`；出问题时用 `--restore <同一文件>` 还原（只动事件身份，不动日报正文、文章与标签）。
  3. **存量标签要吃到「参数型 object」能力，必须补 `object_type`**：`cd backend-go && go run ./cmd/backfill-fingerprints --fill-object-type --dry-run` 预览后实跑（会调 LLM，量级 = 存量 event 标签数，先小批量试点）。**不补也能上线**：没有 `object_type` 的标签按 entity 处理，行为与今天一致（保守），只是数值型事件的跨天串线能力要等补齐后才生效。
- **旧数据如何降级**：
  - 存量事件线**默认不改写**（`first_seen_date = last_seen_date` 的单日线保持原样），界面照常显示，不报错、不冒充「无事件」；
  - 跑 `--reattribute` 后被重算的线：`first_seen_date` / `last_seen_date` 会变成真实跨度，`thread_count` / `article_count` / `participant_count` 随之重算；被合并的线按既有约定置 `status='merged'` + `merged_into` 指向存活者（一跳解析，不允许两层链）；
  - 回退路径有两条：**批量**用 `--restore <快照文件>`；**单条**目前只能用 `story_manual`（人工标记）保护，但该能力**只有数据列、没有端点与界面**，因此本期**不作为可交付的回退手段**（已列入风险表与另开 change 的范围）。
  - 改回旧判据 = 回滚部署（代码侧）；但**已重算的库内状态不会自动回滚**，只能用快照还原 —— 这是必须先快照的原因。
