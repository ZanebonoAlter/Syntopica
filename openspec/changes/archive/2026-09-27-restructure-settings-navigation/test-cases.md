# Test Cases — restructure-settings-navigation

> 复杂档白盒用例文档。覆盖 specs 三个 delta 全部 Scenario；验收措辞遵循 test-design（命令+期望 / 文件存在 / 人工：验证方式）。
> 域：settings-workspace / user-onboarding / data-enrichment。

## 0. 效果核对（⓸）

| # | 触发原因 | 方法 | 量化结果 | 结论 |
| --- | --- | --- | --- | --- |
| E1 | 删除方法卡/参考角色注入后，主张「prompt 无可见差异」 | 真库查询（explore 已做）+ `grep -rn "referenceRoleAppendix\|MethodCard\|methodCard\|AnalysisMethod" backend-go/internal --include='*.go'` | `analysis_methods` 表 total=1/enabled=0/legacy=1（从未启用）；`reference_roles` 仅 seed 数据；grep 零命中（任务 5.2 验证） | 删除零行为损失成立 |
| E2 | 日报时刻修复后调度器真实生效 | 人工：设置页改时刻保存 → 观察 `docker logs` 下一轮 dailyReportWindow 读取新值 | 保存后 ≤60s 调度窗口用新时刻；后端无 404 | 修复生效 |

## 1. 主链路表（节拍 → 落点）

### 故事一：设置页分组导航（settings-workspace）

| # | 节拍/动作 | 来源 Scenario | 期望 | 层 | 落点 |
| --- | --- | --- | --- | --- | --- |
| S1.1 | 打开设置页 | 设置入口进入工作区；分组导航渲染 | 侧栏渲染 4 个分组标题（内容管理/AI 配置/数据源与网络/运行状态）+ 7 个导航项；组标题不可点击；无 15 项平铺 | 组件 | `SettingsSidebar.test.ts`（新建：断言 4 组标题 + 7 项）+ opencli（8.1） |
| S1.2 | 点「数据源与网络」 | 复合 section 进入默认子 tab | 默认落 Firecrawl 子 tab；runtime-status 默认 AI 健康；不跨 section 记忆 | 组件 | 复合容器测试（新建，任务 2.3）：断言默认子 tab |
| S1.3 | 子 tab 切「出站代理」 | 复合 section 子 tab 切换与 URL 承载 | URL 变 `section=datasources-network&tab=proxy`，面板切换为代理面板 | 组件 | 复合容器测试：断言切换后 route.query.tab=proxy + opencli（8.1） |
| S1.4 | 刷新页面 | URL 定位设置分区 | 停留在 `datasources-network` + `tab=proxy` 子 tab | 端到端 | opencli（8.1）：刷新后断言 URL 与激活 tab |
| S1.5 | 访问 `?section=proxy`（旧键） | 旧 section 键深链重定向 | `router.replace` 到 `section=datasources-network&tab=proxy`；9 旧键同构；无 404/空白 | 组件 | Workspace 重定向测试（任务 2.4）：9 旧键用例 + opencli（8.1） |
| S1.6 | 访问 `?section=datasources-network&tab=xx` | 非法 tab 值回退 | 回默认 Firecrawl 并清除 tab 参数 | 组件 | Workspace 重定向测试：非法 tab 用例 |

### 故事二：日报时刻读写（settings-workspace）

| # | 节拍/动作 | 来源 Scenario | 期望 | 层 | 落点 |
| --- | --- | --- | --- | --- | --- |
| S2.1 | 运行状态 → 定时任务子 tab | 打开定时任务设置显示真实值 | 输入框显示库中 `daily_report_time` 真实值（当前库值 21:00 亦为真实配置，验证以 GET /api/ai/settings 200 为准，非静默默认）；后端日志无 404 | 人工 | 人工：设置页打开定时任务子 tab + `docker logs` 核对（任务 3.1） |
| S2.2 | 改时刻并保存 | 保存后生效 | 「已保存」反馈出现；下一轮调度窗口（≤60s）读新时刻 | 人工 | 人工：保存后观察调度器日志（E2） |

### 故事三：settings tour 引导（user-onboarding）

