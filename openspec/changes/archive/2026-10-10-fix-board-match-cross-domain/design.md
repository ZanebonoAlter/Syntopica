# Design

## Context

见 proposal.md Why。两个拦截点分属两个包：direct_hit 占比门槛在 `tagmanagement/service/board`（匹配层），排他在 `topicgraph/service`（日报收集层）。配置均落 `ai_settings`，可缺省。

## 决策

### D1：占比门槛放在 Priority 2 判定内，且拦下时重叠标签同步从 Priority 3 证据剔除

`semantic_board_matching.go` 的 direct_hit 判定处追加占比条件（AND `ratio >= DirectHitMinRatio`），ratio = 交集数 / len(tagAuxiliaryIDs)。占比不足时不 append match、不 continue，自然落入下方 Priority 3 相似度计算（与「交集数不足退回相似度」同一控制流）。

**证据不得换赛道（TDD 实现期发现，2026-10-07）**：占比拦下后若重叠标签照常参与 Priority 3，其与自身的余弦恒为 1.0，max_sim 路径（阈值 0.8、hits≥2、hit_rate≥0.3）会被自相似满分击穿——蹲词 tag 以 1.0 分重新挂载，分数反而高于 direct_hit 的 0.7，且与真归属版块的 1.0 并列导致排他（分差 0）也失效。修复：占比门槛拦截时记录重叠标签 ID 集，Priority 3 的 tag 侧向量剔除这些标签（`tagVectorsExcluding`）；hit_rate 分母保持 tag 全部辅助标签数（证据池不变，仅禁止复用）；剔除后向量为空 → 该版块无相似度证据 → 不挂载。剩余无重叠标签的真实语义相似（hit_rate/max_sim/weighted）仍可正常挂载——拦的是证据复用，不是真跨界。

- 分母用 `len(tagAuxiliaryIDs)`（active 辅助标签全集，加载时已过滤）；tagAuxiliaryIDs 为空时 direct_hit 原本就不可能命中（交集 0 < 2），`directHitRatioSatisfied` 对 total<=0 返回 false 防御除零。
- `DirectHitMinRatio=0` 时条件恒真（ratio ≥ 0），即门槛关闭、回到旧行为——与 spec 声明一致。

### D2：排他做在 collectBoardTags 查询层 + fallback 双落点，不做在 filterTagsByQuality

`filterTagsByQuality` 的输入 `TagInput` 只携带本版块的 match_reason/score，没有跨版块信息；把跨版块分数塞进去需要改 TagInput 结构，扩散面大。而排他本质是「本挂载 vs 该 tag 的其他挂载」比较，`topic_tag_board_labels` 单表即可判定，SQL 侧一条 NOT EXISTS 子查询最便宜：

```sql
NOT EXISTS (
  SELECT 1 FROM topic_tag_board_labels stronger
  WHERE stronger.topic_tag_id = topic_tag_board_labels.topic_tag_id
    AND stronger.semantic_board_id <> topic_tag_board_labels.semantic_board_id
    AND stronger.score >= topic_tag_board_labels.score + :gap
)
```

被排他剔除的标签根本不进 `tags` 切片，天然不参与后续 weighted 保底回捞与 top-30 截断——与 spec「排他剔除不回捞」一致，无需在 filterTagsByQuality 里加逻辑。

**实现期踩坑（2026-10-07，已记入代码注释）**：gap 关闭开关起初写成 `? <= 0 OR NOT EXISTS(...)` 的参数化 SQL 短路——pgx 对「参数作比较左操作数」的绑定求值异常（实验实测 `0.3 <= 0` 求值为 true），EXPLAIN 计划中条件被静默消除。修复为 Go 侧分支（gap <= 0 不拼子查询），SQL 内参数只出现在右侧加法位置。

fallback 路径（无挂载 tag 现场调 `MatchTopicTag`）：matches 数组本身按 score 降序含全部版块结果，择板循环处加一条——若 `matches[0].Score - boardMatch.Score >= gap` 则 continue 不收。

### D3：配置读取机制沿用 LoadWatchMaterializeConfig 模式

- `semantic_board_match_direct_hit_min_ratio`（默认 0.5）：并入 `SemanticBoardMatchingService.LoadConfig` 的 key 开关表 + `board_match_handler.go` matching-config 端点映射（GET/PUT 随 14 个既有 key 一起管理，`InvalidateMatchingConfigCache` 缓存失效自动覆盖）。
- `semantic_board_report_exclusive_gap`（默认 0.3）：topicgraph 侧新加 `LoadDailyReportCollectConfig()`（模式同 `LoadWatchMaterializeConfig`：ai_settings 单 key 读取，缺省/非法回退默认值），`collectBoardTags` 每次生成时读取（日报生成为低频任务，无需缓存）。gap=0 时 NOT EXISTS 的 `score >= score + 0` 会对并列最高分误杀，故 gap ≤ 0 时跳过子查询整体关闭排他。

### D4：存量不重算

不跑 rematch-all、不重建已生成日报（proposal 已声明，用户决策）。已存在的 160 个弱 direct_hit 挂载中 91 个有更强归属，将被 D2 排他在下次日报生成时自然挡住；69 个唯一挂载的误挂由存量数据治理另行处理（本 change 不做）。

## Non-Goals

- 不改 `filterTagsByQuality` 的 weighted 保底/截断规则本身
- 不动 direction_mismatch 的打标与过滤逻辑（现有行为保留）
- 不做版块构成的跨界标签治理（运营动作，后续单议）
- 不提供存量重算工具（`rematch-all` / backfill 端点已存在，用户可自行触发）
- 不做前端改动（matching-config 新 key 由现有 dialog 动态渲染）

## 风险

- **占比门槛误伤**：真跨界 tag（如中美贸易类，占比恰在 0.4~0.5）退回相似度路径后可能两边都挂不上。数据面：占比 0.5 的 67 个 2/4 挂载按 ≥0.5 保留；受影响的仅 <0.5 的 160 个。观察窗内可通过 `direct_hit_min_ratio` 下调补偿。
- **排他过度**：分差恰 ≥0.3 的真跨界弱侧被剔。`exclusive_gap` 上调可放宽；gap=0 可整体关闭。
- **fallback 与主查询口径不一致**：两处排他用同一配置与同一比较方向（严格 `>=`），测试覆盖双路径。
