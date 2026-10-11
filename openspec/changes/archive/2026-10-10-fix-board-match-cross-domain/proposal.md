<!-- complexity: simple -->
<!-- ui-impact: none -->
<!-- constraint-domains: semantic-board, daily-report -->

## Why

2026-10-04「中国国内新闻」日报被美国非农就业新闻占据（标题即《美国9月非农数据意外爆冷》）。根因：文章《美国9月非农意外"爆冷"，节后A股会迎来"开门红"吗？》的 event tag 携带 {美联储, 港股, A股, 美元指数, 非农就业数据} 五个辅助标签，其中 A股/港股 恰好都在「中国国内新闻」版块构成里，重叠数 2 达到 `direct_hit_min_overlap=2` 即直接挂载（score 0.7），跳过了任何相似度计算；同时该 tag 在「美国新闻」版块有 max_sim 满分挂载，但日报收集不比较跨版块归属，弱归属照样进日报。全链路调查存档于 `docs/research/board-matching-misfire/explore-findings.md`。数据显示占比 <0.5 的蹭词挂载共 160 个，其中 69 个是唯一挂载（无更强归属可比，排他救不了），必须源头拦截；占比约束正好卡在 0.40（153 个，蹭词模式）与 0.50（67 个，真跨界为主）的天然断点上。

## What Changes

- **direct_hit 增加占比门槛**：Priority 2 直接命中判定从「重叠数 ≥ min_overlap」收紧为「重叠数 ≥ min_overlap **且** 重叠数 / tag 活跃辅助标签总数 ≥ 0.5」。占比不足时不再直接挂载，落回 Priority 3 相似度路径自然判定。新配置 key `semantic_board_match_direct_hit_min_ratio`（默认 0.5，纳入 matching-config API 与 `LoadConfig` 开关列表）。
- **日报收集排他**：`collectBoardTags` 主查询增加 NOT EXISTS 子查询——同一 tag 在其他版块存在 `score ≥ 本版块 score + gap` 的挂载时不收；fallback 路径（现场计算匹配）同规则：本版块分数与该 tag 最高分差 ≥ gap 时不收。新配置 key `semantic_board_report_exclusive_gap`（默认 0.3，读取机制同 `LoadWatchMaterializeConfig`）。排他剔除发生在现有质量筛选（direction_mismatch 过滤 / weighted 过滤）之前，且**不参与保底回捞**（「有更强归属」是确定性判断，与 weighted 弱规则不同）。
- **存量不重算**（用户决策）：不跑 rematch-all、不重建已生成日报；已挂载的 160 个弱挂载保留在 `topic_tag_board_labels`，新匹配与新生成的日报按新规则走。存量弱挂载中 91 个有更强归属者由排他规则在日报收集时自然挡住。

## Capabilities

### New Capabilities

（无）

### Modified Capabilities

- `tag-to-board-matching`: direct_hit 判定增加占比条件（重叠数达标但占比不足时退回相似度路径），新增可调参数 `direct_hit_min_ratio`
- `daily-report-system`: 日报质量筛选管线在 direction_mismatch 过滤之前新增跨版块排他步骤（分差 ≥ exclusive_gap 的弱归属标签不进入聚类）

## Impact

- 代码：`backend-go/internal/tagmanagement/service/board/semantic_board_matching.go`（Priority 2 判定 + LoadConfig）、`backend-go/internal/tagmanagement/handler/board_match_handler.go`（matching-config 映射表）、`backend-go/internal/topicgraph/service/daily_report_orchestrator.go`（collectBoardTags 主查询 NOT EXISTS + fallback 排他）
- 配置：ai_settings 新增 2 个 key（均可缺省，默认值生效）
- 数据：无迁移、无回填；`topic_tag_board_labels` 表结构不变
- 用户可见行为：新生成的版块日报不再出现「蹭词挂载的跨域新闻」占据标题/正文（本例 10.4 类事故不再复现）；真跨界新闻（占比 ≥0.5 或两侧分差 <0.3）仍可在多版块出现
