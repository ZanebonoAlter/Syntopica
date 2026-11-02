## Context

现状：`boardhealth` 包（口径唯一实现，红线 22）每轮产出两级统计 + LLM 单调用裁决 `{score, comment, notify}`，快照 `board_health_snapshots` 持久化 metrics jsonb（`{board, labels[]}`，labels 含 cosine 分布与 insufficient）+ uncaptured_candidates。`matchedRow` 已携带 `direction_mismatch`（metrics.go:42），贴合度分布的向量和挂载行在统计轮内已全部在内存——三病信号有现成数据基础。摘除接口已存在（`DELETE /api/semantic-boards/:id/composition/:auxiliary_label_id`，board_crud_handler.go:155）。前端右轨体检卡（`RailHealthBrief.vue` brief/full + `BoardHealthDetail.vue`）经 `consoleRail.ts` 归一消费。

调研实证（docs/research/board-matching-evaluation/explore-findings.md）：磁铁污染（全球科技巨头 max_sim 方向失败率 46.9%）、短名天花板（OpenAI cos=0.57 挂载全对）、description 污染（「上海」desc="上海知名商业综合体"→最近邻「举办地点」0.796）三病可鉴别，且当前 LLM 评语无动作输出。

## Goals / Non-Goals

**Goals:**
- 三病信号确定性计算进 metrics（磁铁/最近邻/描述预筛），作为 LLM 分诊的硬证据
- LLM 单调用输出扩展为 `{score, comment, notify, triage[]}`，分诊四分类逐构成标签
- 快照/端点/前端右轨卡全链路携带分诊清单，摘除动作复用既有接口
- 历史快照向后兼容（无 triage 字段按旧形态展示）

**Non-Goals:**
- 全库自动描述清洗与提取提示词源头治理（仍属后续 change）；用户确认的单标签描述修复已按 FB-2 纳入本 change
- 方向校验短名分层阈值（后续 change，依赖本 change 攒「确认保留」数据）
- 匹配算法任何改动（四规则、方向门、豁免带零改动）

## Decisions

### D1. 三病信号在 boardhealth 包内、贴合度统计同轮同内存计算，不新增查询round-trip

- **结论**：磁铁信号复用 `collectBoardStatistics` 已加载的 `matchedRows`（含 direction_mismatch）与 `tagEmbeddings`，按失败子集/全体分组算每标签两侧贴合均值；最近邻信号用一条 pgvector SQL（`ORDER BY embedding <=> ? LIMIT 3`）逐构成标签查；描述预筛纯字符串正则。全部落 `BoardHealthMetrics.Labels[]` 扩展字段（`magnet`, `neighbors`, `desc_flags`），不新建表、不加 DDL。
- **落点**：`backend-go/internal/tagmanagement/service/boardhealth/metrics.go:collectBoardStatistics`；模型扩展 `backend-go/internal/tagmanagement/models/board_health.go:BoardHealthLabelMetrics`
- **取舍**：被否方案①「独立信号计算 job」——多一轮 DB 拉取且与统计口径可能漂移；被否方案②「前端实时算」——违反口径唯一实现纪律（红线 22）。pgvector 逐标签查询（≤ 每板 ~98 标签 × top-3）无 ivfflat 索引时走顺序扫描，15k 行 × 每晚一轮可接受；若实测慢，加 ivfflat 索引属运维项不进本 change。

### D2. 磁铁信号口径=「失败子集 vs 全体」贴合差分组统计，不做挂载归因

