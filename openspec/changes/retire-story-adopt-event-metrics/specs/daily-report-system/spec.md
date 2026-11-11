## MODIFIED Requirements

### Requirement: 日报数据模型

系统 SHALL 使用 `board_daily_reports` 和 `daily_report_sections` 两张表承载日报。每个 SemanticBoard 每天至多一条 `BoardDailyReport` 记录。

**BoardDailyReport 字段**：id, semantic_board_id, period_date, title, summary, highlights(JSON), dynamics(TEXT), article_count, event_tag_count, cluster_count, status(generating/done/failed), raw_clusters(JSON), prev_report_id(可为空，指向前一日日报), generation_prompt_version, created_at, updated_at。

**DailyReportSection 字段**：id, report_id, cluster_index, cluster_label, cluster_tag_ids(JSON), article_count, best_tier, avg_score, embedding(向量列，维度由模型输出决定, 用于语义匹配), bucket_title(可空，主题方向名), bucket_index(整数，默认 0), created_at。事件数据已迁移至 `daily_report_threads` 表，通过 `section_id` 外键关联。跨天关系通过 `daily_report_section_relations` 关系表表达，status 通过关系拓扑动态推导。

**DailyReportThread 字段**：id, report_id, section_id, title, summary, tag_ids(JSONB), confidence, story_id(可空，只读历史列), scope(single/composite), is_development(布尔，默认 false), story_manual(布尔，只读历史列), display_tier(body/flash/hidden，默认 body), created_at。

`story_id` SHALL 保留为**只读历史列**：新生成或重建的日报 SHALL 恒为 NULL，归属判定链 SHALL NOT 再写入该列；历史日报中已存在的 `story_id` 与其指向的 `board_stories` 行 SHALL 保留不动，读路径 MAY 解析存活身份但 SHALL NOT 对外透出事件线身份。`scope` SHALL 继续由 `tag_ids` 中 `category='event'` 的标签数推导：`≥2` 为 `composite`，否则为 `single`；`composite` 事件 SHALL NOT 进入任何同事件聚簇，也 SHALL NOT 计入聚簇文章数。`is_development` SHALL 恒为 false（其唯一驱动源——事件线阶段判定——已退役），SHALL NOT 作任何判定输入。`story_manual` SHALL 保留为只读历史列（人工指派 / 拆分过的历史行仍为 true），SHALL NOT 触发任何判定或写动作。`display_tier` SHALL 恒为 `body`（唯一写入方——依赖 `story_id` 的跨期去重——已退役），SHALL NOT 影响任何计数、排序或展示过滤。

`highlights` SHALL 恒为数组。每项保留 title/reason/tag_ids，并新增可空 `cluster_id`（当天同事件聚簇 id，服务端计算，模型不输出）；同事件聚簇模式还 SHALL 保存由服务端计算的 `article_count`（簇内文章数，按内容身份去重）、`cluster_mode`（向量聚簇 / 语义向量兜底 / 主体近似）、`data_as_of`（数据截至日）与 `research_at`（研究时点）快照，SHALL NOT 接受模型填写这些数值。标签回退与历史数据 MAY 缺这些字段。正常目标 2–3 项，候选不足允许 1 项、事实不足允许 []。`cluster_id` 读取时 SHALL 同版块一跳解析，坏引用置空，不回写历史 JSON；数值 SHALL 保持生成时快照。

**`highlights` SHALL 恒为 JSON 数组**：落库时 SHALL 写 `[]`（空数组），SHALL NOT 写 JSON `null` 标量——`null` 标量会让 `jsonb_array_elements_text` 抛 SQLSTATE 22023，且在读路径上被前端当作「无内容」而整块隐藏。API 读出时 SHALL 同样恒为数组（历史遗留的 `null` 值 SHALL 在读路径归一为 `[]`），SHALL NOT 向客户端返回 `"highlights": null`。

`raw_clusters` JSON 结构：`[{group_name: string, tag_ids: uint[]}]`，LLM 分组原始结果，用于调试。

日报记录的落库 SHALL 以 (semantic_board_id, period_date) 为冲突键执行原子 upsert：同 board 同日重复落库（无论顺序还是并发）SHALL 收敛为一条记录，SHALL NOT 抛出唯一约束冲突错误。

**同一 report 内 `cluster_index` MAY 重复**：一个聚类/话题的当日内容被拆成多条 section 时（泳道内主题分桶、手动编排、watch 物化），兄弟 section SHALL 共享同一 `cluster_index`，以 `bucket_index`（0 起、组内唯一）区分先后；(report_id, cluster_index) SHALL NOT 建唯一约束。未分桶的 section SHALL 取 `bucket_index=0`、`bucket_title=NULL`。