| # | 节拍/动作 | 来源 Scenario | 期望 | 层 | 落点 |
| --- | --- | --- | --- | --- | --- |
| S3.1 | 清除 `syntopica_onboarding_settings_complete` 后访问 /settings | Settings tour auto-starts on first visit | tour 在挂载后自动开始 | 组件 | `useOnboarding.test.ts`（沿用既有用例，锚点键更新后跑绿） |
| S3.2 | 点 header 引导按钮 | Settings tour re-trigger from header | tour 立即重触发（无刷新） | 组件 | `useOnboarding.test.ts`（既有用例沿用） |
| S3.3 | 完成 home/tags tour 后访问 /settings | Settings tour independent of other tours | settings tour 仍运行（独立 completion key） | 组件 | `useOnboarding.test.ts`（既有用例沿用） |
| S3.4 | tour 运行中逐步走查锚点 | Settings tour anchors resolve on grouped navigation | 每步锚点解析到真实元素（含 `settings-nav-runtime-status`）；`settings-nav-schedulers` 不再被引用；pre-filtering 日志零跳步 | 组件+人工 | `useOnboarding.test.ts`（新增锚点断言）+ 人工：跑一次 settings tour 核对（任务 7.1） |

### 故事四：单泳道分析无画像注入（data-enrichment）

| # | 节拍/动作 | 来源 Scenario | 期望 | 层 | 落点 |
| --- | --- | --- | --- | --- | --- |
| S4.1 | 触发单泳道分析 | 单泳道不继承作者画像 | 编排输入不含作者画像与方法卡内容（选卡/清洗/注入代码全链不存在） | service 单测 | `board_investigation*_test.go`（改造后：删方法卡断言、保留「无画像注入」断言）+ grep 零命中（任务 5.2 验证） |
| S4.2 | 解读员处理话题 | 消费分层上下文；解读员领域自适应 | 输入含 context 层+14天窗口+applied review；非金融话题不强制固定方向（既有行为，回归保障） | service 单测 | 既有测试沿用（`enrich_board_test.go` 等，任务 5.2 改造后跑绿） |
| S4.3 | 分析员产出 | 分析员产出深度层而非走向预测 | depth 块兼容、无 direction/trigger 字段（既有行为，回归保障） | service 单测 | 既有测试沿用 |
| S4.4 | 研究助理重复调用 | 死循环防御 | 相同工具+参数重复调用被拦截（既有行为，回归保障） | service 单测 | 既有测试沿用 |
| S4.5 | 从简报/调查下钻 | 下钻问题可修改且可推翻 | 预填 lens 可改、结论可异于预填（既有行为，回归保障） | service 单测 | 既有测试沿用 |

### 故事五：删除链不残留（负面节拍，SHALL NOT）

| # | 节拍/动作 | 来源 Requirement | 期望 | 层 | 落点 |
| --- | --- | --- | --- | --- | --- |
| S5.1 | grep 后端符号 | 独立设置工作区（入口 SHALL NOT 存在）+ 编排（SHALL NOT 注入） | `grep -rn "referenceRoleAppendix\|ReferenceRole\|methodCard\|MethodCard\|AnalysisMethod" backend-go/internal --include='*.go'` 零命中（任务 4.2/5.3 验证） | 静态 | grep 命令（tasks 4.1/5.2/5.3 内置验证） |
| S5.2 | 前端组件/api 不存在 | 独立设置工作区 | `SettingsSectionReferenceRoles/AnalysisMethods.vue`、`AnalysisMethodPanel.vue`、`api/analysisMethods.ts`、`api/referenceRoles.ts`（如存在）文件不存在 | 静态 | `ls` 验证（tasks 2.1 顺带） |
| S5.3 | DB 两表消失 | 画像体系整体移除 | `\dt` 无 `reference_roles` / `analysis_methods` | SQL | 任务 6.1 验证 |

## 2. 变体走查（五组固定清单）

