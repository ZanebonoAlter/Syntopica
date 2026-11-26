<!-- test-cases: fix-board-ghost-false-positive -->
<!-- 资产反查：bash scripts/harness/test-assets.sh board-direction-check
     主 spec 8 Requirements / 22 Scenarios；前继资产 2026-10-13-fix-digest-tag-leak（含 test-cases.md）。 -->

# Test Cases: fix-board-ghost-false-positive

方向校验四层化（composite_hit 免检 → 全库最优豁免 → 绝对门 → 相对门）+ 可选地板线。
主链路故事：**tag「日本首相高市早苗任命…」（具体事件）挂载「日本新闻」（宽泛类目）**——identity cosine 0.5826 低于绝对门 0.6，但为全库最高 → 免标 → 计入日报。

## ⓪ 改契约了吗（继承与调整表）

改了：MODIFIED `board-direction-check`「Direction check for non-direct_hit matches」（三层判定序 + 全库最优豁免层）、ADDED「Direction floor configurable」；`tag-to-board-matching` 红线表述同步。其余 7 个 Requirement（数据加载 / threshold 配置 / flag 落库 / 日报过滤 / 文章 API 过滤 / match detail direction_sim / margin 配置）不变。前继资产逐行处置：

| 旧 Scenario（fix-digest-tag-leak 及主 spec） | 处置 | 旧测试落点 | 动作 |
| --- | --- | --- | --- |
| hit_rate match passes direction check | 继承（不变） | `semantic_board_matching_unit_test.go` "hit_rate match with aligned embeddings passes" | 原样保留 |
| hit_rate match fails direction check | **契约收窄**：fails 仅当非全库最优 | `semantic_board_matching_unit_test.go` "hit_rate match with orthogonal embeddings" | fixture 加候选外真家 board 20（`boardEmbs` 多一项），断言不变 |
| weighted match fails direction check | 同上 | `semantic_board_matching_unit_test.go` "weighted match with orthogonal embeddings" | 同上 |
| tag or board embedding unavailable | 继承 | `semantic_board_matching_unit_test.go` "no tag/no board embedding skips direction check" | 原样保留；新增「无 embedding 版块不进参照系」用例（TestGlobalBestExemption） |
| direct_hit match | **契约收窄**：三层判定 | `semantic_board_matching_test.go` `TestSemanticBoardMatchingDirectHitDirectionMismatch` | fixture 加候选外真家 board（DB 层），断言 true 不变 |
| relative gate catches multi-mount cross-domain leak | 继承（最优者本就 gap=0，行为等价） | `semantic_board_matching_unit_test.go` "multi-mount cross-domain leak flagged" | 原样保留，全绿验证 1.3 |
| legal dual-mount within margin survives | 继承 | 同文件 "legal dual-mount within margin survives" | 原样保留 |
| single-board mount unaffected by relative gate | 继承 | 同文件 "single-board mount unaffected" | 原样保留 |
| composite_hit exempt | 继承 | 同文件 "composite_hit exempt" | 原样保留 + TestGlobalBestExemption「exemption not regressed」加强（cos 0.20 且非最优仍免标） |
| custom margin / margin zero / no margin row | 继承 | `semantic_board_matching_config_test.go` `TestLoadConfig` | 原样保留 |
| DirectionSimThreshold configurable / 数据加载 / 落库 / 日报过滤 / 文章 API / direction_sim | 继承（本轮零改动，1.4 读码比对） | 各自既有落点 | 不动 |
| （旧 spec 无「全库最优豁免」语义） | **新增行为** | `TestGlobalBestExemption`（10 用例）/ `TestDirectionGateThreeLayers`（3 用例）/ unit_test 新 subtests | 新建 |
| （旧 spec 无 floor 配置） | **新增行为** | `TestConfigDirectionFloor`（3 用例）+ floor 两边界用例 | 新建 |

## 主链路故事表