#### Scenario: 创建日报记录
- **WHEN** 为 SemanticBoard #5 在 2026-05-25 生成日报
- **THEN** 系统 SHALL 在 `board_daily_reports` 表创建一条记录，status="generating"，period_date="2026-05-25"，semantic_board_id=5

#### Scenario: 日报记录唯一性
- **WHEN** SemanticBoard #5 在 2026-05-25 已有一条 status="done" 的日报
- **THEN** 系统 SHALL NOT 创建重复记录，而是更新已有记录

#### Scenario: 并发落库同板同日不炸唯一约束
- **WHEN** 两条写路径（正常情况下由生成闸互斥；异常绕过闸时）先后或并发为 SemanticBoard #5 落库 2026-05-25 的日报
- **THEN** 系统 SHALL 通过原子 upsert 收敛为一条记录（后写覆盖先写），SHALL NOT 出现 duplicate key 唯一约束错误，SHALL NOT 产生部分写入的残缺日报（sections 与主记录同事务落库）

#### Scenario: 日报关联昨日报告
- **WHEN** SemanticBoard #5 在 2026-05-24 有一条已完成日报 (id=42)
- **THEN** 2026-05-25 的日报记录 SHALL 设置 prev_report_id=42

#### Scenario: 线程存储在独立表中
- **WHEN** 日报生成完成
- **THEN** 每个聚类的叙事事件 SHALL 作为独立行存储在 `daily_report_threads` 表中，通过 `section_id` 关联到对应的 section

#### Scenario: 兄弟桶共享聚类序号

- **GIVEN** 某聚类被泳道内主题分桶拆成 3 个桶
- **WHEN** 日报落库
- **THEN** 3 条 section SHALL 共享同一 `cluster_index`，`bucket_index` 分别为 0/1/2，`bucket_title` 各自为主题方向名

#### Scenario: 未分桶 section 取默认桶序号

- **GIVEN** 某聚类未达到分桶阈值（事件 < 6 且 tags < 10）
- **WHEN** 日报落库
- **THEN** 该 section SHALL 为 `bucket_index=0`、`bucket_title=NULL`，且其 `cluster_index` 在该 report 内仍然唯一

#### Scenario: highlights 落库恒为数组
- **WHEN** 日报生成时未产出任何 highlight（合法空降级或生成失败降级）
- **THEN** `board_daily_reports.highlights` SHALL 落 JSON 空数组 `[]`，SHALL NOT 落 JSON `null` 标量

#### Scenario: 历史 null highlights 读出归一为数组
- **WHEN** 读取一篇 `highlights` 列为 JSON `null` 的历史日报（2026-11-03 前的存量坏数据）
- **THEN** API 返回的 `highlights` SHALL 为 `[]`，SHALL NOT 为 `null`

#### Scenario: 事件落库时 scope 由事件标签数推导
- **WHEN** 某事件的 `tag_ids` 含 3 个 `category='event'` 标签
- **THEN** 该事件落库时 `scope` SHALL 为 `composite`，`story_id` SHALL 保持 NULL

#### Scenario: 新日报不写事件身份
- **WHEN** 生成或重建任意一期的日报
- **THEN** 该期所有 thread 的 `story_id` SHALL 为 NULL，`story_manual` SHALL 为 false，`display_tier` SHALL 为 `body`
- **AND** 系统 SHALL NOT 新建、合并或改写任何 `board_stories` / `story_signals` 行

#### Scenario: 旧事件身份只读保留
- **WHEN** 读取一篇历史日报，其 thread 带非空 `story_id` 且指向存活的 `board_stories` 行
- **THEN** 该历史值 SHALL 原样保留在库中，SHALL NOT 被清空或改写
- **AND** 对外响应 SHALL NOT 透出事件线身份字段

#### Scenario: 事件未归组不影响日报
- **WHEN** 某事件的 `story_id` 为 NULL
- **THEN** 该事件 SHALL 照常在日报正文展示，SHALL NOT 被过滤或隐藏

### Requirement: 重点依据与正文定位

前端 SHALL 在今日重点旁显示服务端快照「N 篇来源」（0 是合法值，按内容身份去重后的簇内文章数），可读提示附数据截至日、研究时点与聚簇口径标记。字段缺失 SHALL 省略数字而非伪造 0。带有效 `cluster_id` 且有正文目标的重点 SHALL 可键盘激活，定位本 report 当前正文 DOM 顺序中该聚簇第一条 thread；必要时展开目标所在正文容器，滚动、聚焦并高亮 2s。锚点 SHALL 为 `report-thread-{thread.id}`，聚簇只读解析。

