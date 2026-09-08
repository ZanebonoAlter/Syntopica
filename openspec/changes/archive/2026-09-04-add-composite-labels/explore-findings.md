
## 第 4 节匹配规则改造开工上下文

第 4 节（匹配规则升级）开工上下文，semantic_board_matching.go（backend-go/internal/tagmanagement/service/board/）关键结构：

**现状四规则**（evaluateSemanticBoardMatches，约 L246）：
- direct_hit：countDirectSemanticBoardHits ≥ config.DirectHitMinOverlap → score=1.0、match_reason="direct_hit"、`continue` 直接跳过（豁免 direction check 的根源就在这个 continue——matchReason != "direct_hit" 才做方向校验）
- 间接三规则：hit_rate / max_sim（含 downgraded）/ weighted，全部走 direction check（tagEmbedding vs boardEmbeddings[boardID] cosine < DirectionSimThreshold → directionMismatch=true）
- 排序：score 降序、同分 boardID 升序；MatchTopicTag 截断 MaxBoards（默认 3）

**待改造点（tasks 4.1-4.3）**：
1. config 加 DirectHitScoreFactor 字段（默认 0.7，ai_settings key=semantic_board_match_direct_hit_score_factor，迁移已 seed，LoadConfig 的 key 列表要加这个 case）
2. direct_hit 降级：score = factor × 1.0，不再 continue，强制走 direction check（同一处 matchReason != "direct_hit" 条件改为所有 reason 都校验，或 direct_hit 分支单独算）
3. composite_hit 最强：tag 组合 ∩ board 组合 ≠ ∅ → score=1.0、reason="composite_hit"、direction_mismatch=false；同 board 同时满足单标签重叠时只记 composite_hit（优先级：composite_hit > direct_hit > 间接三规则）
4. 输入加载（task 4.1）：LoadTagAuxiliaries 模式照抄——tag 关联组合 = JOIN topic_tag_semantic_labels + label_type='composite' AND status='active'；board 组合 = board_composition 挂载组合（注意：board_composition.auxiliary_label_id 列名复用，查询时 semantic_labels.label_type='composite' 过滤）。组合 embedding 从 semantic_labels.embedding 取（组合创建时 LLM 生成，ParsePgVector 解析）
5. 缓存（semantic_board_cache.go）：packageBoardCache 已有 GetBoardAuxiliaries/SetBoardAuxiliaries、GetBoardEmbeddings/SetBoardEmbeddings、GetConfig/SetConfig 模式——组合标签数据加 Get/SetBoardComposites + GetTagComposites 不缓存（tag 维度单查），board 侧进缓存；composition 变更失效点在 InvalidateBoardCache（board service 调用，composite 挂载变更也要触发）

**旧测试处置（test-cases.md 继承与调整表已定）**：
- semantic_board_matching_test.go:TestSemanticBoardMatchingDirectHit（L24）——断言 score=1.0 → 改 0.7×factor，补 direction_mismatch 两态
- upsertMatchSetting("semantic_board_match_direct_hit_min_overlap","1") 相关用例——交集判定照跑 + 补降级断言
- 测试基建：setupSemanticBoardMatchingTestDB + createMatchLabel(t, db, label, slug, type, status, vector) + createMatchTag 已存在

**handler 路由已注册**：/api/composite-labels（GET 列表/POST 创建/:id/disable/:id/enable），compositeLabelEmbedder 测试缝在 handler 包。

**剩余任务顺序**：4.x 匹配规则 → 5.x 升级建议 compose → 6.x 前端 → 7.x 存量重算 → 8.2 效果核对 → 9.x 文档 → 10.x 验证。semantic_board_upgrade.go 的 SuggestSemanticBoardUpgrades（L~258 事务执行处）与 ComputeSuggestionHash 是 5.x 改造点，loadCoTagEventContext（L452，窗口 CoTagWindowDays=30/TopN=20/DedupeSim=0.85/HardLimit=15）是 compose 候选对的统计基础。

<!-- pinned 2026-09-02T16:50:55Z -->

## 前端交互原型流程缺口与 add-composite-labels 证据

