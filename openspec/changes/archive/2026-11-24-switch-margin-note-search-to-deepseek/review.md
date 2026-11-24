# Review — switch-margin-note-search-to-deepseek

> 审查执行：`change-review` 子线程（`zai-coding-cn/glm-5.4:max`）逐模块审查；实现子线程 `general-purpose`（`commandcode/deepseek/deepseek-v4.1-flash:high`）。审查 brief 存 `.pi/harness/briefs/switch-margin-note-search-to-deepseek/review-m*.md`（本地不入库）。条目格式契约：`openspec/specs/review-trace-gate`。

## 模块 5 — 页边注卡三态标记（MarginNoteCard）

- 审查范围：`front/app/features/tags/components/daily-report/MarginNoteCard.vue`、`MarginNoteCard.test.ts`（diff +26/-9、+49/-18）；核验 `nuxi typecheck` 通过、`pnpm test:unit MarginNoteCard.test.ts` 13/13 绿
- 结论：**通过**（无 High / Medium；6 条 Low）
- 核实要点：`empty` 态文案 / title / testid `mn-web-search-empty` 与 HEAD 逐字一致；四处处读同一个 `qa.cited_web_sources`；`<script>` 段零改动（纯展示红线成立）；旧用例「仅 empty 渲染」已整体替换为三条新用例，「不渲染」循环仅剩 `disabled / null / undefined`

### R-1 2026-11-23【Low】红线文档约束 27 仍是旧语义（「批注卡仅 empty 渲染，ok/disabled/NULL 不渲染」）
- **状态**：已采纳（由 task 6.1 / 模块 6 承接：`docs/reference/flow/daily-report.md:353` 改为三态规则「`ok` 且有网络来源→已联网；`ok` 且无网络来源→已联网 · 未采用；`empty`→联网搜索无结果；`disabled`/NULL 不渲染」，已列入模块 6 必改项）

### R-2 2026-11-23【Low】tasks.md 5.2 复选框未勾选
- **状态**：已修复（审查读到的是主线程并发写 tasks.md 的竞态中间态；现 5.1 / 5.2 均已勾选，勾选统一改走单条 sed）

### R-3 2026-11-23【Low】class `mn-qa__search-empty` 更名为 `mn-qa__web-status`
- **状态**：已采纳（判定成立无需修：规则体与 HEAD 逐字相同，仅选择器更名以承载三态，仓库零外部引用；ui-design §3 已补「原位更名」溯源句）

### R-4 2026-11-23【Low】`MarginNoteCard.vue:153` 注释保留 `add-searxng-health-indicator` 字样
- **状态**：误报丢弃（归档 change 名作溯源标注，HEAD 同位置已有，非用户可见文案）

### R-5 2026-11-23【Low】ui-design §3 对样式族的描述与实际 CSS 不符（写作「同一字号、`--text-secondary`」，实为 0.62rem / `--text-muted`）
- **状态**：已修复（订正 ui-design §3 措辞；实现本身忠实复用既有规则，非实现缺陷）

### R-6 2026-11-23【Low】`mn-web-ok` 多了制品未要求的 title「本次问答已联网检索并引用了网络来源」
- **状态**：已采纳（与另两态对齐，无害，保留）

## 模块 1 — 新搜索入口 `internal/platform/websearch`

- 审查范围：`backend-go/internal/platform/websearch/` 7 个文件（实现 574 行 / 测试 641 行）；brief：`review-m1.md`；核验 `go build` / `go vet` / `golangci-lint`（0 issues）/ `go test -count=1`（21 顶层用例全绿）；spec 四个 Requirement 与「故事四」9 行逐条有代码落点
- 结论：**通过**（无 High；1 条 Medium、10 条 Low）；实现子线程报告的 4 处偏差（日志只在 health.go / ProbeHealth error 恒 nil / PublishedAt 用 string / 多一档 InvalidQuery）均判定成立

### R-7 2026-11-23【Medium】`ProbeHealth` 的 `error` 返回值恒为 nil——下游不得依赖 `err != nil` 判失败
- **状态**：已修复（模块 3 已落实——本模块不改；由 task 3.2 承接：handler 只按 `Health.Status` + `ErrorKind` 分流，`err` 预留恒 nil，杜绝 `err != nil → 500` 死分支）

### R-8 2026-11-23【Low】brief G1/G2「期望零命中」字面未满足（`searxng` / `airouter` 命中全在注释），删包后成悬空引用
- **状态**：已修复（模块 3 已落实——由 task 3.7 承接：删除 searxng 包时把 websearch 注释里「沿用 searxng 取值」类措辞改为不带包名的说法）

