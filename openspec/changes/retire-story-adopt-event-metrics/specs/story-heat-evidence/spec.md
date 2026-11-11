## REMOVED Requirements

### Requirement: 热度证据数据模型

**Reason**: `story_signals` 热度证据表以「一个参与者在某个信源时间讨论过某条事件线」为行，事件线退役后不再有新的事件线，也就不会再产生新的证据行；`observed_at` 取信源时间、半衰期衰减、`heat-v1-48h-hl24h` 版本号这一整套都是围绕事件线时间轴设计的。
**Migration**: 表与列保留且只读，不再有写入方（`RecomputeSignalsForStory` 随停写空转）。新的「当天热度」由 `lane-heat-metric` 定义：输入是泳道当天的线索数，不是证据行。历史 `story_signals` 数据不迁移、不清理。

### Requirement: 参与者身份与去重

**Reason**: 这条 Requirement 的对象是事件线热度里的「独立参与方」计数（`participant_key`、feed 粒度、重复报道不叠加热度）。事件线热度退役后事件线级参与者计数没有消费方。
**Migration**: **去重口径本身不退役，被新能力继承**：内容身份优先、feed 兜底的去重键 `COALESCE(NULLIF(content_key,''), participant_key)` 改由 `lane-heat-metric` 的「线索计数的内容身份去重口径」承载，被当天同事件聚簇的簇内文章数复用（同一篇内容被多个 feed 抓取只算 1 篇）。`story_signals.content_key` 列保留只读，不回填。

### Requirement: 热度公式与窗口

**Reason**: 事件线热度（24h 半衰期 / 48h 窗口）与话题级热度（72h 半衰期 / 14 天窗口）是 story 层的核心公式，其消费方（矩阵格、雷达关注度、升温徽标、今日重点候选排序）全部随 story 退役而换口径。半衰期表达的是「事件在发酵」，与本 change「只表达当天热度」的方向相反。
**Migration**: 新热度公式见 `lane-heat-metric`「泳道当天热度口径」：活跃泳道当天线索数 × 1.0 + 候选泳道当天线索数 × 可调权重（默认 0.5），无半衰期、无跨天累积、无证据行输入。口径版本号随新公式另立（不复用 `heat-v1-48h-hl24h`）。

### Requirement: 热度不从日报派生

**Reason**: 该 Requirement 明确禁止热度从日报派生（日报是消费者不是生产者），是事件线热度的隔离条款。新的当天热度恰恰从日报几何（section → thread）派生——两者直接冲突，继续保留会让规范自相矛盾。
**Migration**: 该隔离条款随事件线热度一并退役。新的隔离边界由 `lane-heat-metric` 的口径重新划定：热度 SHALL NOT 把事件线数、事件数、section 数、主题桶数、参与者数或任何 LLM 输出作为输入，且 SHALL 为确定性函数、任意历史日可回算（热度不落历史表）。

### Requirement: 可重算与落后源处理

**Reason**: 「同一时刻两次计算结果一致」的可重算性仍然成立，但该 Requirement 的另一半——按 feed 采集落后（`max(3 × refresh_interval, 90 分钟)`）标记小时不完整并在恢复后重算——依赖 `story_signals` 的证据时间轴。事件线退役后没有证据行，落后源标记在新的当天热度上无对应物（当天线索数来自已落库的日报几何，完整性由日报本身保证）。
**Migration**: 可重算性由 `lane-heat-metric` 承接（确定性函数 + 任意历史日可回算）。落后源标记整档退役：新的热度不再输出 `heat_incomplete` 一类的不完整标记，界面也不再有「热度不完整」提示。

### Requirement: 热度消费口径一致

**Reason**: 该 Requirement 逐条规定「矩阵日格 = 事件线数」「矩阵格颜色 = 最热事件线参与者数」「雷达关注度由话题热度派生」「今天可能升温基于事件线热度环比」——四条全部以事件线为输入，事件线退役后四条同时失效。
**Migration**: 四条逐条改写并落到新口径上：矩阵日格改「当日线索合计」、矩阵格**取消颜色通道**（数值 + title/aria 承载全部信息）、雷达关注度改「各泳道当天热度之和」、**「今天可能升温」整档退场**（右栏改为「当天线索最多的泳道」清单）。消费口径仍 SHALL 来自同一套热度值、SHALL NOT 各自用条数近似——这条原则保留，落点在 `workshop-board-console` 与 `workshop-radar` 的 delta。