现行 `docs/reference/开发执行规范.md` §3 虽名为“先脑暴→后原型”，但可丢弃原型的 MUST 触发条件仅为≥3状态循环/算法/多模块协议；“交互形态未定”只要求对话探索，并未要求页面交互/视觉原型。§5.3 只在实现后做 opencli 交互验证与 k3 视觉验证；entry-gate 只检查 complex change 的 test-cases 文档。因此前端 UI 可以在没有用户确认的信息架构、布局模式、响应式与状态画面的情况下直接实现。
`add-composite-labels` 的 proposal/spec/tasks 只描述治理面板字段与动作（列表、创建、启停、compose 卡片、过滤）；design.md D1-D7 均为数据/算法/匹配/迁移决策，没有页面入口、主任务、状态流、布局宽度、响应式或原型。change 目录只有 proposal/specs/design/tasks/test-cases/explore-findings，无 HTML/截图/wireframe 原型。事实库 30 天保留窗口内共 130 次 subagent.dispatch，描述命中 UI/视觉/原型的仅 8 次；92 个出现 pnpm lint 的 session 中仅 4 个同 session 有该类派发。add-composite-labels 关联 6 个 session，仅 3 次派发且都是其他 change 的约束文档改写，UI/视觉/组合标签派发为 0（注意技能调用本身不记账、description 关键词统计只能作采用率信号）。
实际布局：`TagsPage.vue` 是 full-width workspace，`.tags-content` 无 max-width；`CompositeLabelPool` 直接撑满主内容区。`CompositeLabelEditDialog` 自写 Teleport + `width:min(560px,92vw)`，未复用 theming.md 强制的 AppDialog；AppDialog 默认 480px 且 width 为自由字符串。前端标准只有色彩 token/组件复用与少数交互约定，没有页面 shell（centered/wide/split）和 dialog size tier，仓内 max-width 值高度碎片化。这支持“居中设计和宽屏设计未统一”主要是流程契约与布局基线缺失，而不只是单页 CSS 问题。

**引用**：docs/reference/开发执行规范.md:132、docs/reference/开发执行规范.md:195、docs/reference/standard/frontend/theming.md:26、docs/reference/standard/frontend/testing.md:17、openspec/changes/add-composite-labels/design.md:25、openspec/changes/add-composite-labels/tasks.md:34、front/app/features/tags/components/TagsPage.vue:167、front/app/features/tags/components/TagsPage.vue:382、front/app/features/tags/components/CompositeLabelEditDialog.vue:228、front/app/components/ui/AppDialog.vue:2、.pi/harness/events.db

<!-- pinned 2026-09-03T07:34:14Z -->

## 8.2 真实库效果核对结论与三个链路缺口修复

2026-09-03 真实库（10451 tags）验证 add-composite-labels 全链闭环并挖出修复 3 个缺口：

**量化结论**：compose 候选 12 对 → LLM 通过 9（75%，拒绝的「伊朗×特朗普」「OpenAI×Anthropic」类同域并列组合合理）；确认 5 组合挂 3 版块；backfill mode=all 重算后 composite_hit 44 行/37 tags（score 全 1.0、0 mismatch）、direct_hit 342 行全部 0.700 降级（35 行方向不符标记）、间接三规则量级稳定。抽样：收购事件簇 12 tags 归「生成式AI」、沃什放鹰簇归「美国新闻」、财报 tag 靠组件齐全推导进「科技巨头」；单 tag 明细零 direct_hit 误挂。

**修复的 3 个链路缺口**（真实验证才暴露，单测全绿也挡不住）：
1. `board_crud_handler.go` composition POST 校验写死 label_type='auxiliary' 拒组合挂载 → 放宽 IN(auxiliary,composite)，补 Case 6/7 测试
2. composition 挂载/移除路径从不失效匹配缓存（板级缓存无 TTL，只有升级确认失效）→ board 包新增导出 `InvalidateMatchCache()`，POST/DELETE composition 两处调用
3. **tag↔组合关联零写入**（确认创建只建组合本体；提取端 Q1 推迟）→ `LoadTagComposites` 改推导式：tag 挂齐 active 组合全部组件 aux 即视为挂该组合（显式 ∪ 推导），全局组合组件集进缓存（`CompositeComponentSet`，随 InvalidateBoardData 失效）——「确认→重算」闭环的语义根基，已回写 design D5/spec delta「组件齐全推导组合」Scenario/flow 文档

**环境事实**：后端重启后仅监听 IPv6（WSL curl v4 localhost 空回复 code 52）——WSL 侧调后端 API 用 `cmd.exe /C curl.exe` 或 powershell.exe 中转（Windows 侧 localhost→::1 均 200），POST JSON 建议落临时文件 `-d @D:\...` 绕三层转义；WSL interop vsock 会瞬断（UtilAcceptVsock accept4 failed 110），sleep 重试自愈。

<!-- pinned 2026-09-03T15:18:23Z -->
