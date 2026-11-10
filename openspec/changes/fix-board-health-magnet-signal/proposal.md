<!-- complexity: complex -->
<!-- ui-impact: minor -->
<!-- constraint-domains: semantic-board -->

## Why

版块体检（board health）的「体验核查」把版块**核心主体**当成疑似污染项列进「待核对」，用户侧表现为：板块「中东地缘政治与美伊关系」（`semantic_labels.id=1974`）体检抽屉列出 47 项待核对，首项就是「伊朗」，证据写着「题材标签候选：全局站台分组 0.666（303 项）· 失败挂载子集 0.542（19 项）· 差值显著，疑似感知污染」。

真库只读核对（`board_health_snapshots.id=410`，2026-11-09 21:23）确认三件事：

1. **磁铁信号口径与语义相反**。`signals.go:computeMagnetSignals` 判 `suspicious = all_mean − failed_mean ≥ 0.08`——「磁铁」的本意是某标签**相对更吸引跑题挂载**（failed 侧更高），代码却判成「failed 比全体低就算嫌疑」。失败子集按定义就是与本板方向不符的挂载，与版块核心主体天然不相似，于是**越是核心主体 gap 越大、越像污染源**：伊朗 all=0.6661 / failed=0.5424 / gap=0.124，德黑兰 0.6417/0.5428，卡塔尔 0.6566/0.5439。本板 47 个构成标签里 **41 个**被判磁铁嫌疑成立（平均 gap 0.106，阈值仅 0.08），信号几乎无区分度。
2. **失败子集是版块级共享行集**，却当标签级证据用（`signals.go` 自带注释已声明「不做单标签吸引归因」）；板级 `magnet_flow`（"失败挂载成因：美国新闻"）又被渲染进**每个**标签的证据块，放大误导。
3. **同一批数据结论会翻转**。本板 11-06 10:14 那轮是 13 keep + 34 rewrite（项目自己在 `openspec/changes/archive/2026-11-06-board-health-triage-advice/tasks.md:42` 记下锚点「德黑兰/伊朗 = keep」），代码与提示词一字未改，11-06 21:03 与 11-09 两轮变成 47/47 全 rewrite。这条锚点当时只以散文形式存在，没变成代码约束或回归测试，所以没人拦住。

放大因素：板块 1974 的 `description` 在库里为空 → LLM 手上的「版块定义」只有标题，没有「伊朗是核心主体」的文本依据；提示词只禁止短实体标签判 `remove`、不禁 `rewrite`，且把「近邻是地点/机构类标签」当窄化证据（对国家/地名这类实体标签，同类近邻恰是健康形态）；分诊清单**没有任何占比合理性闸门**，47/47 全 actionable 原样落库并在前端全量呈现为「待核对」。

## What Changes

- **磁铁信号口径反转（口径层）**：`failed_mean` 与**健康挂载子集**均值比较，`suspicious = failed_mean − good_mean ≥ 阈值`（方向与「磁铁吸跑题」语义对齐）；健康子集样本不足或向量缺失一律 `insufficient`，不硬凑。`all_mean/all_samples` 保留（历史快照兼容 + 仍展示"全板均值"），但不再参与嫌疑判定。
- **核心主体标记（主体保护层）**：构成标签名出现在版块 **description 或 aliases** 中 → `core_subject=true`；**仅标题命中**时还需标签名 ≥ 3 字（2 字短词既可能是简称也可能是泛化地域词，不据此推断全名或泛化词）——纯字面匹配，不做语义推断。标记随快照持久化并进 LLM 证据块。规则层强制：`core_subject` 标签被 LLM 判 remove/rewrite → 改判 keep 并标注「核心主体保护」+ 原判留痕。
  - ⚠️ 边界：板块 1974 的标题「中东地缘政治与美伊关系」用的是简称「美伊」，**标题并不含「伊朗」**——该板的核心主体保护**依赖下面「数据补齐」项在 description 里明示「伊朗」**，代码不做简称→全名推断。