- **结论**：同一构成标签对 direction_mismatch=true 子集与全部挂载的贴合度均值差 + 失败子集样本量门槛，两者可配（`board_health_magnet_gap` 默认 0.08、`board_health_magnet_min_failed` 默认 5）。样本来自该版块全部挂载 tag（与红线 22「不按 aux 挂载数量筛样本」同口径，仅按 mismatch 分组，不做「哪个标签吸引了哪个 tag」的功劳归因）。同时记录失败挂载的「主要流向」（失败 tag 在其他版块的挂载分布 top1）作为摘除证据。
- **落点**：`backend-go/internal/tagmanagement/service/boardhealth/metrics.go`（分组统计）；配置 `backend-go/internal/tagmanagement/service/boardhealth/boardhealth.go`（沿用当前体检 `board_health_*` key 模式；规范名称以本节结论中的 `board_health_magnet_gap` / `board_health_magnet_min_failed` 为准，旧探索前缀不再采用）
- **取舍**：被否方案「重算匹配做真归因」——违反体检纯读红线 20（需写匹配表才能反推吸引关系），且 B3 实验已证明归因可由分布差代理。

### D3. LLM 输出结构扩展 triage 数组，单调用四产出，提示词以证据锚定分诊

- **结论**：`BoardHealthLLMResult` 扩展 `Triage []TriageItem{aux_id, category(enum remove|rewrite|keep|insufficient), evidence, suggestion}`；system prompt 新增「分诊判据」节（三类信号的解读 + 摘除须磁铁证据 + 短名归 keep + insufficient 标签归 insufficient），user prompt 增补每标签信号证据行。坏 JSON/超时降级语义不变（triage 随 score 一起置空）。
- **落点**：`backend-go/internal/tagmanagement/service/boardhealth/llm.go:BoardHealthLLMResult` + `boardHealthSystemPrompt` + `buildScorePrompt`
- **取舍**：被否方案「独立第二次 LLM 调用做分诊」——两次调用间信号可能不一致、成本翻倍、降级路径复杂化；单调用保持 D5（one call, one decision space）纪律。JSON 输出体积增量约 50 标签 × ~100B ≈ 5KB，结构化输出稳健。

### D4. 分诊清单持久化进 metrics jsonb 顶层 `triage` 字段，历史快照零迁移

- **结论**：`BoardHealthMetrics` 增加顶层 `Triage []BoardHealthTriageItem`（jsonb 序列化，Go 零值 nil → JSON null → 消费方判缺省）。历史快照无此字段自然兼容（jsonb 反序列化零值），不做数据迁移；端点响应原样透出，归一器 `consoleRail.ts` 对缺失字段输出空数组。
- **落点**：`backend-go/internal/tagmanagement/models/board_health.go:BoardHealthMetrics`；`backend-go/internal/tagmanagement/handler/board_health_handler.go`（响应透出）；`front/app/api/normalizers/consoleRail.ts`（归一）
- **取舍**：被否方案「新列 llm_triage jsonb」——同一轮 LLM 产物拆两列徒增一致性负担；被否方案「独立表」——分诊是快照附属产物（同未收编候选模式），无独立查询需求。

### D5. 问题处理主线 + 独立人工描述修复链（2026-10-28 FB-2 修订）

- **结论**：紧凑问题索引 + 单项处理区，keep/insufficient 和统计按需展开；major 原型必须获用户明确批准后才实施。摘除仍复用既有 DELETE + 行内确认，不自动回填历史。重写从复制改为核对可编辑文案、声明全局共享影响后提交独立修复任务；生成 storage 向量→原子保存描述/向量→更新匹配，完成后引导下一项，并明确效果待下一轮体检。boardhealth 采集/查询仍纯读，不能将人工写链混进体检轮次。
- **落点**：既有 BoardWideWorkface/体检组件接统一处理区；auxlabel 新增独立修复 API/任务与测试，复用 board 匹配引擎，接口不再是零新增。boardhealth 不得 import board；修复编排位于独立服务，不破坏原依赖边界。
- **取舍**：被否方案「摘除后即时重算匹配」——体检链路变写链路，破坏红线 20；「弹窗确认」——右轨窄空间行内确认更贴既有交互。

### D6. 提示词证据上限与成本护栏：分诊证据每标签限 3 行、最近邻仅 top-3