### R-9 2026-11-23【Low】`DB == nil` 被归入 `ErrNotConfigured`，与「无命中」混同
- **状态**：已驳回（`database.DB` 仅在 `InitDB` 完成前为 nil，服务态不可达；查询失败路径未被吞——包装后 `errors.Is(ErrNotConfigured)=false`、`KindOf=Unavailable`，核心可区分性成立；与旧 `searxng/health.go` 同口径）

### R-10 2026-11-23【Low】`providerTypeOpenAICompatible` 与 `airouter.ProviderTypeOpenAICompatible` 字面重复、无漂移守卫
- **状态**：已修复（模块 3 已落实——由 task 3.7 承接：`credentials_test.go` 加一行 `require.Equal` 守卫；若 import airouter 代价过大则汇报后放弃）

### R-11 2026-11-23【Low】spec「错误分类」正文枚举 6 类，实现多一档 `invalid_query`
- **状态**：已修复（delta spec `web-search-provider`「错误分类」正文补「搜索词非法的本地拒绝 SHALL 返回 `invalid_query` 且不发起网络请求」，与同 spec「空搜索词拒绝」Scenario 对齐）

### R-12 2026-11-23【Low】`PublishedAt` 用 `string` 透传 RFC3339 原值而不解析为 `time.Time`
- **状态**：已采纳（判定成立，无需改动：spec 用词「透传 / 缺失时为零值」，string 最忠实，且避免 `time.Time` 零值的 `0001-01-01T00:00:00Z`）

### R-13 2026-11-23【Low】`hostOf` 接受无 scheme 的 base_url，但请求路径不补 scheme
- **状态**：已驳回（无 scheme 的 DeepSeek vendor 在 chat 侧早已不可用，非现实配置；届时面板可诊断，不改）

### R-14 2026-11-23【Low】`unreachableDetail` 默认分支把 ≤200 runes 响应体摘要透到面板 `Detail`
- **状态**：已修复（模块 3 已落实——由 task 3.7 承接：`Unavailable` 默认分支改固定文案，响应体摘要只进日志；避免 5xx 返回 HTML 时面板出现 `<html>` 片段）

### R-15 2026-11-23【Low】`models.AIVendor` 未登记在 `.golangci.yml` 共享模型白名单注释（存量缺口）
- **状态**：已修复（模块 3 已落实——由 task 3.8 承接：仅补一行注释；该文件若已被其他 active change 弄脏则不改并汇报）

### R-16 2026-11-23【Low】日志只在 `health.go`，`Search` 内不记 Warn
- **状态**：已采纳（判定成立，无需改动：QA 路径每次失败已 Warn 且 `Error()` 自带 Kind；四处日志均不打印 API Key）

### R-17 2026-11-23【Low】模块 2 的并行扇出会每 query 读一次 DB 取凭据
- **状态**：已采纳（design D1 / spec「凭据来源与配置解析」明示每次现读，单行索引小表，成本可忽略；不引入跨调用缓存）

## 模块 2 — 页边注链路切换（topicgraph 层）

- 审查范围：`backend-go/internal/topicgraph/service/margin_notes_websearch.go`、`margin_notes_qa.go`、`margin_notes_qa_test.go`（228+/132-；`margin_notes_handler_test.go` 无需改动）；brief：`review-m2.md`；核验 `go vet` 零告警，service 包 20 用例（+ 12 子用例）、handler 包 7 用例全绿
- 审查执行说明：首次派发中断于 `zai-coding-cn` 5 小时额度上限（429），由用户手动重拉完成，报告经用户转贴后由主线程录入
- 结论：**通过**（无 High / Medium；2 条 Low）；实现子线程报告的 5 处偏差（保留原名合并函数 / 包内 `clampSearchQuery` / 删旧哨兵 / 预检钩子带 ctx / 多加两用例）均判定成立
- 核实要点：`margin_notes_qa.go` 归一化类型改名后与 HEAD 的差异只剩注释行（system prompt、联网块措辞、JSON schema、单次回答调用、operation、capability、两道白名单逐字一致）；「未配置 → query 生成不发起」与「非 NotConfigured 错误不跳过」均有用例锁定；合并去重（首次为准）+ `SliceStable` 按 Score 降序 + 截 5；prompt 拼装处无 200 runes 二次截断；五条真人主故事全部走通