- **提示词硬约束（LLM 层）**：① `core_subject` 标签 MUST NOT 判 remove/rewrite；② 实体类标签（国家/地区/城市/机构/人物）的同类最近邻 MUST NOT 记为漂移/窄化证据；③ 板级共享失败子集与 `magnet_flow` MUST NOT 当作单标签证据；④ 版块 description 为空时显式告知模型「核心主体不可从文本判定」。
- **规则层三道闸门**：G1 核心主体保护（见上）；G2 `remove` 必须有成立的磁铁信号（新口径），否则降级为证据不足；G3 可执行占比（remove+rewrite）超配置阈值时本轮全部可执行项降级为证据不足并在快照记录闸门原因 + 占比（占比取 **LLM 原始裁决**，如实反映模型激进程度）。
- **磁铁口径显式标记**：磁铁信号新增 `caliber` 字段（新口径 `"healthy_subset"` / 空 = 旧口径快照），供前端区分「旧口径快照」与「新快照但健康侧不可算」——避免把新快照误标成旧口径、又把含失败行的全体均值摆回界面。
- **前端展示与提示（minor）**：磁铁对比改用「失败子集 vs 健康子集」口径；核心主体徽标；占比超限时界面显示"本轮信号可能不可用"提示条；闸门降级项自动退出「待核对」索引（复用既有 remove/rewrite 分流，不新增修复入口）。
- **数据补齐**：给板块 1974 补定义式 `description`（**必须明示「伊朗」为本板核心主体**，这是该板核心主体保护生效的前提——标题只含简称「美伊」）——给 LLM 文本依据，同时让代码层核心主体判定能命中；**不替代**上述代码层修复（口径反转与三道闸门对任何板都生效）。
- **重跑一轮体检**：修完后重跑，验证伊朗不再进「待核对」、本板可执行占比回落到合理区间；历史脏快照（370 / 410）保留不删（用户决策）。

### Out of Scope

- 不改 `direction_mismatch` 的产生逻辑（匹配链方向校验不动），不动 `cosine_mean` 贴合度分布口径（spec 口径②不动）；
- 不清理历史脏快照（370 / 410 保留留档，用户决策）；
- 不做「证据引用结构化校验」（triage item 增结构化证据字段并逐条校验）——独立后续 change，本次只加 G2/G3 两道闸门；
- 不改「体检轮次纯读」红线（只写 `board_health_snapshots` + 冷却内通知），不新增扩充建议、不动匹配表；
- 不动组合标签（composite）只读边界与描述修复链的既有隔离。

## Capabilities

**Modified Capabilities**（requirement 变更 → 需 delta spec）：

- `board-health-checkup`：
  - 「标签分诊信号计算」——磁铁信号口径反转（failed vs 健康子集）+ 新增 `core_subject` 标记；
  - 「LLM 综合评分与降级」——分诊判据（核心主体保护 / 实体标签同类近邻不算漂移 / 板级证据不作标签级证据 / remove 必须磁铁成立）+ keep 新增「核心主体保护」情形；
  - 「分诊以问题处理为主线」——闸门提示条、核心主体徽标、磁铁对比口径展示。

**Added Capabilities**（新 requirement → 需 delta spec）：

- `board-health-checkup`：「分诊合理性闸门」——核心主体拦截、remove 证据校验、可执行占比超限降级，降级项不得呈现为可执行建议。

## Impact

- **后端**：`internal/tagmanagement/service/boardhealth/signals.go`（磁铁口径 + 核心主体标记）、`metrics.go`（挂载行按方向分组、健康子集统计）、`llm.go`（提示词硬约束、证据块、规则层闸门 G1/G2/G3）、`boardhealth.go`（新增 `board_health_max_actionable_ratio` 配置，默认 0.8）、`internal/tagmanagement/models/board_health.go`（`BoardHealthMagnetSignal.GoodMean/GoodSamples`、`BoardHealthLabelMetrics.CoreSubject`、快照级 `triage_guard`）、`run.go`（闸门落库与日志）。
- **前端**：`front/app/api/semanticBoards.ts`（类型新增字段）、`front/app/api/normalizers/boardHealthTriage.ts`（归一化新增字段）、`front/app/features/tags/components/console/board-health/boardHealthWorkspace.ts`（派生层）、`BoardHealthIssueIndex.vue` / `BoardHealthIssueDetail.vue`（徽标 / 口径展示 / 闸门提示条）。
- **API**：`GET /api/semantic-boards/:id/health-checkup` 响应**非破坏性追加**字段（`magnet.good_mean` / `magnet.good_samples` / `labels[].core_subject` / `metrics.triage_guard`）；无新端点、无路由变更、旧快照缺字段时按旧形态展示。
- **DB**：无表结构变更、无迁移（新增字段全部落在既有 `board_health_snapshots.metrics` jsonb 内）。
- **配置**：新增 `ai_settings` 键 `board_health_max_actionable_ratio`（默认 0.8，缺行走默认）。
- **数据**：板块 1974 的 `semantic_labels.description` 补一段定义式描述（写库动作，用户已批准；不写则 LLM 仍缺文本依据）。
