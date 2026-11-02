# Test Cases: add-ghost-mount-unmount

> 层选择：墓碑过滤纯逻辑 → 函数单测；SQL/卸载事务/撤销 → testcontainers PG repository 测试；API → handler 测试；体检候选 SQL → repo PG 测试；完整交互（清单勾选→批量卸载→恢复）→ agent-browser 端到端（前端主链路一个落点）。

## ⓪ 继承与调整（新增 capability 无旧资产；board-health-checkup 系 ADDED Requirement 不动现有）

`bash scripts/harness/test-assets.sh board-mount-tombstone` → 无历史；`board-health-checkup` 现有 22 Scenario 全保留零改动（新 ADDED Requirement 不触碰现有行为）。继承表：无处置项。

## 1. 主链路故事表（卸载闭环）

| 步 | 动作 | 来源 Scenario | 期望 | 层 | 落点 |
| --- | --- | --- | --- | --- | --- |
| 1 | tag 双挂 A(干净)+B(mismatch)，卸载 B | 批量卸载同事务 | B 挂载行删、墓碑 (T,B) 写入，A 不动 | repo PG | board_match_repo_pg_test.go |
| 2 | 卸载后 rematch-all | 卸载后 rematch 不复活 | B 不回挂；A 正常 | repo PG | 同上 |
| 3 | 候选 4 个含墓碑 1 个，MaxBoards=3 | 墓碑行不占 MaxBoards 名额 | 墓碑先滤、余 3 正常截断 | 函数单测 | semantic_board_matching_unit_test.go |
| 4 | 撤销墓碑 + 重算 | 撤销触发重算 | B 重新参与判定 | handler | board_crud_handler_test.go |
| 5 | 体检轮次跑双挂板 | 双挂 mismatch 进入候选 | unmount_candidates 含 T-B（带 A 名称） | repo PG | boardhealth repo_pg_test.go |
| 6 | 前端勾选 2 条 → 批量卸载 → 恢复 1 条 | （UI 契约） | 行内消失/恢复，无报错 | agent-browser | 人工+浏览器断言（§4 效果核对后） |

## 2. 变体走查（五组）

- **输入**：空 topic_tag_ids（400 或 no-op 成功——按实现定 400）；非法 ID（不存在 tag——挂载行自然不存在→skipped 幂等路径）；重复 ID（去重或幂等 upsert 兜底）——均有答案（handler 用例）。
- **前置**：板无任何 mismatch（候选空数组非 null）；板无挂载（卸载全 skipped）；墓碑已存在再卸载（no-op）——均有答案。
- **时间窗口**：不适用（无时间语义；墓碑 created_at 仅展示），划除。
- **幂等**：卸载重放（幂等重放 Scenario）；撤销重放（404）；rematch 多次结果稳定——均有答案。
- **可用性(UI 必检前三)**：批量卸载空勾选（按钮禁用/提示——复用未收编候选模式对齐）；候选空态文案；卸载失败 error 反馈——复用既有卡片契约，agent-browser 断言状态。

## 3. 效果核对（真库量化）

| 指标 | 现状 | 预期 | 核对 |
| --- | --- | --- | --- |
| 科技巨头(15220) ghost | 177（167 双挂） | 全部卸载后 ≤10 | 卸载后 `SELECT count(*) ... direction_mismatch` 留痕 |
| 体检评分 | 72 | 次轮回升（ghost 占比降） | 次日快照对比 |
| 复活检验 | — | rematch-all 后墓碑行 0 复活 | 卸载后触发 rematch-all 对比挂载数 |
| 美国 31/影视 7 | 50/11 | 同口径可选 | 快照 unmount_candidates 计数 |

## 4. 展示字段盘点

新用户可见字段：卸载建议清单（tag label/真家板名/gap）+ 已卸载列表（tag/板/时间）——来源语义均有本 change spec 锚（board-mount-tombstone + board-health-checkup ADDED Requirement），走 New Capability 非 MODIFIED，无隐式契约。

## 5. 白盒附加（分支表）

| 分支 | 边界值 | 用例 |
| --- | --- | --- |
| filterTombstonedMatches | 墓碑命中全部候选（结果空）| 函数单测 |
| filterTombstonedMatches | 墓碑 0 条（透传，len 不变）| 函数单测 |
| 卸载事务 | 混合 IDs（2 有效+1 无效）| removed=2 skipped=1 | repo PG |
| 墓碑 upsert | ON CONFLICT DO NOTHING（重放零错误）| repo PG |
| 撤销重算失败 | MatchTopicTag err（日志不回滚删墓碑）| handler mock |
| 候选 SQL | 双挂+单挂混布（单挂不入列）| repo PG |
| 候选 LIMIT | N=0/默认 20/超限 | config 单测 |
| composite_hit 候选遇墓碑 | 同样被滤 | 函数单测 |