| 组 | 变体 | 答案 / 处置 | 落点 |
| --- | --- | --- | --- |
| 输入 | 非法 tab 值（`tab=xx`） | 回默认子 tab + 清参数 | S1.6 组件用例 |
| 输入 | 空 tab 参数（`?section=datasources-network&tab=`） | 等价无 tab → 回默认并清参数 | S1.6 组件用例（补一条空串用例） |
| 输入 | 旧键大小写（`?section=PROXY`） | 键匹配大小写敏感：未命中已知键 → 走「未知 section」既有回退（默认第一个 section），不做大小写归一 | S1.5 组件用例补一条 PROXY 用例断言不重定向到复合键 |
| 输入 | 特殊字符（`?section=<script>`） | 未知键回退默认 section（既有行为），值仅用于 map 查找不渲染 | 既有回退覆盖（不新增用例，白盒分支表 D-else 行留痕） |
| 前置 | 空集 | 15 个 section 组件常量静态定义，无空集态 | 划除（不适用：导航静态） |
| 前置 | 重复 | 9 旧键映射表键唯一，无重复命中 | 划除（静态 map，类型保证） |
| 前置 | 越界引用 | section 键即边界：未知键回默认 | S1.5 补 PROXY 用例同覆盖 |
| 时间窗口 | 调度生效窗口 | 保存后 ≤60s 窗口读新值（后端既有机制，不新建窗口测试） | S2.2 人工核对 |
| 时间窗口 | 边界两端（时刻格式） | HH:MM 校验在后端 SaveSettings 既有（不在本 change 范围） | 划除（后端行为未改） |
| 幂等 | 重定向重复执行 | `router.replace` 同 URL 幂等；旧键→新键映射无环 | S1.5 用例顺带（重定向后 URL 稳定） |
| 幂等 | drop 表迁移重复执行 | `DROP TABLE IF EXISTS` 幂等 | 任务 6.1 验证（临时容器预检含重复执行） |
| 幂等 | 并发 | 单用户设置页，无并发诉求；未声称线程安全 | 划除 |
| 可用性 | 误输入反馈 | 非法 tab 不报错静默回默认（spec 定） | S1.6 |
| 可用性 | 空态 | 导航/tab 均静态无空态；子面板空态沿用现有（研究数据源空目录等） | 划除（沿用） |
| 可用性 | 错误态 | 子面板错误气泡在面板内展示不顶掉 tab 条（ui-design State Matrix） | opencli 8.1 顺带目检 |
| 可用性 | 加载态 | 子面板沿用自身 loading；容器即时渲染 | 划除（沿用） |
| 可用性 | 超长文本 | 分组标题/导航 label 均短文案静态常量 | 划除（不适用） |
| 可用性 | 重复提交 | 保存按钮防抖属各子面板既有逻辑，复合容器不拦截 | 划除（沿用） |

## 3. 继承与调整表（⓪ 改契约了吗）

契约变更：settings-workspace MODIFIED（15 平铺→4 组 7 项、URL +tab）、user-onboarding MODIFIED（tour 锚点）、data-enrichment MODIFIED（编排不注入方法卡）。旧测试资产反查（test-assets.sh 三 capability）：

| 旧资产 | 断言旧契约？ | 处置 | 动作 |
| --- | --- | --- | --- |
| `SettingsSectionAiHealth.test.ts` | 是（挂薄壳组件） | 薄壳删，测试改挂实体面板（AiHealthPanel 等）或并入复合容器测试 | 改 import/挂载目标（任务 2.3/2.5） |
| `SettingsSectionSearxng.test.ts` | 是（挂薄壳组件断言面板行为） | 同上，测试逻辑保留改挂实体 | 改挂载目标（任务 2.3/2.5） |
| `AnalysisMethodPanel.test.ts` | 是（被删功能本体） | 随功能全链删除 | 整删（任务 2.1） |
| `useOnboarding.test.ts` | 部分（settings tour 锚点/文案） | 锚点键改 `settings-nav-runtime-status` 后跑绿；S3.4 新增锚点断言 | 更新步骤定义 + 用例（任务 7.1） |
| `SettingsSectionFeeds.test.ts` / `SettingsSectionMarginNotes.test.ts` | 否（feeds/margin-notes 键不变、组件保留） | 原样跑绿 | 无动作（2.5 收口跑一遍确认） |
| `useMarginNoteAdmin.test.ts` / `TagQueuePanel.test.ts` / `FeedMasterList.test.ts` / `FeedDetailEditor.test.ts` / `FeedSourceQuality*.test.ts` | 否（子面板内部行为不动） | 原样跑绿 | 无动作 |
| dataenrichment 15 个涉及 method/role 的 `_test.go`（repository×2、handler×2、service×9、database×2） | 是（方法卡选卡/注入/清洗/CRUD 断言） | 整删或删相关断言：`analysis_method_*`、`method_sanitizer_test`、`reference_role_seed_retire_migration_test` 整删；`board_investigation*`、`enrich_board_test`、`handler_test`、`repository_test`、`board_brief_cross_relation_test` 删方法卡/角色相关断言保留其余 | 任务 5.2 逐文件改造 |
| `handler_test.go` 中 /reference-roles、/analysis-methods 路由用例 | 是 | 删路由用例 | 任务 4.1/5.1 |
| 历史 change `2026-07-25-preference-vector-feed-discovery`（settings-workspace delta） | 否（与本导航契约无关） | 无动作 | 划除 |

