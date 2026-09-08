## 1. 数据模型与迁移

- [x] 1.1 `models/semantic_label.go`：SemanticLabel 注释与 CompositeLabel 模型新增（label_type="composite" 语义）、`CompositeComponent` 模型（composite_id, component_label_id, position，PK 复合主键，TableName）+ 模型单元测试（表名/主键 gorm tag 校验，照 TopicTagBoardLabel 测试模式）
- [x] 1.2 `platform/database/`：迁移新增 `composite_components` 表（PK (composite_id, component_label_id)，FK composite_id ON DELETE CASCADE 指向 semantic_labels）+ 迁移测试（testcontainer PG，验证表结构与级联删除）
- [x] 1.3 验证 label_type 字符串列对 "composite" 值自然兼容（无枚举 CHECK 约束阻拦），`go test ./internal/models ./internal/platform/database -short -run TestComposite` 通过

## 2. 组合标签服务层（auxlabel）

- [x] 2.1 组合标签创建服务：`CreateCompositeLabel`（组件校验：2-5 个、必须 auxiliary 类型且 active；事务内建行 + composite_components + LLM 组合 embedding（`label + ". " + description` 模式）；embedder 失败整体回滚）+ 单元测试（组件数量越界/类型错误/回滚）
- [x] 2.2 去重 canonical 化：L1 组件 canonical ID 无序集合比较（复用/ ref_count++）、L2 组合 embedding cosine ≥ `composite_dedupe_sim`（默认 0.95，ai_settings 可配）addAlias、均未命中新建；L2 只 addAlias 不改 label 不重算 embedding（防黑洞纪律对齐）+ 单元测试（三种路径 + alias 幂等）
- [x] 2.3 禁用/启用：禁用同事务 embedding 置 NULL（行/组件/aliases 保留），启用后 embedding 重算路径接入 + 单元测试（禁用后向量为 NULL、组件保留）
- [x] 2.4 `go test ./internal/tagmanagement/... -short` 组合标签相关包全绿

## 3. 组合标签 CRUD API

- [x] 3.1 handler：组合标签列表（组件序列按 position、ref_count、status 过滤）/ 手动创建（source="manual"，触发去重返回复用提示）/ 禁用启用三个端点注册路由 + handler 测试（创建去重命中返回既有组合信息）
- [x] 3.2 API 文档注解（swag）+ `go build ./...` 通过（注：仓库无 swag 工具链，API 文档统一落 docs/reference/api/，归任务 9.2；`go build ./...` 已过）

## 4. 匹配规则升级（tag-to-board-matching）

- [x] 4.1 匹配输入：MatchTopicTag 加载 tag 关联组合标签与 board composition 组合标签（含 embedding），纳入 board match cache（composition 变更失效语义扩展）+ 单元测试（缓存命中/失效）
- [x] 4.2 composite_hit 规则：tag 组合 ∩ board 组合 ≠ ∅ → score=1.0、match_reason="composite_hit"、direction_mismatch=false；同 tag-board 同时满足单标签重叠时只记 composite_hit + 单元测试（纯组合命中/优先级覆盖）
- [x] 4.3 单标签 direct_hit 降级：交集 ≥ direct_hit_min_overlap → score=direct_hit_score_factor（默认 0.7，ai_settings 可配）、强制 direction check（低于 direction_sim_threshold 标 direction_mismatch=true）+ 单元测试（方向不符/通过两态 + 分数断言）
- [x] 4.4 `go test ./internal/tagmanagement/service/board -short` 全绿（含既有测试回归：direct_hit 旧断言按新契约更新——score 1.0→0.7、direction_mismatch 新增，遵循 test-design ⓪ 继承与调整）

## 5. 升级建议 compose 决策（board-upgrade）

- [x] 5.1 co-tag 共现对统计：升级生成流程新增候选共现对收集（窗口复用 CoTagWindowDays，频次 ≥ `composite_cotag_min_cooccurrence` 默认 10，组件 ref_count 达升级阈值，topN 上限防 O(n²) 膨胀）+ 单元测试（阈值过滤/上限截断）
- [x] 5.2 LLM 裁决 compose：候选共现对进 LLM prompt（输出 compose/skip + 组合名 + 描述），SuggestSemanticBoardUpgrades 契约扩展；ComputeSuggestionHash 签名扩展 compose 决策（组件 ID 有序序列）+ 单元测试（hash 幂等性——同组件序列同 hash）
- [x] 5.3 compose 建议落库与生命周期：decision="compose" 经 hash 幂等 + 冷却期检查落库 pending；列表/过滤 tab 兼容新决策值 + 单元测试（幂等 skip / cooldown_blocked / 生命周期状态迁移）
- [x] 5.4 确认执行：同一事务创建组合标签（调 2.1 创建服务，含去重复用路径）+ MarkConfirmed；embedder 失败回滚建议保持 pending + 单元测试（成功/失败回滚/去重复用三态）
- [x] 5.5 `go test ./internal/tagmanagement/... -short` 升级建议相关包全绿

