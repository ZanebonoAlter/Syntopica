# Design：磁铁信号口径反转 + 核心主体保护 + 分诊合理性闸门

> 复杂度 complex（算法口径变更 + 多模块：后端统计/提示词/闸门 + 前端展示）。
> 方案选型已由用户拍板（口径方向、修复范围、旧快照处置、描述补齐四问），本文档落点为实现契约。

## 背景事实（真库只读核对，2026-11-09 快照）

| 事实 | 值 | 取证 |
| --- | --- | --- |
| 板块 | 1974「中东地缘政治与美伊关系」 | `semantic_labels.id=1974` |
| 构成标签 | 47 个（全部 active） | `board_composition` ⋈ `semantic_labels` |
| 板块 description | **空** | `semantic_labels.description` |
| 最新快照 | id=410，checked_at=2026-11-09 21:23，llm_score=58 | `board_health_snapshots` |
| 该快照分诊 | rewrite 47 / keep 0 / remove 0 | `metrics->'triage'` |
| 磁铁嫌疑成立数 | **41 / 47**（平均 gap 0.1062，阈值 0.08） | `metrics->'labels'` |
| 伊朗（aux 262） | all_mean 0.6661 / failed_mean 0.5424 / all_samples 303 / failed_samples 19 / suspicious=true / cosine_mean 0.6661 | 同上 |
| 历史翻转 | 快照 357（11-06 10:14）= 13 keep + 34 rewrite；快照 370（11-06 21:03）与 410 = 47 rewrite | 同上 |
| 已知锚点 | 「德黑兰/伊朗 = keep」 | `openspec/changes/archive/2026-11-06-board-health-triage-advice/tasks.md:42` |

## 方案对比（磁铁口径方向，选型已确认）

| 方案 | 公式 | 优点 | 缺点 | 结论 |
| --- | --- | --- | --- | --- |
| A（现状） | `suspicious = all_mean − failed_mean ≥ gap` | 无 | 方向与语义相反；核心主体 gap 最大 → 反向偏置；`all_mean ≡ cosine_mean` 一数两义 | 弃 |
| **B（选定）** | `suspicious = failed_mean − good_mean ≥ gap`（good = 方向通过的挂载子集） | 语义对齐「磁铁把跑题的吸过来」；顺带拆掉「全体均值含失败行」造成的自我稀释 | 需新增 good 侧统计；good 样本不足时一律 insufficient | **用户选定（2026-11-09 决策）** |
| C | 保留 A 口径，改判板内相对分位 | 改动最小 | 仍是反的语义，只把「41/47 全中」换成「另一个数字」；阈值仍随数据漂移 | 弃 |
| D | B ∩ C（双口径取交集） | 最保守 | 召回过低，弱板块的真污染信号也被抹掉 | 弃 |

用户确认记录：`ask_user_question` → 「失败子集 vs 健康子集（推荐）」。

## Decisions

### D1 磁铁信号改按「失败子集 vs 健康子集」判定，方向反转

- **落点**：`backend-go/internal/tagmanagement/service/boardhealth/signals.go:computeMagnetSignals`（重写分组与判定）；`backend-go/internal/tagmanagement/service/boardhealth/metrics.go`（`matchedRow` 已带 `DirectionMismatch`，分组在同一处切分即可，无需新查询）
- **取舍**：健康子集 = `direction_mismatch=false` 且有 tag identity embedding 的挂载行；失败子集 = `direction_mismatch=true` 且有向量的行；`suspicious = failed_mean − good_mean ≥ config.MagnetGap`；任一侧样本不足（good < `MinSampleSize`、failed < `MagnetMinFailed`）或标签自身向量缺失 → `insufficient`，不硬凑
- 理由：现状把「failed 比全体低」判成嫌疑，等于「与跑题挂载越不像就越有嫌疑」，与磁铁语义相反，且导致核心主体被系统性点名（41/47）

### D2 `all_mean/all_samples` 保留但退出嫌疑判定

