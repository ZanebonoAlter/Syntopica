# Test Cases: fix-tagging-pollution

> 主链路用户故事：**一篇薄内容快讯从入库到打标完成，全程不产生泛词垃圾标签；调用失败时的兜底仍然可用。**

## 主链路节拍表

| # | 步骤/动作 | 来源 Scenario | 期望 | 层 | 落点 |
|---|---|---|---|---|---|
| 1 | 快讯（分类「新闻」）LLM 返回 1 event + 空 keyword 数组 | 空 keyword 结果不触发规则回填 | 结果仅含 event，来源 llm，无「新闻」 | 函数单测 | `extractor_enhanced_test.go` |
| 2 | LLM 返回两数组均空 | 空 keyword 结果不触发规则回填 | 零候选，不产生规则词候选 | 函数单测 | `extractor_enhanced_test.go` |
| 3 | 融合提取成功但零候选进入编排 | heuristic 兜底仅限提取调用失败 | `tagArticle` 不写任何行 | 函数单测（testcontainer PG） | `article_tagger_test.go` |
| 4 | 融合提取连接拒绝、3 次重试耗尽 | heuristic 兜底仅限提取调用失败 | 降级 heuristic，来源 `heuristic` | 函数单测 | `article_tagger_test.go` |
| 5 | 任一路径产出候选 label=「新闻」 | 泛词标签黑名单拦截 | persist 前被丢弃，无新行 | 函数单测（testcontainer PG） | `article_tagger_test.go` |
| 6 | sibling（同 link）带「新闻」关联，reuse 复制 | 泛词标签黑名单拦截 | 「新闻」行不被复制，其余照常复制 | 函数单测（testcontainer PG） | `article_tagger_test.go` |
| 7 | 编程文章产出「GLM Coding Plan」等正常标签 | 泛词标签黑名单拦截 | 黑名单不误伤（完整匹配） | 函数单测 | `article_tagger_test.go` |
| 8 | 存量清理 SQL 执行（备份→留底→BEGIN 预览→COMMIT） | （design D5） | 删除行数 = 留底数；二次执行 count=0 | 真库人工流程 | tasks 4.x |
| 9 | 清理后文章由队列重打（本地 4B 主力） | heuristic 兜底仅限提取调用失败 | 24h 抽样新标签泛词出现次数 = 0 | 真库量化核对 | tasks 4.4 |

## ⓪ 继承与调整

| 旧 Scenario/测试 | 处置 | 说明 |
|---|---|---|
| `test-assets.sh tagging-domain` 现存 Requirement | 无需调整 | 均为结构/参数/GORM 类，不断言回填与兜底 |
| `extractor_enhanced_test.go` 可能存在断言「空 keyword 回填 heuristic 词」的用例 | 待跑受影响包测试核对 | 跑红即按新契约改写（回填移除后不再注入） |
| `article_tagger_test.go` 可能存在断言「零结果降级 heuristic」的用例 | 待跑受影响包测试核对 | 旧契约（零结果=heuristic）按新契约改写（零结果=无标签） |
| `article_tagger_reuse_test.go` 现存用例 | 待跑受影响包测试核对 | siblingLinks 查询加黑名单排除 join 后，构造数据若用泛词 label 需换词；普通 label 行为不变 |
| `extractor_enhanced_test.go` 若存在断言「零候选降级 heuristic」的用例 | 待跑受影响包测试核对 | extractor 内部 :67 降级移除后，该分支返回零候选（Source=llm）而非 heuristic 结果 |
| aggregate 路径测试 | 不适用 | 本 change 不触及切片逻辑，persistArticleTags 过滤对聚合路径同为正向 |

## 变体走查

- **输入**：空串摘要 → beat 2 覆盖（两数组空 → 零候选）✓｜纯空白/全角空白 → `buildArticleSummary` TrimSpace 后为空，同空串路径 ✓｜单 token 标题 → 走常规 LLM 提取，无特殊分支（不适用划除）｜大小写 → 黑名单词均为中文，无大小写变体（划除留痕）｜超长输入 → 采样预算由 tagging-domain 既有 Requirement 覆盖（不适用）
- **前置**：文章已有标签 → `tagArticle` 既有 skip/dedupe 逻辑（line 104），非本 change 触及 ✓｜零候选集合 → beat 3 ✓｜重复关联 → 表无唯一约束，黑名单+零候选语义使重复插入源消失；retag 走 Force 先删后插 ✓
- **时间窗口**：无时间窗逻辑（不适用划除）
- **幂等**：清理 SQL 重复执行 → 第二次 count=0 无害 ✓｜并发打标同一篇 → 既有队列单 worker 语义，不在本 change 范围（划除）
- **可用性(UI)**：纯后端 change（不适用划除）

## 效果核对（真库量化，五问句 ④）

LLM 行为与数据覆盖依赖，单测不足以证明效果，须真库量化：

1. **清理留底对账**：删除前 count（heuristic=484 / llm 泛词≈714 新闻 + ≈90 Coding）= DELETE 实际行数
2. **重打效果**：队列重打完成后 24h，抽样受影响文章 ≥30 篇：`SELECT count(*) FROM article_topic_tags att JOIN topic_tags tt ... WHERE tt.label IN (黑名单词)` = **0**
3. **新增污染观测**：上线后 48h `ai_call_logs` 成功调用中，新落库标签命中黑名单次数 = **0**

## 白盒附加

simple 档，不设完整白盒分支表。边界补充：黑名单 Slugify 后比较需覆盖「新闻」全角/半角无差异（中文无此问题，划除）；`filterGenericLabels` 对 nil/空切片入参返回空切片；reuse 查询过滤需覆盖「sibling 标签全部命中黑名单 → 复制零行、reused 返回值语义」。