### R-18 2026-11-23【Low】brief / tasks 的验证命令选不中测试（handler 包 `-run MarginNote` 空跑；service 包窄模式漏掉新增的重排用例）
- **状态**：已修复（tasks 7.1 的 service 包改用宽模式 `-run 'MarginNote|TestMergeSearchResults|TestFilterWebSourcesAgainstCandidates|TestFilterCitedAgainstCandidates|TestParseMarginNoteQA'`、handler 包改用 `-run 'Annotation|MarginNote'`；2.4 / 2.5 的命令文本与 `review-m2.md` 同步订正；模块 6 子线程已经 steer 收到更正）

### R-19 2026-11-23【Low】单 query 回退路径不做 Score 重排，与并行路径不对称（spec 允许：回退后行为与既有实现一致）
- **状态**：已修复（`searchWebForMarginNote` 注释补「单 query 不重排：顺序即服务商顺序」，逻辑不变）

## 模块 4 — 前端 API 层与设置面板（SearXNG tab 退役 → 网页搜索）

- 审查范围：新建 `front/app/api/webSearch.ts`(+test)、`front/app/components/dialog/WebSearchConfigPanel.vue`(+test)；修改 `front/app/api/index.ts`、`SettingsSectionDatasourcesNetwork.vue`(+test)、`SettingsWorkspace.vue`(+test)；删除 `front/app/api/searxng.ts`、`SearxngConfigPanel.vue`(+test)（合计 +119/−445）；brief：`review-m4.md`；核验 7 个测试文件 64 用例全绿
- 审查执行说明：同模块 2，首次派发中断于额度上限，由用户手动重拉完成
- 结论：**有条件通过**（2 条 Medium 需归档前处理；8 条 Low 含域外 / 制品级）；旧能力去向表 9 项全部「已迁移」或「有 spec 依据的主动退役」，零遗漏；两条旧深链经两级同值映射幂等落地且不互踩；实现子线程报告的 4 处偏差均接受
- 根因说明：两条 Medium 的源头都在主线程所写制品——「AI 路由」是写错的分区名（实为「AI 模型」），delta spec 漏修订主 spec 的旧键重定向 Scenario

### R-20 2026-11-23【Medium】空态指引「AI 路由」指向不存在的分区名，最近似项「能力路由」是错目标
- **状态**：已修复（task 4.7 已落地：面板注释/说明句/空态文案全改「AI 模型」；`auth`/`quota` 时状态行下渲染去向提示行 `data-testid="web-search-health-hint"`，其余 error_kind 不渲染；空态断言加 `not.toContain("AI 路由")`）

### R-21 2026-11-23【Medium】delta spec 未修订主 spec「旧 section 键深链重定向」Scenario 对 searxng 的字面
- **状态**：已修复（delta `settings-workspace` 新增 MODIFIED「设置导航分组与复合 section」：旧键列表移除 searxng，新增 Scenario「已下线面板的旧键重定向到替代 tab」覆盖 `?section=searxng` 与 `tab=searxng` 两层及跨分区负向）

### R-22 2026-11-23【Low】`LEGACY_TAB_REDIRECT` 两份字面拷贝、无单一来源
- **状态**：已修复（task 4.8 已落地：新建 `front/app/features/settings/legacyRedirects.ts` 作单一来源，`SettingsWorkspace.vue` 与 `SettingsSectionDatasourcesNetwork.vue` 均改为 import，本地字面拷贝已删；两处消费方测试 16+9 例全绿）

### R-23 2026-11-23【Low】`degraded` 色点硬编码 `#c98a2e`，未用主题变量 `--color-warning`
- **状态**：已修复（task 4.9 已落地：`background: var(--color-warning, #c98a2e)`）

### R-24 2026-11-23【Low】`vendor_configured=true` 且 `vendor_name=''` 时渲染「已关联供应商：」悬空冒号
- **状态**：已修复（task 4.9 已落地：`vendor_name || "（未命名）"` + 用例）

### R-25 2026-11-23【Low】「四态各一例」实为单用例循环；缺 D6「无可编辑字段」契约守卫断言
- **状态**：已修复（task 4.10 已落地：四态与 error_kind 拆为独立用例（15 tests），新增 D6 守卫 `it` 断言无 input/textarea/form/AppToggle）

### R-26 2026-11-23【Low】ui-design §4 字面过期（「GET/POST/health 三接口」）；tasks 4.5 的 `legacyTabMap` 在仓库不存在
- **状态**：已修复（ui-design §4 改为两接口形状并补验收行；tasks 4.5 改为真实的两级重定向结构）

