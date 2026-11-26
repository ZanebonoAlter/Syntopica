# 测试用例 — fix-digest-tag-leak

> 测试单元 = Requirement 的用户故事，本表串成完整故事；spec Scenario 是断言片段。
> 复杂档：文末含白盒附加节（分支表 + 边界值 + 不适用划除留痕）。

## ⓪ 改契约了吗（继承与调整）

改动类型：MODIFIED×2（`Direction check`（含 RENAMED）、`事件标签去重`）+ ADDED×1（margin 配置）。

| 旧 Scenario / 旧断言 | 处置 | 旧测试 | 动作 |
| --- | --- | --- | --- |
| board-direction-check「direct_hit match → 不校验」 | MODIFIED（漂移修正：direct_hit 执行双门校验） | `semantic_board_matching_unit_test.go` L314 `TestEvaluateSemanticBoardMatches_DirectionCheck`（现仅测绝对门，未覆盖 direct_hit 豁免断言） | 旧用例保留（绝对门行为不变）；新增 direct_hit 双门用例 |
| board-direction-check 各绝对门 Scenario | MODIFIED 保留（绝对门语义不变） | 同上 L344/353/359/366/386 | 保留，补相对门新断言 |
| daily-report-system「保留 article_count 更大或 id 更小」 | MODIFIED（三级 tiebreak） | `daily_report_dedup_test.go` L37 `TestDeduplicateTags_SingleArticleOverlap`、L58 `TestDeduplicateTags_HigherArticleCountWins` | L58 保留（第一级不变）；L37 改造为三级 tiebreak 用例 + 新增贴近版块者存活/回退用例 |
| 「DirectionSimThreshold configurable」 | 未动（margin 为 ADDED） | 无独立用例（LoadConfig 路径） | 新增 margin 配置用例时不改旧断言 |

## 主链路故事 A：digest 标签多版块挂载，串味版块被方向校验拦下

| 步 | 动作 | 来源 Scenario | 期望 | 层 | 落点 |
| --- | --- | --- | --- | --- | --- |
| 1 | 标签同轮匹配 3 版块（identity cos 0.81/0.60/0.56，margin 0.15） | relative gate catches multi-mount cross-domain leak | 0.60、0.56 两个挂载 direction_mismatch=true，0.81 为 false | 函数单测 | `semantic_board_matching_unit_test.go` |
| 2 | 合法双挂（cos 0.82/0.70，gap 0.12） | legal dual-mount within margin survives | 两挂载均 false | 函数单测 | 同上 |
| 3 | 单挂（cos 0.6 ≥ 0.5） | single-board mount unaffected by relative gate | false（gap=0 相对门自然通过） | 函数单测 | 同上 |
| 4 | 绝对门独立触发（cos < 0.5） | absolute gate fails | true | 函数单测 | 同上（旧断言保留） |
| 5 | direct_hit 挂载 gap ≥ margin | direct_hit subject to direction check | true（漂移修正对齐） | 函数单测 | 同上 |
| 6 | composite_hit 挂载任意 cos | composite_hit exempt | 恒 false | 函数单测 | 同上 |
| 7 | identity embedding 缺失 | identity embedding unavailable | 跳过双门，false | 函数单测 | 同上（旧断言保留） |
| 8 | margin 配置 0.18 / 0 / 缺省 | custom margin / margin zero disables / no margin row | 分别用 0.18 / 关相对门 / 默认 0.15 | 函数单测 | 同上（LoadConfig 断言） |

## 主链路故事 B：digest 双事件标签去重二选一，保留贴近版块者

| 步 | 动作 | 来源 Scenario | 期望 | 层 | 落点 |
| --- | --- | --- | --- | --- | --- |
| 1 | 同文章两事件标签（vs 版块 cos 0.60/0.80，article_count 均 1） | 同文章双事件标签保留贴近版块者 | cos 0.80 者存活 | 函数单测 | `daily_report_dedup_test.go` |
| 2 | 任一 embedding 缺失的同文章平手 | embedding 缺失回退 ID 顺序 | id 小者存活 | 函数单测 | 同上 |
| 3 | 文章集合完全相同的标签 | 文章集合完全相同的标签去重 | 三级 tiebreak 保留存活者 | 函数单测 | 同上（旧用例改造） |
| 4 | article_count 不等 | （第一级，无 Scenario 变化） | article_count 大者存活 | 函数单测 | 同上（旧用例保留） |
| 5 | 去重后检查 topic_tags | 去重不影响原始数据 | 原始记录不变（纯内存） | 函数单测 | 同上（旧用例保留） |
| 6 | orchestrator 接线：collect 后加载 identity/board embedding 传入去重 | （实现接线，无独立 Scenario） | 加载失败时去重回退现行为且管线不中断 | 函数单测 | `daily_report_orchestrator_test.go` 或 dedup 单测注入 |

