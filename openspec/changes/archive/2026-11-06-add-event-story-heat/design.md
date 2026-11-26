# Design — 事件线与热度证据

> 图谱与代码落点见 [`data-map.md`](data-map.md)。术语：**事件** = `daily_report_threads`（代码字段名保留 `thread`），**事件线** = `board_stories`（新增），**话题** = `board_persistent_topics`，**分区** = `daily_report_sections`（展示容器，不是身份）。

## 1. 问题定位

热度现在的四个口径（矩阵桶峰值 / 演进合计 / 行序事件合计 / 雷达分区数）都从 `daily_report_*` 派生，而 `daily_report_*` 是**生成策略的产物**：事件数随 LLM 的 `k = clamp(round(lines/2.5), 2, 6)` 分桶变化，分区数随话题分桶阈值变化。于是「热度」测的是编辑流水线的形状，不是世界的动静。

修法不是换公式，是**换数据源**：热度从原始观察（文章 + 来源 + 信源时间）算，事件身份单独建一层。日报从生产者降级为消费者。

## 2. 三层模型

```
版块 semantic_boards
  └── 话题 board_persistent_topics      跨月主题（已有，不动）
        └── 事件线 board_stories         跨天事件（新增）
              └── 事件 daily_report_threads  某天的一次发生（已有，加 story_id）
                    └── 文章 articles
board_stories ──< story_signals          热度证据（新增，身份与热度分离）
```

展示维度与身份维度正交，见 `data-map.md` §2。`section`（分区）只属于展示维度，**任何判定、计数、热度都不得引用它**。

**为什么事件线挂在版块而不是话题下**：话题是主题，一个主题下可以有多个事件；且话题会漂移/吸尘归档，把事件线绑死在话题上会让历史归属不稳定。`lane_id` 只记录首次归属，用于展示分组，不参与身份判定。

**为什么不复用话题当事件**：实测同日一话题可被拆成 1–6 个 section（`lane_snapshot_repository.go:437` 的 `byDate map[string][]laneSectionRow` 即证），说明话题 > 事件。把话题当事件会让「今天这条话题有几件事」永远答成 1。

## 3. 事件指纹

### 3.1 产出位置

指纹在 `tagmanagement/service/core/extractor_section.go` 的**融合提取调用**内产出，不新增调用。

理由：
- 该调用已经在读文章正文并产出 `event` 标签，加指纹是同一次语义理解的副产品（token 增量约 30–50）。
- 若另起一次调用做 fact 抽取，每条文章成本 +30%，而 Syntopica 没有独立的结构化步骤。
- 标签本身的粒度已实证合适：`category='event'` 10,077 个，平均 1.03 篇文章、1.01 天，本身就是「一次具体发生」。

### 3.2 字段

```
subject        主体原文（展示用）
subject_key    归一化实体键 person:trump / company:bytedance / org:pboc / product:gpt-5 / other:*
action         动作（发布/宣布/涨价/回应/下架）
object         对象（产品/政策/资产/事项）
occurred_at    发生日期（可空）
```

### 3.3 校验

用已有 `metadata.event_keywords`（覆盖 99.7%）做一致性校验：`subject` 与 `object` 必须能在关键词里找到（允许包含与别名）。

- 通过 → `fingerprint_trusted = true`
- 不通过 / 关键词缺失 → `fingerprint_trusted = false`，标签照常入库，归属判定走降级路径

这条校验是免费的（纯字符串比对），且能挡住模型把「市场情绪」写成 subject 这类错误。

### 3.4 为什么不用 auxiliary labels

实测 `topic_tag_semantic_labels`（auxiliary）覆盖 52.3%，且内容是实体锚点而非主谓宾成分：

| event 标签 | auxiliary | 缺什么 |
|---|---|---|
| 阳光电源股价暴涨 | A股 / 美国中期选举 / 储能 / 阳光电源 / IRA法案 | 无动作、无对象 |
| 黄金价格反弹至 4100 美元 | 美联储 / 美元 / 黄金 / 加息 / 期货 | 无动作 |