| 步 | 动作 | 来源 Scenario | 期望 | 层 | 落点 |
| --- | --- | --- | --- | --- | --- |
| 1 | tag identity embedding 就绪，对 `boardEmbeddings` 全量（含候选外）算 identity cosine，取最优集合（并列差值 < 1e-9 入集） | requirement 规则行「全库最优参照系 MUST 覆盖候选外」 | 最优集合 = 全库最高者集合 | 函数单测 | `computeGlobalBestBoards`；TestGlobalBestExemption 全组 |
| 2 | 挂载「日本新闻」cos 0.5826 < 0.6 且 ∈ 最优集合 | global-best mount exempted despite low absolute cosine | `direction_mismatch=false` | 函数单测 | TestGlobalBestExemption "global-best mount exempt…"；TestDirectionGateThreeLayers exemption branch |
| 3 | 两版块并列最优均挂载 | tied global best also exempted | 双 false | 函数单测 | TestGlobalBestExemption "tied global best…" |
| 4 | 浮点近并列（差 5e-10）视为并列；差 1e-8 不算（白盒边界） | requirement 规则行「1e-9 判等」 | 近并列 false / 明确更低 true | 函数单测 | TestGlobalBestExemption "near-tie within 1e-9…" |
| 5 | 单挂唯一候选，真家在候选外（cos 0.82 > 挂载 0.55） | non-best mount below threshold still flagged / global-best reference spans beyond this run's candidates | 挂载 true（不误豁免） | 函数单测 | TestGlobalBestExemption "true home outside candidates…" |
| 6 | 地板线覆盖：floor=0.55 > best cos 0.515 | floor overrides exemption | true（豁免收回，绝对门照判） | 函数单测 | TestDirectionGateThreeLayers floor branch；TestGlobalBestExemption "floor overrides…"；等值边界 cos==floor false |
| 7 | direct_hit 同三层 | direct_hit match / 降级 direct_hit 全库最优豁免 / 方向校验不通过 | 最优 false；非最优低 cos true | 函数+DB | TestGlobalBestExemption "direct_hit follows…"；`TestSemanticBoardMatchingDirectHitDirectionMismatch`（真库 testcontainer） |
| 8 | composite_hit 免检 | composite_hit exempt | 恒 false（cos 0.20 且非最优也免） | 函数单测 | TestGlobalBestExemption "composite_hit exemption not regressed"；既有 relative gate composite 用例 |
| 9 | tag / board embedding 缺失 | tag or board embedding unavailable | 全跳门 false；缺失版块不进参照系 | 函数单测 | 既有 "no tag/no board embedding…"；TestGlobalBestExemption "missing tag…"/"board without embedding…" |
| 10 | 相对门对最优集合显式跳过 | relative gate catches multi-mount cross-domain leak | 最优挂载 false、弱挂载 true | 函数单测 | `TestEvaluateSemanticBoardMatches_DirectionRelativeGate`（既有全绿，1.3 验证） |
| 11 | flag 落库 → 日报 `NOT direction_mismatch` 过滤 | direction_mismatch persisted / Daily report excludes… | 免标 tag 进日报 | DB 写路径（下游零改动） | `TestSemanticBoardMatchingDirectHitDirectionMismatch`（写行断言）+ backfill 幂等用例；日报读路径无改动（1.4 同源确认） |
| 12 | floor 配置读取（默认 0 / 自定义 / 越界拒绝） | floor disabled by default / floor overrides exemption | 默认 0=豁免无下限 | 函数单测 | `TestConfigDirectionFloor`（config 层） |

## 五组变体走查

| 变体组 | 答案 |
| --- | --- |
| 输入（空串/空白/分隔符/超长） | **不适用**——判定输入是向量非字符串。「空」的向量形态由 len==0 前置检查覆盖：`board without embedding excluded from reference set` / `missing tag embedding skips all layers` |
| 前置（空集/单元素/重复/越界） | 空集：`boardEmbeddings` 空 → 最优集合空 → 走旧门（missing embedding 用例覆盖）；单元素：唯一 board 即最优 → 豁免（"global-best mount exempt…" 即单元素参照系）；重复：并列最优（"tied…"）；越界引用：无（boardEmbeddings 与挂载集合天然同源） |
| 时间窗口 | **不适用**——判定无时间维度（划除留痕） |
| 幂等 | rematch-all 重跑结果稳定（`TestSemanticBoardBackfillAllModeAppliesNewRules` 幂等断言继承不变）；LoadConfig 缓存改值后 `InvalidateBoardCache` 重读一致（`TestConfigDirectionFloor` 每子测试失效缓存） |
| 可用性（UI） | **不适用**——`ui-impact: none`，前端零改动（划除留痕） |

