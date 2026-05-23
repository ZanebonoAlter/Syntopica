## Context

当前叙事系统分为 4 层 LLM 调用：抽象树→叙事（Pass 1）、散装 event→叙事（Pass 2）、跨分类叙事、关注标签叙事。叙事直接平铺展示在 Canvas 时间线上，没有中间组织层。当事件数量增多时，平铺的叙事难以浏览，用户无法快速定位感兴趣的领域。

现有约束：
- Per-category 叙事上限 8，跨 category 收集 cap 5，总 cap 30
- 散装 event 标签最多 50 个一次性丢给 LLM，信息过载
- 抽象标签树已有良好的结构和 description，但叙事生成时没有充分利用
- 前端 P5.js Canvas 渲染叙事卡片 + bezier 连线

## Goals / Non-Goals

**Goals:**
- 新增版块（Board）聚合层，提供折叠/钻取浏览体验
- LLM 调用从 4 层简化为 2 层 + fallback
- 抽象标签直接映射为叙事卡，充分利用已有结构
- 版块跨天连接从叙事 parent_ids 推导，版块无需独立生命周期
- 全局维度合并跨 category 的相似版块
- 抽象标签 status 根据子标签文章活动自动计算

**Non-Goals:**
- 版块不需要独立的生命周期字段（emerging/continuing/ending），全部从叙事推导
- 暂不实现版块到话题图谱的跳转
- 暂不保留 GenerateCrossCategoryNarratives 和 GenerateWatchedTagNarratives
- 不做现有叙事数据迁移，已清空重建

## Decisions

### D1: 版块是持久化实体（路线 A）

**选择**：新增 `narrative_boards` 表，叙事通过 `board_id` 外键关联。

**理由**：版块有 id 才能被叙事引用、被 LLM 标注 prev_board_ids、被全局合并时去重。纯派生视图（路线 B）会导致 board_name 冗余且无法跨天精确引用。

**替代方案**：路线 B（在 narrative_summaries 加 board_name 字段）——更轻量但无法支持跨天版块精确匹配和全局合并。

### D2: 版块划分在 category 内部，LLM 同时处理抽象标签归类

**选择**：Pass 0 一次 LLM 调用完成：版块划分 + 抽象标签归入版块 + prev_board_ids 标注。

**理由**：LLM 看到完整信息（散装 event + 抽象标签 + 昨天版块）能做出更好的版块划分。合并调用减少 LLM 开销。

**替代方案**：分两步——先版块划分再抽象标签归类——增加了调用次数但降低了单次复杂度。

### D3: 跨天版块匹配——LLM 在 Pass 0 标注 prev_board_ids（策略 2）

**选择**：Pass 0 的 LLM 看到昨天的版块列表（name + description），输出今天版块时标注 prev_board_ids。

**理由**：版块摘要很短（名字+一句话），prompt 开销可忽略。LLM 理解语义，能处理名称变化。

**替代方案**：embedding 相似度匹配（额外计算开销）、tag 重叠度匹配（新兴版块无 tag 重叠）。

### D4: 跨天版块连线从叙事 parent_ids 推导（方案 B）

**选择**：版块间的延续/分裂/合并关系从其包含的叙事的 parent_ids 推导。

**理由**：叙事已有成熟的 parent_ids 机制，版块不需要独立的生命周期管理。

**替代方案**：方案 A（内容重叠自动连接）——需要额外的重叠计算；方案 C（LLM 单独判断版块关系）——增加调用次数。

### D5: 全局版块合并由 LLM 判断

**选择**：Phase 2 收集所有 category 版块，LLM 判断跨 category 的相似版块并合并。

**理由**：LLM 能理解"地缘政治"出现在政治新闻和经济新闻中是同一主题。embedding 匹配对短文本（版块名+描述）不够可靠。

### D6: 抽象标签 status 根据子标签文章活动计算（方案 A）

**选择**：简单活动窗口——看抽象标签子标签在过去 N 天关联的文章数，> 阈值 continuing，衰减中 ending，新建 emerging。

**理由**：status 是粗粒度标签，不需要精确趋势分析。方案 B（趋势感知）增加复杂度但收益有限。

### D7: 事件数 ≤ 5 时跳过版块直接叙事

**选择**：设定阈值 5，低于此值跳过版块划分，直接 LLM 生成叙事（类似旧 Pass 2）。

**理由**：2-3 个事件不值得建版块，直接生成叙事更高效。

### D8: 关联失败 Fallback——单独 LLM 重试最多 3 次

**选择**：叙事 parent_ids 指向的昨天叙事不在匹配版块内时，单独调用 LLM 带完整上下文重试，最多 3 次。

**理由**：避免因版块匹配遗漏导致叙事断链。

## Risks / Trade-offs

- **[Pass 0 输入过大]** → 当 category 下 event 标签 + 抽象标签很多时，prompt 可能超长。缓解：限制输入数量（event ≤50, abstract tree 最多取前 10 棵），超过时分批处理。
- **[LLM 版块划分不稳定]** → 每天版块名可能不同，影响跨天连续性。缓解：prompt 强调参考昨天版块命名，prev_board_ids 提供锚点。
- **[全局合并延迟]** → 需要等所有 category 生成完毕才能做全局合并。缓解：per-category 生成并行，全局合并是轻量 LLM 调用。
- **[前端 Canvas 复杂度]** → 双层节点（版块+叙事）增加渲染复杂度。缓解：版块折叠态只渲染大节点，展开时才渲染子叙事。
- **[Fallback 成本]** → 极端情况下可能多次额外 LLM 调用。缓解：硬性限制最多 3 次，超限则放弃关联。

## Open Questions

- 抽象标签 status 计算的活动窗口 N 天和阈值具体数值——实现时可调，先暂定 3 天窗口、3 篇文章阈值
- 版块的 embedding 是否需要生成——当前不需要，但未来如果需要向量搜索版块可能要加
- 现有 tag feedback 机制（feedbackNarrativesToTags）在版块模式下是否需要调整——初始版本保持不变