`person` 标签同样不可用：330 个里约一半是完整事件句（「亚当·汉密尔顿击败罗杰·马歇尔」），且事件的 `tag_ids` 中只占 1.3%。

## 4. 主体归一化

### 4.1 三层匹配

```
① subject_key 精确匹配（lower + 下划线）
② subject_key embedding cos ≥ 0.92
③ subject 字面归一（lower / NFKC / 去空格 / 去「公司」「集团」后缀）
未命中 → 登记新主体候选
```

`subject_key` 由模型在同一次提取调用里产出，格式 `类型:规范名`。模型天然知道 Trump = 特朗普 = 川普、字节跳动 = ByteDance，这是它比后处理更合适的原因。

### 4.2 为什么 embedding 只做兜底

| 资产 | 实测 | 结论 |
|---|---|---|
| `person` 标签 aliases | 332 个，**0%** | 不可用 |
| `event` 标签 aliases | **0%** | 不可用 |
| `keyword` 标签 aliases | 7.3%（842/11559） | 覆盖太低 |
| 标签 embedding 覆盖 | 99.9–100% | 唯一可用的向量资产 |

现有标签体系**没有实体层**（person 标签一半是事件句、搜不到 Trump/川普；keyword 是产品技术名），所以不能靠 `topic_tags.aliases` 归并。

embedding 阈值取 **0.92**，远高于事件召回的 0.5——实体误归并的代价（把 A 公司的事件合并到 B 公司）高于漏归并（同一主体被拆成两条线，下次会合并）。

### 4.3 存储

- `subject_key` 存 `board_stories.subject_key`
- `subject_embedding` 存 `board_stories.subject_embedding`（可空，仅归一化兜底用）
- 本期**不建独立实体表**；将来若需要实体管理，再把 `subject_key` 提升

## 5. 归属判定

### 5.1 召回（宽进）

```sql
-- 候选事件线：同版块、14 天窗口、活跃、向量近邻
SELECT id, subject_key, action, object, occurred_at,
       embedding <=> $1 AS dist
FROM board_stories
WHERE board_id = $2
  AND merged_into IS NULL
  AND last_seen_date >= $3          -- 今天 - 14 天
  AND status = 'active'
ORDER BY dist
LIMIT 10;
```

向量阈值放宽（`dist <= 0.5`），只求不漏；**不设「距离小于 X 即合并」**。

### 5.2 判定（严出）

```
1. subject_key 不同                                  → 新建事件线
2. subject_key + action + object 全同（含别名/包含）  → 同一次发生
3. subject_key + object 同，action 不同              → 同一事件线，is_development = true
   └ 例外：action 语义反向（发布 vs 召回/下架/否认） → 不套用本层，走第 5 层
4. subject_key 同，object 不同                       → 新建事件线
5. 指纹不可信 / 无法判定                             → LLM 四分类（带置信度）
   └ 置信度 < 阈值 或 调用失败 或 ROUNDUP             → 新建事件线（保守）
```

**为什么 subject 一票否决**：「A 公司涨价 15%」与「B 公司涨价 10%」向量距离极近，但主体不同；「某厂发布新品」与「该产品开售」向量距离较远，但主体与对象相同、动作是阶段推进。判定的判据是实体，不是语义距离。

**为什么阶段用 action 差异而非阶段枚举**：中文新闻里同一事件的阶段动词不可穷举（预告/曝光/发布/上架/开售/降价/召回/回应/辟谣）。只要 `subject_key+object` 对上、`action` 不同，就视为同一事件的推进。真正会误判的是「同一主体对同一对象的两个不相关动作」，例如「发布 A 产品」与「召回 A 产品」，这类由反向动作表拦下，交 LLM 兜底。

### 5.3 综合稿

`scope = composite`（`tag_ids` 中 event 标签 ≥ 2）的事件不建身份。

