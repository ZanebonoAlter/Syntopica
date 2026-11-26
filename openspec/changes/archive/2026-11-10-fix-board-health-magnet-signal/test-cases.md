# Test Cases：磁铁信号口径反转 + 核心主体保护 + 分诊合理性闸门

> 单元 = 一个 Requirement 的用户故事；本文档串起完整故事（spec Scenario 只是断言片段）。
> 层次：后端纯逻辑 → Go 函数单测（`boardhealth` 包）；前端派生/归一化/组件 → Vitest；真库效果 → 只读 SQL + 重跑一轮。
> 关键约束：体检轮次**纯读**，只有 `board_health_snapshots` 写入；真库核对一律 **只读 SELECT**，有副作用的写动作（重跑体检、补 description）单独授权。

## 1. 单元与故事边界

| 单元（Requirement） | 一句话故事 | 主角层 |
| --- | --- | --- |
| 标签分诊信号计算 | 「磁铁信号要抓的是把跑题挂载吸过来的标签；核心主体要被打上标记」 | Go 函数单测（`metrics_test.go`） |
| LLM 综合评分与降级 | 「提示词把三条新判据讲清楚，模型少乱判」 | Go 断言单测（`llm_test.go` 提示词锚点） |
| 分诊合理性闸门 | 「模型判的东西要先过三道闸门才能给用户当建议」 | Go 函数单测（`llm_test.go` 闸门函数） |
| 分诊以问题处理为主线 | 「用户看到的待核对列表要诚实：口径对、核心主体有标记、信号不可用时有提示」 | Vitest（normalizer / model / 组件）+ 人工页面核对 |

## 2. ⓪ 改契约了吗（继承与调整）

`bash scripts/harness/test-assets.sh board-health-checkup` 反查旧资产（主 spec 54 Scenario / 104 条历史映射）。本 change **改了磁铁信号契约**（方向反转 + 新增 core_subject + 新增闸门），旧断言磁铁口径的用例必须逐条处置：

| 旧 Scenario / 旧断言 | 处置 | 旧测试文件 | 动作 |
| --- | --- | --- | --- |
| 磁铁信号失败子集贴合差成立（旧：failed 0.52 < 全体 0.68 → 成立） | **改写**（语义反转：failed 高于健康子集才成立） | `backend-go/internal/tagmanagement/service/boardhealth/metrics_test.go:220` `TestComputeMagnetSignalsSuspicious` | 改构造：failed 向量与标签同向、good 向量正交；断言 `failed_mean > good_mean` 且 suspicious=true |
| 旧 gap 边界用例（gap = AllMean − FailedMean） | **改写**（新 gap = FailedMean − GoodMean） | `metrics_test.go:240` `TestComputeMagnetSignalsGapBoundary` | 改公式与构造；保留「== 阈值即成立（>=）」边界断言 |
| 失败子集样本不足不强判 | **继承 + 扩充** | `metrics_test.go:274` `TestComputeMagnetSignalsFailedSamplesBoundary` | 保留失败侧不足用例；新增「健康侧不足 → insufficient」用例 |
| 向量缺失信号不硬算 | **继承** | `metrics_test.go:294` `TestComputeMagnetSignalsInsufficientScenarios` | 不动 |
| 磁铁标签凭失败子集证据判摘（旧数值 0.52 vs 0.68） | **改写**（数值方向） | `backend-go/internal/tagmanagement/service/boardhealth/llm_test.go` | 改 fixture 数值与断言语义 |
| 短实体标签不因低 cosine 被判摘除 | **继承** | `llm_test.go` | 不动 |
| 证据不足标签归入下轮再判 | **继承** | `llm_test.go` | 不动 |
| 组合标签的分诊项只读 | **继承** | `front/app/api/normalizers/boardHealthTriage.test.ts`、`front/app/features/tags/components/console/board-health/boardHealthWorkspace.model.test.ts` | 不动 |
| 先处理问题而非遍历四组卡片 / 修复完成不冒充体检验证 | **继承** | `front/app/features/tags/components/console/board-health/BoardHealthWorkspace.test.ts` | 不动 |
| 历史快照缺分诊字段兼容 | **继承 + 扩充** | `front/app/api/normalizers/boardHealthTriage.test.ts`、`BoardHealthWorkspace.test.ts` | 新增「旧快照缺 good_mean/core_subject/triage_guard」的容错用例 |
| 磁铁 fixture（`all_mean: 0.68, failed_mean: 0.52`） | **继承**（字段仍在）+ 新增 good 侧字段 | `front/app/api/normalizers/boardHealthTriage.test.ts:245,263`、`BoardHealthWorkspace.test.ts:78` | fixture 追加 `good_mean` / `good_samples` / `caliber`，旧字段保留 |
| 旧快照磁铁结论（`suspicious` 旧公式语义） | **继承 + 扩充** | `front/app/features/tags/components/console/board-health/BoardHealthWorkspace.test.ts` | 新增「旧口径快照 verdict 文案带口径标记」用例（R-13） |