## 4. 白盒附加（复杂档）

### 4.1 分支表（Workspace 重定向 computed，任务 2.4 核心）

| 分支 | 输入 | 走向 | 用例 |
| --- | --- | --- | --- |
| W-a | section ∈ 9 旧键 | replace(新键, {tab: 旧键}) | S1.5（9 用例） |
| W-b | section = datasources-network 且 tab ∈ 6 子键 | 停留该 tab | S1.3/S1.4 |
| W-c | section = datasources-network 且 tab ∉ 6 子键（含空串/缺失） | replace 清 tab → 默认 firecrawl | S1.6（非法值 + 空串） |
| W-d | section = runtime-status 且 tab ∈ 3 子键 | 停留 | S1.2 扩展（ai-health/queues/schedulers 三值各一断言） |
| W-e | section = runtime-status 且 tab 非法 | replace 清 tab → 默认 ai-health | S1.6 复用 |
| W-f | section ∈ 保留键（feeds/preferences/margin-notes/ai-providers/capability-routes） | 直达不重定向 | S1.5 补一保留键反证用例 |
| W-g | section 未知（含大小写不匹配如 PROXY） | 既有默认 section 回退 | 变体·旧键大小写用例 |
| W-h | tab 参数出现在非复合 section（如 ?section=feeds&tab=proxy） | tab 无效不被消费；不报错 | S1.6 补用例 |

### 4.2 边界值

| 边界 | 值 | 期望 |
| --- | --- | --- |
| 旧键映射表完整性 | 恰 9 键：firecrawl/bocha/searxng/rsshub/datasources/proxy/ai-health/queues/schedulers | 每键有唯一新键+tab 映射（静态断言映射表长度） |
| 子 tab 首项 | datasources-network→firecrawl；runtime-status→ai-health | 进入默认落首项 |
| 抽屉断点 | 767.98px | 抽屉内分组结构同构（S1.1 组件测试 + 8.2 窄屏截图） |
| sticky tab 条 | 内容滚动时 tab 条贴顶 | 8.2 视觉检查留痕 |
| 迁移幂等 | 执行两次 drop | 第二次无错（IF EXISTS） |

### 4.3 不适用划除

- 算法复杂度/性能阈值：无新算法（map 查找 + 布局重组），划除。
- 多用户/权限：单用户无鉴权，划除。
- 国际化变体：导航文案单语常量，划除。
- 并发一致性：单用户本地应用未声称线程安全，划除。

## 5. 验收命令汇总

| # | 命令 | 期望 |
| --- | --- | --- |
| V1 | `grep -r "sectionComponents" front/app` | 无结果 |
| V2 | `pnpm exec nuxi typecheck`（front/） | 通过 |
| V3 | `pnpm test:unit SettingsSidebar SettingsSectionDatasourcesNetwork SettingsSectionRuntimeStatus --maxWorkers=2`（front/，文件名以实际为准） | 全绿 |
| V4 | `grep -rn "referenceRoleAppendix\|ReferenceRole\|methodCard\|MethodCard\|AnalysisMethod" backend-go/internal --include='*.go'` | 零命中 |
| V5 | `cd backend-go && golangci-lint run ./internal/dataenrichment/... && go test ./internal/dataenrichment && go vet ./...` | 全绿 |
| V6 | `docker exec <pg> psql -U postgres -d syntopica -c '\dt'` | 无 reference_roles / analysis_methods |
| V7 | `openspec validate restructure-settings-navigation --strict` | 通过 |
