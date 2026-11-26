# test-cases：anchor-topic-centroid-with-embedding

测试单元 = Requirement「PersistentTopic 质心表示」（MODIFIED）的用户故事。complexity: simple，但任务行命中算法关键词，按 test-design ⑤a 提供本文档。

## 故事 1：话题质心随命中演化但有界（主链路）

用户手工建话题「日本内外产业政策」，选了 2 条产业政策新闻（首义=均值）。此后 79 天杂闻持续命中，期望质心被内容拉着走但始终锚在创建定义附近——今天的社会新闻 tag 不再直连。

| 步 | 动作 | 来源 Scenario | 期望 | 层 | 落点 |
| --- | --- | --- | --- | --- | --- |
| 1 | CreateManualTopic 选 2 条 section | （既有能力，回归） | embedding=所选均值，centroid 为空→首刷后=首义 | PG 集成 | daily_report_centroid_test.go（既有回归） |
| 2 | 命中 section 累计 <2 条时重算 | section 不足退化首义向量 | centroid=首义 | PG 集成 | daily_report_centroid_test.go |
| 3 | 命中达 2+ 条后重算质心 | 质心为首义与滚动窗口混合 | centroid=α·首义+(1-α)·窗口均值，元素级断言（α=0.5） | PG 集成 | daily_report_centroid_test.go |
| 4 | 杂闻持续命中再重算 | 质心有界漂移 | 质心向杂闻方向移动量 < 纯滚动位移；到首义距离有界 | PG 集合 + 函数单测 | blendCentroidWithAnchor 单测 + centroid_test 数值断言 |
| 5 | 社会新闻 tag 以原 0.166 距离再匹配 | （需求动机，端到端效果） | 新质心下距离 >0.18，降级 L2 走 LLM | 人工：真库效果核对（见下） | — |
| 6 | 新 section 归属变更触发刷新 | 质心增量更新 | 刷新自动走新公式 | PG 集成（既有用例改造断言） | daily_report_centroid_test.go |

## 故事 2：存量漂移质心一次性纠正

部署新公式后，用户调用重算端点把漂了 4 个月的质心拉回锚定态。

| 步 | 动作 | 来源 Scenario | 期望 | 层 | 落点 |
| --- | --- | --- | --- | --- | --- |
| 1 | POST recompute-topic-centroids（无 board_id） | 存量质心重算 | 202 + status=processing，异步执行 | handler | recompute handler 测试 |
| 2 | 带 board_id 过滤 | 存量质心重算 | 只重算该板话题 | handler | recompute handler 测试 |
| 3 | board_id 非法（0/abc） | 存量质心重算 | 400 invalid board_id | handler | recompute handler 测试 |
| 4 | 重复调用 | 存量质心重算 | 幂等：同输入同输出 | PG 集成 | centroid_test（连调两次断言相等） |
| 5 | 单话题重算抛错 | 存量质心重算 | 记日志继续其余话题 | PG 集成 | centroid_test（注入一条畸形维度话题） |

## 故事 3：配置边界

| 步 | 动作 | 来源 Scenario | 期望 | 层 | 落点 |
| --- | --- | --- | --- | --- | --- |
| 1 | embedding_config 未设 key | 读取默认配置 | α=0.5 | 函数单测 | LoadPersistentTopicConfig 单测 |
| 2 | 设 α=0.3 | 自定义锚定权重 | 混合按 0.3/0.7 | 函数单测 | 同上 |
| 3 | 设 α=0 / α=1 | 锚定权重配置边界 | 分别退化纯滚动 / 纯首义 | 函数单测 | blend 单测 + config 单测 |
| 4 | 设 α=-0.5 / 1.5 / "abc" | 锚定权重配置边界 | 回退默认 0.5 + Warnf | 函数单测 | config 单测 |

## ⓪ 继承与调整（契约 MODIFIED 反查：bash scripts/harness/test-assets.sh persistent-topic）

| 旧 Scenario | 处置 | 旧测试 | 动作 |
| --- | --- | --- | --- |
| 质心按近期 section 计算 | MODIFIED（改混合公式） | daily_report_centroid_test.go（TestMeanPgVectors 系 + DB 用例，验收轨） | 更新断言：均值后叠加 α 混合；meanPgVectors 纯函数保持不动 |
| 质心随内容漂移 | MODIFIED（改有界漂移） | 无直接自动化（设计意图场景） | 新增数值断言进 centroid DB 用例（步 4）；保留人工真库核对 |
| section 不足退化首义向量 | UNCHANGED | centroid_test 退化用例 | 保留原样 |
| 质心增量更新 | UNCHANGED（刷新自动走新公式） | daily_report_lane_integration_test.go 刷新路径 | 保留，不改变断言结构 |
| 配置项·读取默认配置 | MODIFIED（+1 key） | 无独立自动化（散在 lane 测试默认值） | config 单测新增 key 默认值断言 |
| 其余 63 个 Scenario | UNCHANGED | — | 不动（lane 分桶/生命周期/watch 均只消费锚点） |

## 变体走查（五组固定清单）

- **输入**：α 非法值（负/超1/非数字）→ 回退默认✓（故事3-4）；首义 NULL/空串 → 回退纯滚动✓（spec 场景「首义不可用」）；首义维度≠均值维度 → 回退纯滚动+Warn✓；滚动均值零向量（meanPgVectors nil）→ 既有退化路径（返回首义）保持。
- **前置**：0 条 section（新话题未命中）→ ComputeTopicCentroid 查空 → <2 退化首义✓；1 条 → 退化✓；恰好 2 条 → 混合生效（边界：窗口=2）；centroid_window=1 配置 → 均值=单条，混合仍成立。
- **时间窗口**：窗口边界（第 30/31 条 section）→ 既有窗口 SQL 不变；同日多条 section → 逐条计入（既有 Scenario 不变）；跨窗口淘汰 → 均值侧不变。
- **幂等**：recompute 重跑 → 同输入同输出（无随机、无时间依赖）✓；部分失败重试 → 逐话题独立，失败话题可单独重跑✓；并发：端点沿 backfill 惯例不做并发保护（单用户系统、运营操作），不声称线程安全——划除。
- **可用性（UI）**：本 change 无 UI 改动——整组划除。

## 效果核对（真库量化，④）

- **触发原因**：错挂直接动机是 1009 漂移（见 proposal Why）。
- **方法（重算前只读模拟）**：Go 只读脚本连真库取 1009 首义+最近 30 条窗口均值，算 α=0.5 混合质心，再查 tag 268293/268151 及创建时 5 条 section 到新质心的距离（不写库）。
- **量化预期**：268293（陈杰寻人）到 1009 距离 0.166 → >0.18（出直连区）；创建时 5 条 section 距离 0.214 → 显著下降（拉回锚定）；268151（海保）0.111 → 预期仍在 L1/L2 区（观察是否误伤）。
- **结论落点**：验证节留 psql 复查命令；若 268151 被推出过远，调 α 或结合 L2 观察后复盘。