## 3. 主链路表（节拍）

| 步 | 动作 | 来源 Scenario | 期望 | 层 | 落点（文件） |
| --- | --- | --- | --- | --- | --- |
| 1 | 构造一块板：284 个方向通过的挂载行 + 19 个 `direction_mismatch=true` 失败行；标签「伊朗」向量与健康行同向、与失败行反向 | 磁铁信号按失败子集相对健康子集判定 | `failed_mean < good_mean` → `suspicious=false`；两值与样本数写入信号 | Go 单测 | `backend-go/internal/tagmanagement/service/boardhealth/metrics_test.go` |
| 2 | 同一块板，把失败行向量改为与标签同向 | 磁铁信号按失败子集相对健康子集判定 | `failed_mean − good_mean ≥ 0.08` → `suspicious=true` | Go 单测 | 同上 |
| 3 | 板 description 含「伊朗为核心主体」，构成含「伊朗」，标题为「中东地缘政治与美伊关系」（简称） | 核心主体按版块文本判定 | 「伊朗」`core_subject=true`；「卡塔尔」`core_subject=false` | Go 单测 | `metrics_test.go` |
| 4 | 构造 LLM 分诊：核心主体被判 `rewrite` + 建议文案 | 核心主体被判可执行时规则层改判保留 | 持久化前改判 `keep`，evidence 含「核心主体保护」+ 原判，建议文案清空 | Go 单测 | `backend-go/internal/tagmanagement/service/boardhealth/llm_test.go` |
| 5 | 构造 LLM 分诊：某标签判 `remove`，但其磁铁信号不成立 | 摘除建议必须有成立的磁铁信号 | 降级 `insufficient` + 注明原因，不呈现为摘除建议 | Go 单测 | `llm_test.go` |
| 6 | 构造 47 条全 `rewrite` 的分诊 | 可执行占比超限触发整轮降级 | 全部降级 `insufficient`；闸门记录占比 1.0 / 阈值 0.8 / degraded=true / 原因 | Go 单测 | `llm_test.go` |
| 7 | 构造 13 keep + 34 rewrite（占比 72%） | 占比未超限不降级 | 原样保留，无闸门降级记录 | Go 单测 | `llm_test.go` |
| 8 | 跑一轮体检（fake LLM），检查落库快照 | 闸门不影响评分与评语 | 快照含 `triage_guard`；评分/评语保留 | Go 单测 | `backend-go/internal/tagmanagement/service/boardhealth/run_test.go` |
| 9 | 前端读含 `good_mean` 的快照 | 磁铁对比使用健康子集口径 | 详情区渲染「失败子集 X vs 健康子集 Y」，不出现全体均值对比 | Vitest | `front/app/features/tags/components/console/board-health/BoardHealthWorkspace.test.ts` |
| 10 | 前端读旧快照（缺 `good_mean`） | 旧快照回退旧口径并标注 | 回退旧对比 + 显示「旧口径快照」标注 | Vitest | 同上 + `front/app/api/normalizers/boardHealthTriage.test.ts` |
| 11 | 前端读含 `core_subject` 与 `triage_guard` 的快照 | 核心主体标记可见 / 闸门降级时提示信号不可用 | 索引与详情出现核心主体标记；顶部出现警示条；被降级项不在待核对索引 | Vitest | `BoardHealthWorkspace.test.ts` + `boardHealthWorkspace.model.test.ts` |
| 12 | 真实板块重跑后打开体检抽屉 | （人工核对，见 §6） | 「伊朗」不在待核对首项；待核对条数回落 | 人工 | 静态 :5100 页面 |

## 4. 变体走查（五组固定清单）

### 4.1 输入