实测：6,265 篇文章有 1 个 event 标签，495 篇 2 个、806 篇 3 个、30 篇 4 个、65 篇 5 个——约 18% 是多事件文章。这类文章被 LLM 归进一个事件时，事件本身就不是单一事件，拿它建事件线会把多个事件粘在一起。

### 5.4 合并与地址

- `merged_into` 指向存活事件线；读取时一跳解析（不允许两层链）。
- 合并条件：两条事件线被同一事件以 `SAME_OCCURRENCE` 连接。本仓库没有参考实现的 `groupReview` 多 provider 复核（airouter 只按 `Capability` 选路由，`ChatRequest` 无 provider 覆盖字段），实现降级为：规则层三重指纹全同直接合并；模型路径按单分类器置信度 ≥ 0.75 保守处理。该偏离记入 `review.md`。
- 合并时把旧记录直接指向最终存活者，避免多层链。

### 5.5 人工纠正

事件级标记落在 `daily_report_threads.story_manual`（人工指派=写 `story_id` + `story_manual=true`；人工拆分=`story_id=NULL` + `story_manual=true`）。自动归属与回填一律跳过 `story_manual=true` 的事件；`board_stories.origin='manual'` 只表示事件线本身人工创建/合并，不能替代事件级标记。仓库没有参考实现中的 `grouping_overrides` 表，故不复用。

## 6. 热度

### 6.1 表

```sql
CREATE TABLE board_stories (
  id                bigserial PRIMARY KEY,
  board_id          bigint NOT NULL REFERENCES semantic_boards(id) ON DELETE CASCADE,
  lane_id           bigint REFERENCES board_persistent_topics(id) ON DELETE SET NULL,
  title             text NOT NULL,
  subject           text,
  subject_key       text,
  action            text,
  object            text,
  occurred_at       timestamptz,
  status            text NOT NULL DEFAULT 'active'
                      CHECK (status IN ('active','settled','merged')),
  first_seen_date   date NOT NULL,
  last_seen_date    date NOT NULL,
  thread_count      integer NOT NULL DEFAULT 0,
  article_count     integer NOT NULL DEFAULT 0,
  participant_count integer NOT NULL DEFAULT 0,
  embedding         vector,
  subject_embedding vector,
  merged_into       bigint REFERENCES board_stories(id),
  origin            text NOT NULL DEFAULT 'rule'
                      CHECK (origin IN ('rule','model','manual')),
  created_at        timestamptz NOT NULL DEFAULT now(),
  updated_at        timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX board_stories_recall_idx
  ON board_stories (board_id, last_seen_date DESC)
  WHERE merged_into IS NULL AND status = 'active';

CREATE INDEX board_stories_subject_idx ON board_stories (board_id, subject_key);

ALTER TABLE daily_report_threads
  ADD COLUMN story_id bigint REFERENCES board_stories(id) ON DELETE SET NULL,
  ADD COLUMN scope text NOT NULL DEFAULT 'single'
    CHECK (scope IN ('single','composite')),
  ADD COLUMN is_development boolean NOT NULL DEFAULT false,
  -- 人工纠正标记（spec event-story「人工纠正优先」）：true = 人工指派/拆分
  -- 的结果，自动归属与回填一律跳过；人工拆分时 story_id 置 NULL 但本列保持 true。
  ADD COLUMN story_manual boolean NOT NULL DEFAULT false,
  -- 跨期去重的展示层载体（tasks 6.1-6.3）：body=正文（默认）、flash=快讯
  -- （重述降级）、hidden=未展示（预留）。只改展示，身份/signals/热度
  -- 永不读它；读路径（日报详情）本期只回 body（丢弃 flash/hidden，字段
  -- 留供未来快讯 UI）。
  ADD COLUMN display_tier text NOT NULL DEFAULT 'body'
    CHECK (display_tier IN ('body','flash','hidden'));

CREATE INDEX daily_report_threads_story_idx ON daily_report_threads (story_id);

CREATE TABLE story_signals (
  story_id           bigint NOT NULL REFERENCES board_stories(id) ON DELETE CASCADE,
  article_id         bigint NOT NULL REFERENCES articles(id) ON DELETE CASCADE,
  feed_id            bigint NOT NULL,
  participant_key    text NOT NULL,
  kind               text NOT NULL DEFAULT 'editorial'
                       CHECK (kind IN ('editorial','signal')),
  observed_at        timestamptz NOT NULL,
  observed_at_source text NOT NULL DEFAULT 'pub_date'
                       CHECK (observed_at_source IN ('pub_date','fallback')),
  PRIMARY KEY (story_id, article_id)
);

CREATE INDEX story_signals_time_idx ON story_signals (observed_at DESC);
CREATE INDEX story_signals_story_idx ON story_signals (story_id, participant_key, observed_at DESC);
```

