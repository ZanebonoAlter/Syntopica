# Test Cases — fix-board-match-cross-domain

复杂度：simple（行为变更，含匹配算法逻辑，按 test-design ⑤a 关键词命中建账）。

## ⓪ 继承与调整（契约 M/R 反查）

| 旧 Scenario（delta 触及） | 处置 | 旧测试 | 动作 |
| --- | --- | --- | --- |
| 直接命中 board 构成标签 | MODIFIED（追加占比 ≥0.5 前提） | `TestSemanticBoardMatchingDirectHit` | 检查构造数据的占比：tag 辅助标签全集 vs 交集数；若 <0.5 补足辅助标签数使占比过半，保持原断言意图（仍断言 direct_hit 挂载） |
| 降级 direct_hit 方向校验不通过 | MODIFIED（原文示例 {国债,收益率} 占比 2/2=1.0） | `TestSemanticBoardMatchingDirectHitDirectionMismatch` | 同上检查占比；断言 direction_mismatch=true 不变 |
| direct_hit_min_overlap=1 向后兼容 | MODIFIED（追加占比条件） | 无直接旧测试 | 新增用例覆盖（交集 1 + 占比达标 → 命中） |
| 匹配参数用户可调 | MODIFIED（新增 min_ratio） | `TestMatchingConfig*`（handler） | 扩展断言：GET 响应含 `semantic_board_match_direct_hit_min_ratio` |
| 日报质量筛选（过滤 weighted/保底/截断） | MODIFIED（排他步骤插在最前） | `TestCollectBoardTags_PGHonorsDowngradedColumn` / `TestCollectBoardTags_PopulatesArticleContext` | 旧测试数据无跨版块强归属挂载 → 排他不触发 → 继续绿（回归确认，不改） |

## 主链路（故事：蹭词跨域新闻不再污染版块日报）

| 步 | 动作 | 来源 Scenario | 期望 | 层 / 落点 |
| --- | --- | --- | --- | --- |
| 1 | tag 5 个辅助标签、交集 2（占比 0.4），对某版块执行匹配 | 交集数达标但占比不足退回相似度 | SHALL NOT direct_hit；相似度不达阈值时不挂载该板 | service 单测（PG fixture）/ `semantic_board_matching_test.go` 新增 |
| 2 | tag 4 个辅助标签、交集 2（占比 0.5），方向校验过 | 直接命中 board 构成标签 | SHALL direct_hit 挂载 score=0.7，direction_mismatch=false | service 单测（PG fixture）/ 既有测试调整后复跑 |
| 3 | 配置 `direct_hit_min_ratio=0`，重复步 1 数据 | 用户调整 direct_hit 占比门槛 | 占比门槛关闭，步 1 数据回到旧行为 direct_hit 挂载 | service 单测（PG fixture + upsertMatchSetting） |
| 4 | GET/PUT matching-config 含新 key | 用户调整 direct_hit 占比门槛 | 新 key 随 14 个既有 key 读写生效 | handler 单测 / `board_match_handler_test.go` 扩展 |
| 5 | tag 挂 A 板 1.0、B 板 0.7（分差 0.3），生成 B 板日报收集 | 跨版块排他剔除弱归属标签 | 步骤 1 排他剔除：该 tag 不进 B 板候选 | PG testcontainer / `collect_board_tags_test.go` 新增 |
| 6 | B 板当日 12 标签，3 个被排他、5 个 weighted，剩 4 (<10) | 排他剔除不参与保底回捞 | weighted 回捞至 9 个；被排他 3 个 SHALL NOT 回来 | PG testcontainer / `collect_board_tags_test.go` 新增 |
| 7 | tag 挂 A 板 0.8、B 板 0.7（分差 0.1） | 排他不影响分差不足的双挂载 | 两板候选均含该 tag | PG testcontainer / 同上 |
| 8 | 无挂载 tag 走 fallback，匹配结果 [A 板 1.0, B 板 0.7]，生成 B 板日报 | fallback 路径同样应用排他 | fallback SHALL NOT 以 0.7 收进 B 板 | service 单测（fallback 择板逻辑，可用 matcher stub） |
| 9 | 真库 SQL 模拟排他对现有数据的影响 | （效果核对，非 spec 场景） | tag 196587（非农）在 gap=0.3 下不再进板 2272 候选；量化各板被剔 tag 数 | 真库只读 SQL / tasks 3.2 |

## 变体走查

- **输入**：tag 无辅助标签（交集 0，direct_hit 不可能命中，行为不变 ✓ 覆盖于白盒分支表）；交集 1 且 min_overlap=2（不命中，与占比无关 ✓）；辅助标签含 disabled 标签（加载时已过滤，分母只算 active ✓ 既有行为）
- **前置**：tag 无其他版块挂载（NOT EXISTS 为真，排他不触发 ✓）；两板并列同分 0.7（分差 0 < 0.3，保留 ✓）；配置值非法（"abc"/负数 → 回退默认 0.5 / 0.3 ✓）；gap=0（排他整体关闭 ✓ 步 3 同构）
- **时间窗口**：不适用（占比与排他均不涉时间语义）——划除
- **幂等**：日报重复生成（排他为纯查询过滤，无副作用，幂等 ✓）；rematch-all 重复执行（占比判定确定性 ✓）
- **可用性**：ui-impact none，无 UI 可用性用例——划除

## 效果核对（真库）

- 触发原因：修复 10.4 类跨域污染；效果依赖存量挂载数据分布，需真库量化。
- 方法：只读 SQL 模拟 NOT EXISTS 排他（gap=0.3）+ 占比门槛建模，对 `topic_tag_board_labels` 全量与 10.4 板 2272 数据集各算一遍。
- 量化结论（探索阶段已得）：占比 <0.5 的弱 direct_hit 挂载 160 个（91 有强归属将被排他挡住、69 唯一挂载由占比门槛在源头拦新）；模拟 196587 在 2272 的 10.4 收集中被剔除。上线后核对实际日报生成结果与模拟一致。
- 复核命令：`docker exec <pg> psql -U postgres -d syntopica -c "<tasks 3.2 的模拟 SQL>"`（只读，无副作用）。

## 白盒附加

分支表（`MatchTopicTag` Priority 2 与 `collectBoardTags` 排他）：

| # | 条件组合 | 分支 | 用例 |
| --- | --- | --- | --- |
| B1 | overlap≥2 ∧ ratio≥0.5 | direct_hit 挂载 | 步 2 |
| B2 | overlap≥2 ∧ ratio<0.5 | 落回 Priority 3 | 步 1 |
| B3 | overlap<2（ratio 任意） | 不 direct_hit（与占比无关） | 白盒补一条（交集 1） |
| B4 | ratio 配置=0 | B2 退化为 B1（门槛关闭） | 步 3 |
| B5 | 收集时存在他板挂载 score≥本板+0.3 | NOT EXISTS 排他剔除 | 步 5 |
| B6 | 他板挂载分差 <0.3 / 无他板挂载 | 保留 | 步 7 |
| B7 | fallback matches[0] 与本板分差 ≥gap | fallback 不收 | 步 8 |
| B8 | gap≤0 | 排他子查询跳过 | 白盒补一条（gap=0 全收） |

边界值：ratio=0.5 恰好达标（≥ 含等号）、分差=0.3 恰好排他（≥ 含等号）、score 并列（不排他）。