## 变体走查（五组固定清单）

| 组 | 变体 | 答案 |
| --- | --- | --- |
| 输入 | margin 配置非法值（非数字/负数） | `LoadConfig` parse 失败沿用默认 0.15（现有 parse 语义）；负数同 0 处理回退（实现按 min(0) 收敛，白盒表补边界） |
| 输入 | embedding 空串/全零向量 | 视同缺失：方向校验跳过该门；去重该对决回退 ID |
| 前置 | 标签同轮零匹配版块 | 无挂载可标记，相对门无参照集不触发 |
| 前置 | 参照集全部挂载 cos 相同（多挂 gap 全 0） | 相对门全过，仅绝对门生效 |
| 时间窗口 | —— | 不适用（去重为当日管线内存操作，无跨天窗口语义），划除 |
| 幂等 | 匹配轮重跑 / 去重重跑 | 纯函数无随机，结果确定；去重不改库，天然幂等 |
| 可用性(UI) | —— | 不适用纯后端（mismatch 前端隐藏为既有逻辑），划除 |

## 白盒附加（复杂档）

### 分支表：方向校验（evaluateSemanticBoardMatches 双门）

| # | 分支条件 | 结果 | 用例落点 |
| --- | --- | --- | --- |
| 1 | matchReason=composite_hit | 恒 false，双门均不评估 | 故事 A-6 |
| 2 | identity emb 缺失 或 board emb 缺失 | 双门跳过，false | 故事 A-7 |
| 3 | cos < 0.5（绝对门） | true（无论 gap） | 故事 A-4 |
| 4 | cos ≥ 0.5 且 gap ≥ margin | true | 故事 A-1 |
| 5 | cos ≥ 0.5 且 gap < margin | false | 故事 A-2 |
| 6 | 参照集=同轮全部匹配候选（含将被标 true 的） | bestCos 不受 mismatch 标记影响 | 故事 A-1（0.56 候选在参照集内，best 仍 0.81） |
| 7 | margin=0 | 相对门关闭（分支 4 永不触发） | 故事 A-8 |

边界值：gap == margin → true（≥ 语义，用例 A-1 用 0.21/0.15、A-2 用 0.12/0.15 夹逼，另加 gap==margin 精确等值用例）；cos == 0.5 → false（< 语义，沿用旧断言）。

### 分支表：去重三级 tiebreak（shouldReplace 扩展）

| # | 分支条件 | 结果 | 用例落点 |
| --- | --- | --- | --- |
| 1 | article_count 不等 | count 大者胜（不触 embedding） | 故事 B-4 |
| 2 | count 等 + 双 emb 齐 + cosChallenger > cosIncumbent | challenger 胜 | 故事 B-1 |
| 3 | count 等 + 双 emb 齐 + cosChallenger < cosIncumbent | incumbent 胜 | 故事 B-1 镜像 |
| 4 | count 等 + cos 相等（浮点精确相等） | id 小者胜 | 白盒补 |
| 5 | count 等 + 任一 emb 缺失 | id 小者胜（现行为回退） | 故事 B-2 |
| 6 | 标签文章集合为空 | 现行为：直接淘汰（alive=false） | 旧用例保留 |

边界值：cos 差为浮点最小间隔 → 仍按高者（不做容差，白盒表记录决策）；参照的版块 embedding 取当前日报版块单行。

### 不适用划除留痕

- LLM 行为分支：本 change 两处均为纯逻辑，无 LLM 调用路径，划除。
- 并发分支：去重与匹配均在单轮管线内串行执行，未声称线程安全，划除。