### 6.2 公式

```sql
-- 事件线热度（窗口 48h、半衰期 24h、每参与者只计一次）
WITH per_participant AS (
  SELECT story_id, participant_key, MAX(observed_at) AS last_at
  FROM story_signals
  WHERE observed_at >= $1::timestamptz - interval '48 hours'
  GROUP BY story_id, participant_key
)
SELECT story_id,
       SUM(power(0.5, EXTRACT(EPOCH FROM ($1::timestamptz - last_at)) / 3600.0 / 24)) AS heat,
       COUNT(*) AS participants
FROM per_participant
GROUP BY story_id;
```

话题热度同结构，半衰期 72h、窗口 14 天，事件线通过 `COALESCE(merged_into, id)` 归并。

`rule_version = 'heat-v1-48h-hl24h'` 写入响应，便于口径变更后区分。

### 6.3 为什么热度不落新表

事件线热度是纯函数，可由 `story_signals` 实时算；只有需要历史曲线的场景才落快照。本期不落（proposal 的「不含」），矩阵与雷达读时计算。计算量：单版块 14 天 × 数百事件线的聚合，走 `story_signals_story_idx` 可在毫秒级完成。

### 6.4 落后源

某 feed 超过 `max(3 × refresh_interval, 90 min)` 未成功采集时：
- 该 feed 相关的当日热度计算标记 `incomplete`；
- 补抓完成后按 `pub_date` 重算受影响日期；
- 因为公式无模型输出，重算结果与首次一致。

## 7. 展示口径

| 产物 | 新口径 | 旧口径 |
|---|---|---|
| 矩阵日格数字 | 当日该话题的事件线数 | 当日各分区事件数的最大值 |
| 矩阵格颜色 | 当日最大事件线的 `participant_count` 分档 | 无 |
| 演进节点 | 当日事件线数 | 当日各分区事件数之和 |
| 行序 | `lane_heat` 降序 → 事件线数 → id | 全窗 clueTotal → sectionTotal → id |
| 雷达关注度 | `lane_heat`（保留一位小数） | `today_sections*2 + last3 + (last3-prev3)` |
| 升温判据 | `story_heat` 环比 ≥ 2 且事件线数 ≥ 3 | 分区数 ≥ 近 3 日均 2 倍且 ≥ 3 |

**阈值标定**：旧阈值 50/15 是 `section` 计数量纲下的经验值，换到 `lane_heat`（浮点、带衰减）后不可直接沿用。实现时先跑一遍全量历史计算，取当日活跃版块 `lane_heat` 的 P80 / P20 作为热点 / 常温初始阈值，写入配置并在本文附录记录分布；`RADAR_HOT_ATTENTION` / `RADAR_WARM_ATTENTION` 常量位置保留，只改取值。

## 8. 迁移与回填

### 8.1 迁移

- 新增两张表 + 事件三列，全部 `ADD COLUMN`（可空或有默认），无表重写。
- 走项目现有版本迁移（`postgres_migrations.go`），不用 GORM AutoMigrate 做结构性变更。
- 回滚：三列与新表可 `DROP`，不影响存量日报读取。

### 8.2 回填

CLI：`cmd/backfill-stories`，参数 `--board <id>` / `--from <date>` / `--to <date>` / `--dry-run`。

流程：