| 变体 | 答案 | 落点 |
| --- | --- | --- |
| 标签名为空串 | 不可能（构成标签名非空）；防御：空名不参与核心主体匹配，不 panic | `metrics_test.go` |
| 纯空白 / 全角空格 | 归一化后为空 → 不匹配任何版块文本 → `core_subject=false` | `metrics_test.go` |
| 纯分隔符（`-` / `/`） | 同上，`core_subject=false` | `metrics_test.go` |
| 单 token（如「伊朗」） | 出现在 description/aliases → 命中；仅出现在标题且仅 2 字 → 不命中（简称/泛化词不据标题推断） | `metrics_test.go` |
| 大小写差异（IRAN vs 伊朗） | 中文标签大小写无意义；英文标签按 casefold 归一后匹配 | `metrics_test.go` |
| 特殊字符（`伊朗（伊斯兰共和国）`） | 按字面子串匹配，不做括号剥离；命中与否以版块文本为准 | `metrics_test.go` |
| 短泛化词命中标题（如标题含「中东」、标签名为「中东」） | 不算核心主体（仅标题命中需标签名 ≥ 3 字），MUST NOT 被 G1 强制 keep | `metrics_test.go` |
| 标题命中但标签名 ≥ 3 字（如标题含「中东地缘政治」、标签名「中东地缘政治」） | 算核心主体 | `metrics_test.go` |
| 超长版块描述 | `strings.Contains` 无长度限制；性能不构成问题（单板 47 标签） | — |

### 4.2 前置

| 变体 | 答案 | 落点 |
| --- | --- | --- |
| 空集（板无挂载行） | 两侧样本 0 → 信号 `insufficient`，不判嫌疑 | `metrics_test.go` |
| 单元素（仅 1 个挂载行） | 样本不足 → `insufficient` | `metrics_test.go` |
| 重复（同一 tag 多次挂载） | 按行计数，不去重（沿用既有口径） | 人工核对（真库数据即有重复行） |
| 越界引用（triage 里出现未知 aux_id） | 既有 `normalizeTriage` 判非法 → 整轮降级（沿用现行契约，本 change 不改） | `llm_test.go`（继承既有用例） |
| 部分满足（failed 达标、good 不达标） | `insufficient`（两侧各自达标才判） | `metrics_test.go` |

### 4.3 时间窗口

| 变体 | 答案 | 落点 |
| --- | --- | --- |
| 边界两端 | 本 change 无时间窗口语义；体检快照时间戳沿用既有逻辑 | — |
| 空窗口 / 跨窗口 | 不适用（划除：本 change 不引入时间窗） | — |
| 归一化 | 不适用（划除） | — |

### 4.4 幂等

| 变体 | 答案 | 落点 |
| --- | --- | --- |
| 重复执行 | 闸门是纯函数（输入同一分诊清单 + 同一信号 → 同一输出），重复调用结果一致 | `llm_test.go` |
| 部分失败重试 | LLM 调用失败 → 整轮降级（triage=NULL），闸门不介入 | 继承 `llm_test.go` 既有降级用例 |
| 并发 | 本 change 不声称线程安全；体检轮次本身按板串行 | — |

### 4.5 可用性（UI 必检前三 + 其余）

| 变体 | 答案 | 落点 |
| --- | --- | --- |
| 误输入反馈 | UI 不接受用户输入（体检数据只读展示），不适用（划除） | — |
| 空态 | 无快照 / 无 triage → 既有空态展示不变 | 继承 `BoardHealthWorkspace.test.ts` 空态用例 |
| 错误态 | 体检端点失败 → 既有错误展示不变 | 继承既有用例 |
| 加载态 | 快照加载中 → 既有骨架不变 | 继承既有用例 |
| 超长文本 | 核心主体徽标 + 警示条为短文本；长评语沿用既有截断策略 | `BoardHealthWorkspace.test.ts`（断言徽标/提示条存在，不做像素断言） |
| 重复提交 | UI 不适用（划除：无提交动作） | — |
| 闸门提示条可读性 | 提示条文案含「本轮信号可能不可用、不得按此批量操作」，机械断言文案存在 | `BoardHealthWorkspace.test.ts` |

## 5. 效果核对（真库量化）

> 效果依赖断言外因素（真实挂载分布 + LLM 行为）→ 必须真库量化核对。核对一律**只读 SELECT**；写动作（重跑体检、补 description）单独授权。