带簇内标签的重点 SHALL 可展开（默认收起）列出本期正文中该簇的全部线索：前端用 `tag_ids`（缺失时回退 `cluster_id`）与本 report 正文 thread 的 `tag_ids` 求交集现算，按正文 DOM 顺序列出，每行标注所属话题名；命中代表标签（`cluster_id`）的线索 SHALL 排最前并显式标注「代表线索」。行点击 SHALL 精确定位该条 thread（必要时展开所在正文容器，滚动/聚焦/高亮 2s，语义同重点定位）。展开 SHALL 纯前端现算，SHALL NOT 依赖后端新增字段或快照补齐（历史 highlights 同样可展开）；交集为空 SHALL 显示「本期正文未找到对应线索」，SHALL NOT 编造线索或跳到近似位置。重点条目主体点击与 `cluster` 深链的定位语义不变（仍为正文 DOM 顺序首条）。

无 `cluster_id` 的重点 SHALL 仅展示文案、不猜标签归属。无目标 SHALL 禁用定位并提示「本期正文暂无对应内容」；点击时目标消失 SHALL 原地轻提示，SHALL NOT 打开新详情页、近似跳 section 或自动重建。重复激活 SHALL 替换单个高亮与计时器，切 report/卸载清理；reduced-motion SHALL 不平滑滚动，划选 SHALL NOT 触发定位。

跨页寻址 SHALL 通过日报阅读台路由的 `cluster` 查询参数（`/tags/:boardId/:date?cluster=<cluster_id>`）：参数 SHALL 在该期日报详情就绪后，按聚簇身份解析并定位本 report 当前正文 DOM 顺序中该聚簇第一条 thread，展开/滚动/聚焦/高亮 2s，与刊头重点定位同一路径。参数所指聚簇在本期正文无落点时 SHALL NOT 滚动、SHALL NOT 跳到近似位置，SHALL 提示「本期正文暂无对应内容」。旧的 `story` 查询参数 SHALL 继续被解析但 SHALL 被忽略，并提示该定位方式已下线，SHALL NOT 报错、SHALL NOT 定位到任何内容。翻页或切换 report 后 SHALL 清理该参数，SHALL NOT 在其它报告重复定位；无效或非法的参数值 SHALL 忽略且 SHALL NOT 报错阻断阅读。

#### Scenario: 点击第一条可见正文线程
- **WHEN** 同一聚簇在本期两个正文位置出现，用户点击或键盘激活重点
- **THEN** SHALL 展开需要展开的正文容器并滚动/聚焦 DOM 中第一条对应 thread，高亮 2s，SHALL NOT 按最小 id 或跳别的 report

#### Scenario: 失去目标与旧重点
- **WHEN** 重点无 `cluster_id` 或有 id 但正文已无对应 thread
- **THEN** 无 id 的重点 SHALL 保持静态文案；无目标的聚簇重点 SHALL 提示本期正文暂无对应内容，SHALL NOT 产生空跳转

#### Scenario: 重复定位与减弱动画
- **WHEN** 连续定位两条重点，随后切换 report，且用户设置 reduced-motion
- **THEN** SHALL 始终最多一个高亮，切换时清理，无平滑动画，划选文案 SHALL NOT 触发定位

#### Scenario: 展示筛选不改变热度
- **WHEN** 某 thread 因其它原因未进入正文展示
- **THEN** 该聚簇的 `article_count` SHALL 不因其被筛掉而改变，SHALL NOT 在定位时把用户带到其它内容

#### Scenario: 深链参数定位与失去目标
- **WHEN** 用户打开 `/tags/1974/2026-11-06?cluster=7`，该期正文含该聚簇的 thread
- **THEN** 阅读台 SHALL 定位该聚簇在正文 DOM 顺序中的第一条 thread 并高亮 2s
- **AND** 当该期正文已无该聚簇的 thread 时 SHALL 提示「本期正文暂无对应内容」且不滚动，SHALL NOT 跳到近似位置

#### Scenario: 旧 story 深链被忽略不报错
- **WHEN** 用户打开携带 `?story=42` 的历史链接
- **THEN** 页面 SHALL 正常打开并提示该定位方式已下线
- **AND** SHALL NOT 定位到任何 thread，SHALL NOT 报错

## ADDED Requirements

### Requirement: 当天同事件聚簇先筛重点候选

系统 SHALL 在本 report 当天同事件聚簇中去重筛选「今日重点」候选。候选 SHALL 为该版块当天日报命中的 event 标签聚成的簇（口径与降级链见 `related-events`「日报同事件聚簇」），每个候选 SHALL 附代表标签、label/description、簇内文章数（按内容身份去重）与聚簇口径标记；排序 SHALL 按簇内文章数降序 → 簇内 event 标签数降序 → 聚簇代表标签 id 升序，最多 8 条，不按四舍五入后的篇数排序、不用版块 section 数或泳道热度替代。