- **落点**：`backend-go/internal/tagmanagement/models/board_health.go:BoardHealthMagnetSignal`（保留字段 + 新增 `GoodMean`/`GoodSamples`）；`signals.go:computeMagnetSignals`（继续计算 all 侧，仅作展示/历史兼容）
- **取舍**：旧快照缺 `good_mean` 时前端回退旧口径并标注「旧口径快照」；`all_mean ≡ cosine_mean`（同集合同算法）在代码注释里写明，避免下一个维护者再把两者当独立证据
- 理由：`all_mean` 与 `cosine_mean` 恒等是「一个数字两个含义」的认知陷阱，但直接删字段会破坏历史快照解析与既有 API 消费者，保留 + 退出判定是成本最低的收敛

### D3 核心主体判定：描述/别名命中即成立；仅标题命中需标签名 ≥ 3 字

- **落点**：`backend-go/internal/tagmanagement/service/boardhealth/metrics.go`（新增 `computeCoreSubjects(board *models.SemanticLabel, labels []tagmodels.BoardHealthLabelMetrics)`），`run.go:collectBoardStatistics` 传入 `board`（当前签名只有 `boardID`，需改签名或在调用处补传 board 指针）
- **取舍**：纯文本字面匹配（空白与大小写归一后子串）。命中 `board.Description` 或 `board.Aliases` → 核心主体成立；**仅命中 `board.Label`（标题）** → 还需该标签名长度 ≥ 3 字才成立（2 字短词既可能是简称也可能是泛化地域词）；不做向量相似度或语义推断
- 理由：① 纯字面匹配可核、可解释、不臆造；② 「标题命中 ≥3 字」这一约束堵住误命中（标题含「中东」而构成标签名为「中东」时，不应把泛化地域词当核心主体，否则 G1 会把它强制 keep、屏蔽真污染）；③ **重要边界**：板块 1974 的标题「中东地缘政治与美伊关系」用的是简称「美伊」，**标题并不含「伊朗」**，因此该板的核心主体保护**依赖 description 明示「伊朗」**（见 D10）——简称 ≠ 全名，系统不做推断
- 修正记录（2026-11-10 review R-3/R-6）：本条原理由行写「标题直接含『伊朗』→ 命中」是事实错误（标题含的是简称「美伊」），且原表述「零误判」不成立（纯子串只是换了一种误伤形态）；现按上面取舍行定稿

### D4 核心主体被 LLM 判 remove/rewrite 时，规则层改判 keep（而非 insufficient）

- **落点**：`backend-go/internal/tagmanagement/service/boardhealth/llm.go:applyTriageGuards`（G1 分支，核心主体拦截与摘除证据校验、占比闸门同函数内顺序执行；不单列 `applyCoreSubjectGuard`）
- **取舍**：改判为 `keep`，evidence 重写为「核心主体保护：该标签出现在板块描述/别名中，不作为摘除或重写对象（原判：<原 evidence>）」；原判 evidence 保留在括号内可追溯；`suggestion` 清空（keep 不携带建议文案）
- 理由：规则层拦截是**唯一不依赖 LLM 听话**的防线（提示词层已被实证会翻转：同一证据 11-06 判 keep、11-09 判 rewrite）；改判 keep 而非 insufficient，是因为证据其实是充分的——只是「不允许动核心主体」，keep 是准确结论，insufficient 会让下轮再来一遍同样的拦截

### D5 `remove` 必须有成立的磁铁信号，否则降级为证据不足（G2）