## 6. 前端

- [x] 6.1 治理面板：组合标签管理页（列表：label/组件序列/ref_count/status；手动创建：组件选择器限 auxiliary active 2-5 个；禁用/启用）+ Vitest 组件测试（列表渲染/创建校验）
- [x] 6.2 升级建议面板：compose 建议卡片（组合名/组件序列/共现证据：频次+窗口+代表事件）、确认（调 upgrade-execute）/dismiss、决策过滤 tab 增「组合」+ Vitest 组件测试（compose 卡片渲染/过滤）
- [x] 6.3 匹配详情：composite_hit 命中展示（组合名 + 组件序列）+ Vitest 组件测试
- [x] 6.4 `cmd.exe /C "cd /d D:\project\Syntopica\front && pnpm lint && pnpm exec nuxi typecheck && pnpm test:unit"` 通过（lint 0 error/5 存量 warning；typecheck ✓；test:unit 809 全绿；build ✓）
- [x] 6.5 组件推荐交互（design D7，用户两轮 review）：第一轮——`GET /composite-labels/component-options` 推荐排序 + 「挂 N 版块」徽标 + 相关现有组合提示（含 L1 集合一致预告复用）；第二轮升级为**版块中心**——版块详情「组合标签」区（挂载列表/移除/创建并挂载，composition 接口返回组合挂载条目）+ `board_id` 上下文（本版块组件置顶「本版块」徽标 + 创建成功自动挂载）+ `related_to` 共现联动（选中组件后候选按同 tag 共现频次实时重排标「共现N」——真实库组合稀少，纯现有组合提示联动太弱）；后端 3 测试 + 前端 7 测试全绿

## 7. 存量重算

- [x] 7.1 一次性全量重算：复用现有 backfill mode="all"（新规则自动生效），验证脚本确认重算后 direct_hit 存量记录 score=0.7 且方向不符记录带 direction_mismatch=true（testcontainer PG 集成测试或 staging 手动验证留痕）→ TestSemanticBoardBackfillAllModeAppliesNewRules 覆盖（降级/组合命中/幂等三断言）

## 8. 测试

- [x] 8.1 test-cases.md：主链路故事表（compose 建议 → 确认 → 匹配 → 重算全链）+ 变体走查（输入/前置/时间窗口/幂等/可用性五组）+ 白盒附加（匹配优先级分支表）+ ⓪ 继承与调整表（direct_hit 旧测试处置：`bash scripts/test-assets.sh tag-to-board-matching` 反查）
- [x] 8.2 效果核对：真实库量化——候选 12 对/LLM 通过 75%（9/12，拒绝项合理）/确认 3+2 组合/重算 10451 tags 后 composite_hit 44 行全 1.0、direct_hit 342 行全降 0.700；抽样 3 组合（收购簇/放鹰簇/财报推导）误归类验证通过；过程修复 3 个链路缺口（composition 拒 composite、缓存不失效、组合关联零写入→推导式语义），量化与结论记入 test-cases.md 效果核对节

## 9. 文档

<!-- doc-impact: flow api database -->

- [x] 9.1 `docs/reference/flow/semantic-board.md`：匹配规则（composite_hit + direct_hit 降级）、组合标签链路、升级建议 compose 决策、粒度阶梯、业务约束节新增组合标签红线（去重 canonical 化/禁用弃向量/embedding 禁合成）
- [x] 9.2 `docs/reference/api/`（组合标签 CRUD + 建议决策新值）与 `docs/reference/database/`（composite_components 表）同步
- [x] 9.3 `docs/research/composite-label/explore-findings.md` 关键结论合并入 flow 文档后标注已吸收

## 10. 验证

- [x] 10.1 `cd backend-go && golangci-lint run ./...` 退出码 0（0 issues）
- [x] 10.2 `cd backend-go && go vet ./... && go build ./...` 退出码 0
- [x] 10.3 `cd backend-go && go test ./internal/tagmanagement/... ./internal/models ./internal/platform/database -short` 全绿（影响包：models / database / tagmanagement）
- [x] 10.4 `cmd.exe /C "cd /d D:\project\Syntopica\front && pnpm exec nuxi typecheck && pnpm test:unit && pnpm build"` 全部退出码 0（typecheck ✓；test:unit 838 全绿——含另一 workstream 新增 25 例；build ✓）
- [x] 10.5 `bash scripts/doc-impact.sh verify openspec/changes/add-composite-labels/` 本 change 自身声明域（flow/api/database）无缺失；报告的 3 个「疑似遗漏」（architecture/standard/configuration）系 make-ui-design-first-class workstream 脏文件（runtime.go/config.go/standard 三份文档）跨污染——该批改动不属本 change，归档时留痕说明
- [x] 10.6 `bash scripts/check-standards.sh` G 段（死链）通过；F 段本 change 项同 10.5 跨污染说明（make-ui-design-first-class 自身的 FAIL 由该 workstream 负责）