- **结论**：user prompt 中每构成标签的证据块限「贴合分布摘要 + 磁铁两侧均值 + 最近邻 top-3 + desc 预筛标记」四行；构成标签超过 60 个时证据块截断至（全部 🔴/🟡 候选 + 低分/磁铁命中 + 按位次补齐），保证提示词体积可控（当前最大板 98 标签 × ~80B ≈ 8KB 增量）。
- **落点**：`backend-go/internal/tagmanagement/service/boardhealth/llm.go:buildScorePrompt`
- **取舍**：被否方案「全量标签进 prompt 不截断」——98 标签板提示词膨胀 ~3 倍，评分锚定漂移风险大（fix-board-health-llm-misread 教训）；被否方案「分批多次调用」——同 D3。

### D7. 描述修复任务：原子保存、幂等恢复、匹配独立重试

- 原型用模拟任务；正式实现需要持久任务记录（新增模型/迁移），不能复用 SemanticBoardBackfillService 的进程内 map 冒充重启可恢复。记录幂等键、标签/来源版本、最终文案、阶段、提交版本、固定匹配范围及逐项结果。相同请求回同任务；同标签执行互斥，旧快照建议须与实时版本核对。
- 先在事务外生成 label+description 的 storage embedding，再锁行校验 active auxiliary 与来源版本，原子写 description+embedding。失败不清空旧向量，不动 label/aliases/merge_embedding。别名治理与 L2 去重规则不变。
- 提交后 InvalidateMatchCache；以既有匹配引擎处理执行起点范围内全部 active topic_tags（含未归属者），持久记录可恢复范围和成功/失败。不能直接调 rematchAll（只枚举既有匹配），也不能使用仅补 NULL 的 backfill-embeddings。匹配失败仅续跑未完成匹配，不重复生成向量/覆盖描述。
- 完成仅意味着匹配已更新；历史体检快照/日报不重写，不自动多跑体检。提交前提示共享标签及全局匹配影响。用户已批准 v2，endpoint/字段与写入分工见 implementation-contract.md，新实施 tasks/test-cases/data-map 已刷新；不可再按 v1 复制-only 假设实施。

## Risks / Trade-offs

- [LLM 把「低 cosine」当摘除依据（提示词漂移回归）] → system prompt 摘除判据只认磁铁信号/流向证据，test-cases 白盒断言提示词含锚定语句；分诊单测用桩 LLM 验证结构不验语义，语义回归靠效果核对（真库抽检）。
- [磁铁信号把「多归属 tag 多」误报为污染（Claude Opus 5.0 类合理次归属流向固定对手版块）] → 证据里带「主要流向」让 LLM 归因时区分「边界重叠」（流向集中且对手版块语义重叠 → 归 keep/rewrite 版块定义而非摘标签）；D2 的 min_failed=5 门槛拦零星失败。
- [最近邻查询无索引时逐标签顺序扫描慢] → 每晚一轮 × ≤600 标签可接受；实测超 5s/板再补 ivfflat（运维项，非本 change 交付）。
- [摘除建议执行后匹配关系短期残留旧挂载] → 既有自动触发机制按自身节奏重算，分诊卡展示「已摘除，等待下轮匹配」状态，不承诺即时生效。

## Migration Plan

新增持久修复任务存储及迁移；原 metrics jsonb 仍向后兼容，历史快照不改写。后端迁移/服务先行，前端静态托管随后更新；部署不自动修复标签，必须逐项人工确认。回滚界面可恢复旧评语展示，但已确认提交的标签修改与匹配不能靠回滚二进制自动撤销；保留任务记录与前后版本用于核查，未完成任务不可静默丢弃。原型阶段不部署、不执行任何真库修改。

## Open Questions

- 磁铁信号阈值 0.08/5 的甜点值：首周真库观察失败子集分布后可经 ai_settings 调，不影响契约（配置项已参数化）。