LLM SHALL 只从这 8 条以内候选中选至多 3 条不同聚簇写 title/reason，SHALL NOT 编造事实、数字、情绪或因果。模型返回的 `cluster_id` SHALL 属当次候选白名单，`tag_ids` SHALL 属该聚簇的本期标签白名单；过滤后空 tag、空白标题理由、集外 id 或类型错误 SHALL 按既有语义非法响应同参重试一次；合法 `[]` 不重试，最终失败仍 `[]`。重复聚簇 SHALL 保留首条。

文章数、聚簇口径、数据截至日与研究时点 SHALL 由服务端补齐快照，SHALL NOT 由模型输出。候选不足允许 1 条，零候选 SHALL 在调用前选择标签回退模式（`cluster_id` 为空、缺省快照字段），SHALL NOT 伪造身份或编造聚簇。旧报告 SHALL NOT 回填补造聚簇身份或篇数快照。

#### Scenario: 候选按簇内文章数截断
- **WHEN** 本期有 10 个候选聚簇，簇内文章数排序明确，第 9 名拥有最多 section
- **THEN** LLM SHALL 只收到排序前 8 个候选的事实与篇数，第 9 名 SHALL NOT 因 section 数进入候选

#### Scenario: 同分与精度不反转
- **WHEN** 两个候选簇内文章数相同，或四舍五入后相同但原值不同
- **THEN** SHALL 依次按簇内 event 标签数降序、代表标签 id 升序破同分，未舍入篇数更高者仍排前

#### Scenario: 只可从候选事实写文案
- **WHEN** 模型返回集外 `cluster_id` 或仅含别的聚簇 tag_ids 的重点
- **THEN** SHALL 判非法并按既有上限重试，SHALL NOT 将该条作为合法重点落库

#### Scenario: 无聚簇走标签回退
- **WHEN** 当天全部为综合稿、无候选 event 标签，或全部候选无正文落点
- **THEN** SHALL 选择现有标签聚类重点路径，`cluster_id` 为空，正文与统计字段 SHALL NOT 为了凑候选被修改

#### Scenario: 少量候选与合法空返回
- **WHEN** 只有一条候选或候选事实不足导致模型返回 `[]`
- **THEN** SHALL 分别允许一条重点或合法空数组，SHALL NOT 为凑 2 条编造内容或重试合法 `[]`

#### Scenario: 重点快照与旧数据兼容
- **WHEN** 读取新聚簇重点、旧 tag_ids-only 重点或 JSON null
- **THEN** 新重点 SHALL 返回生成时来源、篇数、聚簇口径与时点快照，旧重点 SHALL 不伪造快照，JSON null SHALL 归一为 `[]`，首条重点仍为报告 title/summary 来源，无重点仍用既有兜底

## REMOVED Requirements

### Requirement: 跨期重复降级

**Reason**: 该 Requirement 的唯一判据是「同一事件线最近 7 天的 body 事件文章并集与当前事件的重叠率」，依赖事件线身份做跨期去重。事件线在线生产已退役（归属判定停写），`story_id` 退为只读历史列，新日报恒为 NULL，去重判据失去唯一输入；用户口径明确「不跨天」。
**Migration**: 跨期去重整档退役，不再产生 `flash` / `hidden` 的 `display_tier` 降级。历史日报已落库的 `display_tier` 值原样保留、只读；新生成与重建的日报 `display_tier` 恒为 `body`。跨期重复的读者侧诉求由「当天同事件聚簇」承担（同一天的同事件聚合），跨天重复本期不做。

### Requirement: 事件线热度先筛重点候选

**Reason**: 该 Requirement 以「本 report 已归属且有 body thread 的存活事件线」为候选单位、以 `heat-v1-48h-hl24h` 的 story 热度排序，属事件线热度的消费契约。事件线退役后无新归属、无 story 热度证据，判据失去输入。
**Migration**: 候选单位改为「当天同事件聚簇」，排序改为簇内文章数（内容身份去重）降序，事实白名单随之改为聚簇内的本期标签；候选上限仍为 8、重点仍为至多 3 条、非法响应同参重试一次、合法空不重试、降级不阻断日报等既有语义逐字保留（见本 change 的 ADDED Requirement「当天同事件聚簇先筛重点候选」）。

### Requirement: 昨日重点延续徽标

**Reason**: 该 Requirement 以「今天与昨天非空 `story_id` 经同版块一跳解析后比较」派生 `is_progress` 徽标，依赖事件线身份且天然跨天。事件线退役后 `story_id` 恒为 NULL，聚簇口径又不跨天，判据失去输入。
**Migration**: 昨日重点延续徽标整档退役，日报重点不再展示「进展」徽标，也不由模型或服务端另行推断跨天延续。跨天延续的读者侧需求本期不承接；`prev_report_id` 等既有日报链路字段不受影响。