| 项 | 内容 |
| --- | --- |
| 触发原因 | 修复前快照 410（2026-11-09 21:23）：伊朗 `suspicious=true`（failed 0.5424 / all 0.6661，gap 0.124），全板 41/47 成立，47/47 rewrite，伊朗被判待核对，板分 58 |
| 方法 | 修复后执行 `cd backend-go && go run ./cmd/admin board-checkup`（全量一轮，13 板，10m15s，写新快照）；再对 1974 的**最新快照**（id=414，2026-11-10 12:29）只读查询：`metrics->'triage'` 分类分布、`metrics->'labels'` 里 aux 262 的 magnet 与 core_subject、`metrics->'triage_guard'`、全板 suspicious 计数 |
| 量化结果 | ① 伊朗 `suspicious=false`（good_mean 0.6743 / failed_mean 0.5424 / `caliber=healthy_subset`）✅；② 伊朗 `core_subject=true`（description 命中）且 triage 类别 = `keep` ✅；③ 全板 47 条分诊 **全部 keep，可执行 0 条**（阈值 0.8，闸门 `degraded=false`、`actionable=0`）——待核对列表从 47 项降到 0 项 ✅；④ 板分 **58 → 88** ✅；⑤ 全板 8 个标签被标核心主体（伊朗/卡塔尔/以色列/霍尔木兹海峡/中东/中东地区/沙特/沙特阿拉伯——均由补写的 description 点名命中）；⑥ R-14 量化：本轮 13 个板 `suspicious=true` 的标签数 **全部为 0** |
| 结论 | 主目标达成（伊朗不再进待核对、信号方向反转后核心主体不再被点名）。但暴露两个**残余问题**（均不属本 change 范围，已如实记录）：① **R-14 成真**——新口径下 `failed_mean` 普遍低于 `good_mean`，磁铁信号在真实数据上几乎不成立，「🔴 建议摘除」事实上成为不可达类别，真污染只能走 rewrite 或人工判断（见 design D5 注记）；② 本轮 3 板（生成式 AI / 刚果疫情 / 游戏版块）降级，原因均为 **LLM 把 `aux_id` 输出成字符串**导致 JSON 解析失败（既有解析脆弱点，本 change 未改解析层；历史对照：11-09 那轮同样降级 5/12 板，属既有常态非本 change 引入） |

## 6. 展示字段盘点（改数据结构时）

| 字段 | 用户可见 | 来源语义 | spec 锚 |
| --- | --- | --- | --- |
| `magnet.good_mean` / `magnet.good_samples` | 是（详情证据行） | 健康挂载子集（`direction_mismatch=false`）的贴合均值与样本数 | MODIFIED「标签分诊信号计算」 |
| `magnet.caliber` | 是（口径标注） | 磁铁口径标记：`healthy_subset` = 新口径；空 = 旧口径快照 | MODIFIED「标签分诊信号计算」+ MODIFIED「分诊以问题处理为主线」 |
| `magnet.all_mean` / `magnet.all_samples` | 是（仅旧快照回退时） | 全部挂载子集的贴合均值（**含**失败行），历史兼容 | MODIFIED「标签分诊信号计算」+ MODIFIED「分诊以问题处理为主线」 |
| `labels[].core_subject` | 是（徽标） | 标签名出现在版块标题/描述/别名中 | MODIFIED「标签分诊信号计算」 |
| `metrics.triage_guard` | 是（警示条） | 闸门结论：可执行条数/总数/占比/阈值/是否降级/原因 | ADDED「分诊合理性闸门」 |
| `triage[].category` | 是（索引分组） | 四值闭集，本 change 新增「闸门降级落 insufficient」路径 | ADDED「分诊合理性闸门」 |
| `triage[].evidence` | 是（证据摘要） | 规则层降级原因追加在证据内（核心主体保护 / 磁铁不成立 / 占比超限） | ADDED「分诊合理性闸门」 |

> 无锚即盲区：上表所有字段均在 specs delta 中有 Requirement 锚；`triage_guard` 为**新字段**，已先补 ADDED Requirement 再改代码（⓪ 纪律）。

## 7. 白盒附加（complex 档）

### 7.1 分支表 —— `computeMagnetSignals`（新口径）

| 分支 | 条件 | 期望 |
| --- | --- | --- |
| B1 | 标签自身向量缺失 | `insufficient=true`、`suspicious=false`、两侧均值为 nil |
| B2 | 健康侧样本 < `MinSampleSize` | `insufficient=true`、`suspicious=false` |
| B3 | 失败侧样本 < `MagnetMinFailed` | `insufficient=true`、`suspicious=false` |
| B4 | 两侧达标，`failed_mean − good_mean ≥ MagnetGap` | `suspicious=true` |
| B5 | 两侧达标，`failed_mean − good_mean == MagnetGap` | `suspicious=true`（>= 语义，与旧口径一致） |
| B6 | 两侧达标，`failed_mean − good_mean < MagnetGap` | `suspicious=false` |
| B7 | 失败侧均值**低于**健康侧（核心主体典型形态） | `suspicious=false`（B6 的特例，防回归锚） |
| B8 | 某挂载行无 tag 向量 | 该行不进任一侧（不按 0 计入），两侧样本数相应减少 |
| B9 | 全部挂载行都无向量 | 两侧样本 0 → `insufficient=true` |