- **落点**：`backend-go/internal/tagmanagement/service/boardhealth/llm.go`（`applyTriageGuards` 内第二道闸门：逐条查该 aux_id 对应标签的 `magnet.suspicious`；false 或 `insufficient` → category 改 `insufficient`，evidence 追加降级原因）
- **取舍**：只对 `remove` 做硬校验，`rewrite` 不做同等硬校验（rewrite 的证据形态多样：desc_flags 命中、近邻窄化，规则层难以可靠机器化）
- 理由：remove 是破坏性建议（摘掉构成标签），必须证据充分；rewrite 是可逆的（用户要手工核对后才进修复任务），门槛可低一档
- **效果核对注记（2026-11-10 重跑后，R-14）**：新口径下真库 13 个板的 `suspicious=true` 标签数 **全部为 0**——失败挂载按定义与本板方向不符，其贴合度普遍**低于**健康挂载，因此「失败侧显著高于健康侧」极难成立。**后果：「🔴 建议摘除」在真实数据上事实上不可达**，真污染只能走 rewrite（描述漂移）或人工判断；本闸门不是「让体检更准」，而是「把误伤摘除挡在门外」。若后续要恢复摘除通道，需另开 change 重新定义磁铁证据口径（例如以「失败挂载相对该标签在本板外的有效挂载」作对照），不得在本闸门上调阈值硬凑
- **后续方向（不在本 change 范围）**：① 解析层对 LLM 输出的数字字段做宽容解析（`aux_id` 被输出成字符串导致整轮降级，本轮 3/13 板触发，历史轮次同样存在）；② rewrite 证据的机器化校验

### D6 可执行占比超限时，本轮全部可执行项降级为证据不足（G3），阈值 0.8 可配

- **落点**：`backend-go/internal/tagmanagement/service/boardhealth/boardhealth.go`（新增 `ai_settings` 键 `board_health_max_actionable_ratio`，默认 `DefaultMaxActionableRatio = 0.8`，`Config.MaxActionableRatio` 字段）；`llm.go:applyTriageGuards`（第三道闸门）
- **取舍**：占比 = `(remove + rewrite) / triage 总数`，**取 LLM 原始裁决**（G1/G2 之前统计，否则占比随前序闸门漂移、闸门记录失真）；超阈值时**逐条**把 remove/rewrite 降为 `insufficient`，evidence 追加「可执行占比 N/M（X%）超过阈值 Y，本轮信号不可用」；快照级记录 `triage_guard = {actionable, total, ratio, threshold, degraded, reason}`；**不**整轮置 `triage=null`（保留证据展示，只是不给可执行入口）
- 理由：阈值 0.8 的取值依据——已知**正确**轮次的可执行占比最高到 72%（快照 357：34 rewrite / 47），取 0.8 留出余量避免误杀好轮次；坏轮次是 100%（快照 370 / 410），必被拦。整轮置空会连证据与评语一起丢，用户的「47 项待核对」会变成「无结论」，无法自证；降级为证据不足则索引里不再出现可执行项，提示条说明原因

### D7 提示词加四条硬约束（LLM 层，与规则层互补而非替代）

- **落点**：`backend-go/internal/tagmanagement/service/boardhealth/llm.go:boardHealthSystemPrompt`（分诊判据段）+ `buildScorePrompt`（证据块追加 `core_subject` 标记与"本板无描述"声明）+ `renderEvidenceBlock`
- **取舍**：四条为：① `core_subject=true` 的标签 MUST NOT 判 remove/rewrite；② 实体类标签（国家/地区/城市/机构/人物）的同类最近邻 MUST NOT 记为漂移/窄化证据；③ 板级共享失败子集与 `magnet_flow` MUST NOT 当作单标签证据（只可佐证）；④ 版块 description 为空时显式告知模型「本板无描述文本，核心主体只能依据 core_subject 标记判定」
- 理由：规则层（D4/D5/D6）能兜住误伤，但 LLM 少判/多判仍会拖低体检有用性；提示词层把判据讲清楚能减少无谓的 rewrite 建议量（降低闸门被触发的概率）

### D8 闸门降级项落 `insufficient`，前端自动退出「待核对」索引

- **落点**：后端 `llm.go:applyTriageGuards`；前端复用既有 `front/app/features/tags/components/console/board-health/boardHealthWorkspace.ts:splitTriageItems`（`remove|rewrite` → 待核对；`keep|insufficient` → 其余标签折叠区）
- **取舍**：不新增前端过滤逻辑、不新增"降级"分类（闭集仍四值），避免分类膨胀与消费方兼容成本
- 理由：既有 spec 已规定「补齐项 MUST NOT 被猜成可执行建议，界面 MUST NOT 为其提供摘除/重写入口」——复用同一条契约，前端零改动即获得正确行为