### R-27 2026-11-23【Low】loading 态用文字「正在读取关联状态…」而非 ui-design §2.1 写的「骨架」
- **状态**：已采纳（功能等价，保留实现；ui-design §2.1 措辞改为与实现一致）

### R-28 2026-11-23【Low / 域外】`SettingsWorkspace.test.ts` 未 mock `@iconify/vue`，happy-dom 真实出网拉图标（ETIMEDOUT 噪声，单文件 139s）
- **状态**：已采纳（task 4.10 顺手加 `vi.mock('@iconify/vue')`，参照同目录 `SettingsSidebar.test.ts`；属既有问题，非本模块引入）

### R-29 2026-11-23【Low / 制品级】ui-impact=minor 的 `ui-design.md` 缺 `## Interaction Decisions` 与 `## Data Provenance`（`interaction.md` 写「minor 档起必填」，但无门禁强制）
- **状态**：已修复（本 change 的 ui-design 补齐两节）；仓库级治理缺口（无检查脚本、归档区 50 份 minor 制品仅 4 份有）不在本 change 处理，已在完工汇报中提请用户另开治理项

## 模块 3 — 设置 API 与 searxng 清理（后端）

- 审查范围：`discovery/routes.go`、`discovery/handler/discovery_handler.go`（+新增 `websearch_health_handler_test.go`）、`platform/aisettings/config_store.go`(+test)、`platform/config/config.go`、`admin/routes.go`、`platform/websearch/**`（3.7 收尾）、`.golangci.yml`（+1 行）；删除 `platform/searxng/` 整目录 + `searxng_health_handler_test.go`；brief：`review-m3.md`
- 结论：**有条件通过**（0 High；2 条 Medium 均为 spec 制品缺陷，主线程已修；4 条 Low）；旧能力去向表 7 项全部「已迁移」或「有 spec 依据的主动退役」，零遗漏；六条真人主故事全通；`config_store.go` 的 `+141/−12` 经逐行核对确为「他人的 141 行原样 + 删掉 searxng 三处」；R-7/8/10/14/15 五条「已修复」标注经复核属实
- 核实要点：`go build ./...` exit 0（删包后无悬挂 import）；两条 GET 路由、settings 三分支、外层 10s > 内层 8s、四态一律 200 且 `err != nil` 仅兜底、接缝 `t.Cleanup` 还原；探测不走 airouter / 不写 `ai_call_logs` / 无 ticker（红线 27）；R-14 的 `Detail` 断言升级为「非空且不含响应体片段」

### R-30 2026-11-23【Medium】`web-search-provider` delta 出现与「无 POST / 无独立保存动作」自相矛盾的旧剧本「保存后自动探测」
- **状态**：已修复（改写为「探测异常不阻塞面板」，去掉「完成保存」前提）

### R-31 2026-11-23【Medium】`settings-workspace` delta 的 REMOVED「SearXNG 配置与探测」指向主 spec 中不存在的 Requirement，归档时被当「已移除」静默跳过，Reason/Migration 丢失
- **状态**：已修复（删除悬空 REMOVED 块；退役范围与迁移要点并入 MODIFIED「数据源连通性探测」正文：`searxng_config` 键 / endpoint·enabled 字段 / `SEARXNG_URL` / `searxng.endpoint` 不再被读写、三条旧路由下线 404、旧深链重定向）

### R-32 2026-11-23【Low】tasks 3.3 验证口径「`rg -i searxng backend-go --type go` 零命中」字面不成立
- **状态**：已修复（任务行补记实际余 4 处均为 `add-searxng-health-indicator` 归因注释及具体行号，按口径视为零残留）

### R-33 2026-11-23【Low】`.golangci.yml` 的「（18 项 + 1 例外）」「跨 platform 层共享（6）」两个计数在补登 `AIVendor` 后失真
- **状态**：已驳回（受 task 3.8「至多 1 行变更」预算约束；行内已标「基线后补登」；计数属 `decouple-backend-domains` 的基线快照，改它应由该治理项统一重算）

### R-34 2026-11-23【Low】`openspec validate --strict` 退出码 1，两条 WARNING 均为「Requirement 文本 >500 字符」
- **状态**：已驳回（MODIFIED 必须整块复制主 spec 正文，长句源于主 spec 既有写法；两条均 0 ERROR，tasks 8.x 未把 strict 列为硬门禁）