```
for board in boards:
  for date in [from..to] ascending:
    events = events(report(board,date)) where story_id is null
    for t in events:
      if t.scope == 'composite': continue
      story = judge(t)          -- 召回 + 五层判定
      link(t, story); upsert_signals(story, t.related_article_ids)
```

幂等：只处理 `story_id IS NULL` 且非人工标记的事件；事件线查找走「同 board + subject_key + action + object 精确匹配」优先，避免重复建。

预估量：20 版块 × 180 天 × 15 事件 ≈ 5.4 万条，每条一次向量召回，单机分钟级。`--dry-run` 输出将新建/合并/跳过计数，供人工先看。

### 8.3 降级

未回填期间：
- 矩阵格值按「非综合稿事件数」作事件线数上界，footnote 标「事件身份未回填」；
- 参与者数按 `COUNT(DISTINCT feed_id)` 从 `related_article_ids` 直接算（不需要 signals）；
- 雷达关注度退回「按事件数 + 参与者的简化热度」，标同一条降级提示。

## 9. 风险与取舍

| 风险 | 处置 |
|---|---|
| 指纹质量差，subject 抽取不稳定 | 用 `event_keywords` 校验，不可信就走 LLM；不信任任何未校验的指纹 |
| `subject_key` 两次生成不一致（`person:trump` vs `person:donald_trump`） | embedding 兜底（0.92）+ 字面归一层；仍不一致时按不同主体处理，下次合并 |
| `action` 语义方向相反被误判为阶段 | 反向动作表（发布 vs 召回/下架/否认），命中则跳过阶段规则走 LLM |
| 事件线数量爆炸（每个事件一条线） | 可接受的失败模式：误合并比误拆分代价高；主指标是误合并率 |
| 回填把历史事件粘错 | `--dry-run` + 人工抽检 20 条后再执行；回填只写 `story_id` 与 signals，不改日报正文 |
| 雷达阈值沿用旧值导致全站「常温」 | 按真实分布标定，写入 design 附录 |
| 事件 embedding 部分为空（watch 物化） | 这些事件只走文章集合重叠判定，不走向量召回；无重叠则新建事件线 |

## 附录 A：雷达阈值标定（2026-11-06，真库）

**方法**：`cd backend-go && go run ./cmd/calibrate-heat --days 14 --board 0`（只读）。样本 = 全版块、每版块以最新报表日为锚回看 14 天的 **正值** `lane_heat`（0 热度泳道属无信号区，不参与分位，否则会把 P20 压到 0 使常温档退化）。

**结果**：

| 项 | 值 |
|---|---|
| 样本数 | 734 |
| P20 | **0.7013741541042114** |
| P80 | **3.3259922121090404** |
| `rule_version` | `heat-v1-48h-hl24h` |

**写入阈值**（`front/app/features/tags/components/console/boardRadarHelpers.ts`，保留一位小数）：

| 常量 | 旧（临时） | 新（标定） |
|---|---|---|
| `RADAR_HOT_ATTENTION` | 10 | **3.3** |
| `RADAR_WARM_ATTENTION` | 3 | **0.7** |

**敏感性对照**：`--days 30 --board 0` 输出与 14d **完全一致**（734 / 0.7014 / 3.3260）——因为 `lane_heat` 本身只有 14 天窗口，锚日再往前采样也只会得到 0，被正值过滤掉，故 30d 不引入额外样本。结论：阈值对锚日跨度不敏感，只对窗口口径敏感（窗口变了必须重标）。

**为何旧值不可用**：临时值 10 / 3 是在**热度全 0** 的未回填态下拍的，与真实分布差一个量级；若沿用，界面几乎不会有行进入「关注/热点」档。

## 10. 不做

- 不做事件线级 LLM 综述（现有话题快照已覆盖）。
- 不接社媒/讨论帖信号（`kind='signal'` 只预留）。
- 不做热度历史快照表与曲线页面。
- 不改话题分桶阈值、不改日报分区生成算法。
- 不建独立实体表（`subject_key` 先存在事件线上）。
