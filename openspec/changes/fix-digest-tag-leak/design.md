## Context

见 proposal.md（Why）：digest 文章实体污染 + 两个环境缺陷（绝对阈值无区分度、去重 tiebreak 内容盲）导致跨域标签串味进版块日报。本设计只覆盖已拍板的两刀：方向校验相对门、去重 tiebreak 版块相关性。

现状约束：
- 方向校验位于 `tagmanagement/service/board/semantic_board_matching.go` `evaluateSemanticBoardMatches`，逐版块独立标记 `direction_mismatch`；收集端（topicgraph `collectBoardTags`）过滤 `direction_mismatch=true`。
- 去重位于 `topicgraph/service/daily_report_dedup.go` `DeduplicateTags`/`shouldReplace`，纯内存操作（红线：不改 `topic_tags` 原始数据），tiebreak 现为「文章数 > ID 小者」。
- 红线（flow/semantic-board）：composite_hit score 恒 1.0 且 `direction_mismatch=false`（组合命中即指向一致）；direct_hit 降级后**与间接规则一样强制方向校验**。
- spec 漂移：`board-direction-check` spec 写「direct_hit 跳过方向校验」，但红线与现行代码（direct_hit 分支带 `DirectionMismatch: directionMismatch(boardID)`）均为做校验——delta spec 顺手修正。
- margin 数据定值调研已存档本档 pin finding（真库 2596 挂载行全量）：gap≥0.15 抽样 18/18 串味、0.10~0.15 出现合法双挂、多挂影响面 ~7%。

## Goals / Non-Goals

**Goals:**
- 相对门拦截「绝对阈值漏掉的多挂串味」（本 change 触发案例：gap 0.209/0.250 类挂载）
- 去重二选一时保留与当前版块语义更近的标签，杀死明显串味方（OpenAI vs 柴油案例）
- 两处改动均有明确回退路径（margin=0 关门 / embedding 缺失回退 ID 规则）

**Non-Goals:**
- 不改事件标签抽取链路（辅助标签句级作用域留观察）
- 不改 L3 聚类行为（LLM 拆组留观察）
- 不回填历史 `direction_mismatch` 标记（新门只影响后续匹配轮次，存量等自然 rematch 或手动 rematch-all）
- 不解决单挂标签的串味（相对门对单挂 gap≡0 恒过，绝对阈值继续兜底）

## Decisions

### D1 方向校验相对门：绝对 OR 相对，双条件任一触发即标记

`evaluateSemanticBoardMatches` 在现有绝对阈值外增加相对条件：挂载点标记 `direction_mismatch=true` 当且仅当

```
cos(tag_identity, board) < DirectionSimThreshold          # 现有绝对门（0.5）
OR
gap = bestCos - cos(tag_identity, board) >= margin        # 新相对门（默认 0.15）
```

- **参照集（用户拍板）**：该标签本次匹配上的全部版块（`matchReason != ""` 的候选，含同轮将被标 mismatch 的），`bestCos` = 参照集内 identity cosine 最大者。单挂标签 gap≡0 恒过——相对门天然只作用于多挂标签。
- **margin=0.15 数据定值**：≥0.15 抽样全串味（含本案例 OpenAI→中东 0.250、→俄乌 0.209）；0.10~0.15 出现合法双挂（乌美会谈→美国新闻 0.149、国内大模型厂商→全球科技巨头 0.148）。0.15 卡在干净/混杂分界。
- **配置**：`ai_settings` key `semantic_board_match_direction_relative_margin`，默认 0.15，设 0 关闭相对门（回退纯绝对阈值）。与现有 15 个匹配参数同机制管理（LoadConfig）。
- **豁免与纳入**：composite_hit 豁免（红线：mismatch 恒 false）；direct_hit 纳入（红线：降级后与间接规则同样强制校验）——delta spec 同步修正旧 spec 的「direct_hit 跳过」漂移。
- 备选「提高绝对阈值到 0.6」被否：各档绝对水位重叠（direct_hit p50=0.678 vs weighted p50=0.634），一刀切会大量误杀 weighted 真挂载。

### D2 去重 tiebreak：文章数 > 版块相关性 > ID

`shouldReplace` 扩展为三级比较：

```
1. article_count 多者胜（不变）
2. 平手 → cos(tag_identity, 当前版块 embedding) 高者胜（新增）
3. 仍平手（cosine 相等或 embedding 缺失）→ ID 小者胜（不变，兜底）
```

- **接线**：orchestrator 在 collect 后、`DeduplicateTags` 前批量加载参战标签的 identity embedding（`topic_tag_embeddings` WHERE embedding_type='identity'，topicgraph repository 直查本表，不跨包调 tagmanagement）与当前版块 embedding（`semantic_labels` WHERE label_type='board' 单行）。`DeduplicateTags`/`shouldReplace` 签名增加相关性上下文参数。
- **回退即安全网**：任一 embedding 缺失 → 该对决退回 ID 比较（现行为），无需独立配置开关；红线约束保持——去重仅内存，不改 `topic_tags`。
- 备选「按 match_reason 档位优先」被否：digest 案例两标签同为 weighted 0.680，档位信息量为零，identity 向量是唯一有区分度的信号（案例 204331 柴油 vs 俄乌版块 ≫ 204328 OpenAI vs 俄乌版块）。

### D3 相对门与 tiebreak 共用 identity embedding，加载一次

两处都需要「标签 identity 向量」（方向校验端在 tagmanagement 匹配轮已有；日报去重端在 topicgraph 新增查询）。各自按需加载、不共享缓存：匹配轮与日报轮生命周期不同步，强行共享引入跨包耦合，且全量 identity embedding 仅千级行、查询成本可忽略。

## Risks / Trade-offs

- [相对门误杀未来合法双挂] → margin 可配：分布漂移时调大或设 0；观察指标=rematch 后 mismatch 增量（预期 ~7% 多挂挂载）。
- [identity embedding 依赖事件 label 纯度] → identity 文本=label+aliases+category，digest 污染的是辅助标签而非 label，identity 相对干净（案例区分度 0.81/0.60）；若未来 label 抽取被污染则两刀同时失效，属上游 Non-Goal 范畴。
- [单挂串味标签不受相对门保护] → 已知限制（Non-Goals），绝对阈值兜底；改进留给句级辅助标签方案。
- [direct_hit 多挂中 gap≥0.15 仅 2.7%（5/183）] → 抽样属串味，拦截无副作用；但 direct_hit 纳入是红线要求，非本设计裁量。
- [去重 tiebreak 无开关] → embedding 缺失自动回退现行为，逻辑改动局部，git revert 兜底；避免配置项膨胀。

## Migration Plan

1. 部署：无 DB 迁移、无前端改动；`ai_settings` 新 key 缺省时用默认 0.15，无需预写配置。
2. 部署后行为变化：后续匹配轮次（自动或手动触发 rematch）开始按双条件标记 mismatch；下一轮各版块日报生成时去重保留更贴版块的标签。
3. 存量错误挂载（如 OpenAI→俄乌）不会自动消失：可选手动触发 rematch-all 清理（`POST` board rematch 端点），预期多挂挂载 mismatch 增量 ~7%。
4. 回滚：margin 设 0 关相对门；tiebreak 靠 embedding 缺失回退或 revert。