### D9 快照结构新增字段全部落在既有 jsonb 内，无表结构变更

- **落点**：`backend-go/internal/tagmanagement/models/board_health.go`（`BoardHealthMagnetSignal.GoodMean/GoodSamples`、`BoardHealthLabelMetrics.CoreSubject`、`BoardHealthMetrics.TriageGuard`）
- **取舍**：`triage_guard` 挂在 `BoardHealthMetrics` 顶层（板级信号，板级维度）；不做独立表、不加迁移
- 理由：体检快照本就是 jsonb 载荷（`BoardHealthMetrics` 已含 board/labels/triage 三层），新增字段是既有模式的自然延伸；独立表会带来迁移 + 兼容成本，收益为零

### D10 补板块 1974 的 description；重跑一轮体检；历史脏快照保留

- **落点**：数据面 —— `semantic_labels.description`（id=1974）补定义式描述（点明伊朗/美伊关系为本板核心主体、覆盖范围为中东地缘政治）；重跑走既有 CLI `go run ./cmd/admin board-checkup`（全量一轮，约 14 分钟，会给全部活跃板写新快照——这是体检的正常行为，体检本就是每日全量）
- **取舍**：历史快照 370 / 410（47/47 的两轮）**保留不删**（用户决策），保留历史 = 趋势序列里能看到信号翻转；不改写历史（spec 明确「历史快照 MUST NOT 被覆盖或修改」）
- 理由：description 补齐能直接给 LLM 文本依据，但**不能替代**代码层修复——description 为空的板块仍会出现，必须靠 D1/D3/D4 兜住

### D11 「证据结构化校验」不在本次范围

- **落点**：无代码落点：承接制品 = `openspec/changes/fix-board-health-magnet-signal/proposal.md` 的「### Out of Scope」节，因为结构化证据字段会改 LLM 输出契约与消费方解析，属独立重构
- **取舍**：本次只加 G2（remove 必须磁铁成立）+ G3（占比闸门）两道可机器化校验；rewrite 的证据质量校验留给后续 change
- 理由：避免一次改太多契约面；G2/G3 已能拦住本次事故的两种表现形态

### D12 磁铁口径显式标记（解决「旧快照」与「新快照但健康侧不可算」不可区分）

- **落点**：`backend-go/internal/tagmanagement/models/board_health.go`（`BoardHealthMagnetSignal` 增加 `Caliber string json:"caliber"`）、`backend-go/internal/tagmanagement/service/boardhealth/signals.go`（写值）、`front/app/api/semanticBoards.ts` + `front/app/api/normalizers/boardHealthTriage.ts`（搬运）、`front/app/features/tags/components/console/board-health/boardHealthWorkspace.ts`（派生文案）
- **取舍**：`caliber` 取值 `"healthy_subset"`（新口径）/ 空串（历史快照或未写入 = 旧口径）；前端按 `caliber` 决定标注「旧口径快照」还是「健康子集不可算（样本不足/向量缺失）」，**不再**拿 `goodMean === null` 当旧快照的判据（`good_mean` 无 `omitempty`，新快照在健康侧不可算时同样序列化为 null，两者不可区分）
- **理由**：review R-4 指出前端口径回退条件过宽，会把新快照误标为旧口径、还把含失败行的全体均值摆回界面（正是本 change 判为语义相反的旧口径）；显式标记是唯一能区分两者的办法，且不引入新表

## 实现顺序与依赖

1. 后端模型字段（D9/D12）→ 2. 磁铁口径（D1/D2）→ 3. 核心主体标记（D3）→ 4. 规则层闸门（D4/D5/D6）→ 5. 提示词（D7）→ 6. 前端字段与展示（D9/D12 前端侧 + ui-design 三处变化）→ 7. 数据补齐与重跑（D10）

后端 1-5 依赖同一个 `BoardHealthLabelMetrics` 结构变更，宜同一子线程串行完成；前端 6 与后端 1-5 无文件冲突，可并行（但接口字段名需先在 specs delta 固定，本 design 已固定）。