### 7.2 分支表 —— `computeCoreSubjects`（核心主体判定）

| 分支 | 条件 | 期望 |
| --- | --- | --- |
| C1 | 标签名出现在 board.Description | `CoreSubject=true` |
| C2 | 标签名出现在 board.Aliases 任一项 | `CoreSubject=true` |
| C3 | 标签名仅出现在 board.Label（标题）且长度 ≥ 3 | `CoreSubject=true` |
| C4 | 标签名仅出现在 board.Label（标题）且长度 = 2 | `CoreSubject=false`（简称/泛化词不据标题推断） |
| C5 | 标签名在三个文本中均不出现 | `CoreSubject=false` |
| C6 | 标签名含大小写/空白差异（`IRAN` vs `iran`、` 伊朗 `） | 归一化后命中（C1/C2 口径） |
| C7 | board 为空指针或三个文本全空 | `CoreSubject=false`，不 panic |
| C8 | 标签名为空串 | `CoreSubject=false`，不 panic |

### 7.3 分支表 —— `applyTriageGuards`（三道闸门）

| 闸门 | 分支 | 期望 |
| --- | --- | --- |
| G1 核心主体 | 核心主体 + `rewrite` | → `keep`；evidence 含「核心主体保护」+ 原判；suggestion 清空 |
| G1 | 核心主体 + `remove` | → `keep`；同上 |
| G1 | 核心主体 + `keep`/`insufficient` | 不动 |
| G1 | 非核心主体 + 任意类别 | 不动 |
| G2 摘除证据 | `remove` 且磁铁成立 | 保持 `remove` |
| G2 | `remove` 且磁铁不成立 | → `insufficient` + 原因 |
| G2 | `remove` 且磁铁 `insufficient` | → `insufficient` + 原因 |
| G2 | 非 `remove` 类别 | 不动 |
| G3 占比闸门 | 占比 ≤ 阈值 | 全部不动，`triage_guard.degraded=false`（或不写该字段） |
| G3 | 占比 > 阈值 | 全部 `remove`/`rewrite` → `insufficient` + 原因；`triage_guard.degraded=true` + 占比 + 阈值 |
| G3 | 分诊清单为空 | 不触发（无分诊可降级），不写闸门记录 |
| G3 | 占比取自 **LLM 原始裁决**（G1/G2 之前统计） | 前序闸门降级不影响记录的占比（remove 主导轮次仍记原始 1.0） |

### 7.4 边界值

| 量 | 边界 | 值 |
| --- | --- | --- |
| `MagnetGap` | 恰好等于阈值 | 成立（>=） |
| 健康侧样本 | `MinSampleSize - 1` / `MinSampleSize` | 不足 / 达标 |
| 失败侧样本 | `MagnetMinFailed - 1` / `MagnetMinFailed` | 不足 / 达标 |
| 可执行占比 | 阈值恰好相等 | **不降级**（超限才降，`>` 语义） |
| 可执行占比 | 47/47 | 1.0 > 0.8 → 降级 |
| 占比闸门阈值 | 0.8 配置为 1.0 | 占比 1.0 时不超限（1.0 > 1.0 为假）→ 不降级（边界自洽，配置项可调） |

### 7.5 不适用划除留痕

- 时间窗口类变体：本 change 无时间窗语义（划除，理由见 §4.3）；
- 并发安全：本 change 不声称线程安全，体检按板串行（划除）；
- 用户输入校验：体检面板为只读展示，无输入通道（划除）；
- 快照并发写：沿用既有体检轮次串行语义（划除）。

## 8. 验收措辞（三形态）

- 命令 + 期望：`cd backend-go && go test ./internal/tagmanagement/service/boardhealth → PASS`；
- 文件存在：`openspec/changes/fix-board-health-magnet-signal/review.md` 存在且含 R-N 条目；
- 人工：`人工：静态 :5100 打开板块「中东地缘政治与美伊关系」体检抽屉，「体验核查」索引首项不再是「伊朗」，且出现/不出现闸门提示条与实际闸门结论一致`。