## 五问句

0. **改契约了吗**：是——见 ⓪ 继承与调整表；旧测试 4 处 fixture 按新契约调整、其余原样继承。
1. **节拍全吗**：主链路 12 步全部有落点；无负向节拍；外部依赖失败答案 = embedding 缺失跳门保持 false（spec 明文，防「证据缺失」误读为「方向不符」）；真库效果核对为人工步骤（见效果核对节）。
2. **变体走查**：见五组变体表，逐项有答案或划除留痕。
3. **层选对了吗**：纯判定逻辑 → 函数单测（`semantic_board_matching_unit_test.go`）；DB 写路径 → testcontainer PG（`semantic_board_matching_test.go` / `semantic_board_backfill_test.go`，repository 层禁 SQLite 已遵守）；HTTP/前端零改动（1.4 读码比对 + 下游消费方读同一 flag），`ui-impact: none` 无 agent-browser 落点需求。
4. **效果核对了吗**：效果依赖真库数据分布 → 部署后真库量化核对（下节 4.1/4.2）。
5. **展示字段盘点了吗**：无新增用户可见字段；`direction_mismatch` 既有字段的行级真值分布变化（约 671 行 true→false），来源语义有 spec 锚（MODIFIED requirement），前端消费方零改动。

## 效果核对（部署后人工执行，对应 tasks 4.1/4.2）

### 4.1 rematch-all 全量重算

- 人工：后端部署后手动触发 rematch-all（管理界面或 API 触发 `semantic_board_backfill` mode=all）。
- 只读核对：`SELECT count(*) FROM topic_tag_board_labels WHERE direction_mismatch;` 期望从 1214 降至 ~543（±10% 容差，差异来源为存量陈旧 flag 修正）。
- 执行日期：2026-10-15　结果：基线留底 1217（1214 + 日常新增 3）→ 重算后 **1070**（总量 5190，rematch-all 3448/3448 成功、9m43s）。**未达字面预期 ~543±10%，偏差归因见下**。
- 偏差归因（豁免生效性实证）：① 全库最优挂载 **2313 行被标 mismatch 的为 0**（复算 SQL 切分：`best_still_flagged=0 / non_best_flagged=1070`）——豁免层零冤案，预测的 671 条冤案全部翻案；② 剩余 1070 条全部非全库最优，抽搜 5 条形态全部为「该拦的真串味」（挂载 cos 0.55-0.60 < 阈值 0.6、真家明显在别处）；③ 543→1070 的差额来源：proposal 预期基于验证时点快照，本次重算前真库发生过 embedding 重生成（proposal 已记录的陈旧 flag 背景）+ 日常新增数据 + rematch 挂载重排，非最优集合本身膨胀，±双向 flag 修正都体现在 1070 里——预测容差 ±10% 低估了数据漂移幅度，非判定逻辑问题。
### 4.2 次日体检快照与日报回归

- 只读核对：`SELECT metrics->'board'->>'ghost_matches' FROM board_health_snapshots WHERE semantic_board_id=2128 ORDER BY checked_at DESC LIMIT 1;` 期望从 33 回落至个位数。
- 人工：日报（日本新闻）页面抽查——高市早苗任命 / 央行类事件 tag 重新出现。
- 留痕（2026-10-24 归档前核对）：ghost_matches 快照 10-18/19/21 依次 28/28/25（事故期）→ **2026-10-22 重算后 3**（个位数达标，当日三轮体检均为 3）；llm_score 65→85。日报回归：日本新闻(2128) event_tag_count 10-20 事故期 1 → 10-22 **8**（含高市早苗）→ 10-23 **5**（含央行），核心 tag 恢复收录 ✅。补注：本 4.2 字面口径「33→个位数」以 fix-board-direction-gap-exempt tasks 4.2 数据为准（gap 豁免 10-22 上线后达标，D4 衔接）。
