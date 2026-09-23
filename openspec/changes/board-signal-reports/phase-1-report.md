# 阶段1交付报告：数据与源（phase-1-report）

> 2026-09-22，develop 主仓库直改（树上其他 change 脏改未触碰、未还原）；实现定位以 `phase-0-runtime.md` 为准，controller 裁决（EIA 归档 CSV 路线 / table9 禁接入 / 4.2 划入本阶段 / 假日周四跳变）全部落实，无偏离。

## ① 改动文件清单

**datasources（3.1/3.2）**

| 文件 | 改动 |
| --- | --- |
| `backend-go/internal/datasources/sources/eia.go` | `Fetch` 增加可选变参 `weeks`（显式 1~12 才生效，发网前校验）；新增归档版次 URL 拼装、周三→周四→周二版次探测、期望周缺失显式失败守卫、扁平周序列收集器与响应装配；缺省路径逐字节保持原行为 |
| `backend-go/internal/datasources/sources/eia_test.go` | 新增 8 用例：真实归档 fixture 解析、weeks 校验零网络、窗口装配、假日周四探测、版次错配显式失败、缓存键不串、既有默认模式回归保持 |
| `backend-go/internal/datasources/sources/jodi.go` | `Fetch` 增加可选变参 `years`（显式 1~5，与 month 互斥）；新增 `validateJodiWindow`（发网前）与 `fetchYearsWindow`（多年循环、本年 404 回退去重、历史年 gap、非 404 整呼失败）；单文件路径改名 `fetchSingle` 行为零变化 |
| `backend-go/internal/datasources/sources/jodi_test.go` | 新增 6 用例覆盖 HR-2~5（窗口/互斥/404去重/gap不级联/非404失败/缓存复用），既有 8 用例零改动通过 |
| `backend-go/internal/datasources/wiring/tools.go` | 最小增量：`EIAFetcher/JODIFetcher` 接口加尾随变参；两个工具 schema 增加可选 `weeks`/`years` 属性（不入 required）；新增 `optionalIntArg` 严格整数参数解析（小数/字符串/越界→INVALID_ARGUMENT） |
| `backend-go/internal/datasources/wiring/tools_test.go` | stub 改指针实现；新增 3 用例：四工具注册面不变+新参数不入 required、weeks/years 透传与非法类型拒绝 |

**dataenrichment repository（2.1/4.2）**

| 文件 | 改动 |
| --- | --- |
| `backend-go/internal/dataenrichment/repository/signal_models.go` | 新增：`BoardSignalDiscovery`/`BoardSignalCandidate` 模型、granularity/mode 常量、`ValidSignalGranularity`/`ValidSignalPeriod`、`SignalEvidenceRefs` 解码、`ComputeSignalCandidateDedupeKey`（标题规范化+证据集合排序去重 sha256） |
| `backend-go/internal/dataenrichment/repository/signal_repository.go` | 新增：`CreateSignalDiscoveryBatch`（单事务原子保存+同批机械去重+owner/period 代码级 stamping）、候选/报告查询（按 board+period+id 游标列表、候选最新成功报告 id、报告版本列表、批次计数） |
| `backend-go/internal/dataenrichment/repository/models.go` | `ResultKindSignalReport` 常量；`TopicEnrichmentResult` 增加 nullable `Granularity/Period/SourceSignalID` 三列；新表注册进 `RegisterModels` |
| `backend-go/internal/dataenrichment/repository/repository.go` | `validateResultShape` 增加 signal_report 分支（board scope+三列必填+候选同板块同周期存在性校验）；旧 kind 分支零改动，`isBoardResultKind` 不含 signal_report（旧 API 语义隔离） |
| `backend-go/internal/dataenrichment/repository/signal_candidate_test.go` | 新增：隔离 PG（SetupTestDB）8 用例覆盖 DB-1/2+S1 持久化（原子保存/回滚无半批/直写 FK 拒绝/零候选批次/同批去重/游标分页/派生状态数据源/去重键规范化） |
| `backend-go/internal/dataenrichment/repository/signal_report_test.go` | 新增：隔离 PG 5 用例覆盖 DB-3~5（repo 形状校验、直写非法形状被 PG 拒绝、版本追加不覆盖、kind/period 隔离列表、旧行三列 NULL） |

**platform/database（4.2）**

| 文件 | 改动 |
| --- | --- |
| `backend-go/internal/platform/database/postgres_migrations.go` | 追加迁移 `20260922_0001`（详见②）；幂等（依赖感知的 DROP 顺序：result-FK → candidate-FK → 两个 UNIQUE → 重建） |
| `backend-go/internal/platform/database/board_signal_migration_test.go` | 新增：迁移驱动测试 3 用例（DB-4 旧库模拟：先 DROP 三列→插 legacy 行→跑迁移→旧行 NULL+payload 不变；DB-3 直写拒绝；幂等重跑+约束/索引计数） |

**dataenrichment service（2.2，仅限新文件）**

| 文件 | 改动 |
| --- | --- |
| `backend-go/internal/dataenrichment/service/signal_material.go` | 新增：`ParseSignalPeriod`（Asia/Shanghai 半开区间+未来周期拒绝，不改现有 `ParsePeriodRange`）、`SignalCutoff`/`SignalAnalysisMode`、切片/摘要/背景选择的纯函数核心、`SignalMaterialBuilder.AssembleSignalMaterial`（只读复用 LifelineReader，freshness 零重写） |
| `backend-go/internal/dataenrichment/service/signal_material_test.go` | 新增：纯逻辑 10 用例覆盖 PC-1~PC-5（无 DB 无 SQLite） |

**制品**：`openspec/changes/board-signal-reports/tasks.md`（勾选 2.1/2.2/3.1/3.2/4.2，证据格式参照 1.2/1.3）；本报告。

## ② 新增 DDL 摘要（迁移 20260922_0001）

**新表**（AutoMigrate 建表，迁移补约束）：

```sql
board_signal_discovery (
  id BIGSERIAL PK, semantic_board_id BIGINT NOT NULL,
  granularity VARCHAR(10) NOT NULL, period VARCHAR(12) NOT NULL,
  analysis_mode VARCHAR(16) NOT NULL DEFAULT 'current',  -- current|retrospective
  cutoff TIMESTAMPTZ NOT NULL, input_snapshot JSONB, session_id VARCHAR(120),
  candidate_count INT NOT NULL DEFAULT 0, created_at
)
board_signal_candidate (
  id BIGSERIAL PK, discovery_id BIGINT NOT NULL,
  semantic_board_id BIGINT NOT NULL, granularity VARCHAR(10) NOT NULL, period VARCHAR(12) NOT NULL,
  signal TEXT NOT NULL, why_it_matters TEXT NOT NULL, research_question TEXT NOT NULL,
  evidence_refs JSONB NOT NULL(非空数组), score INT NOT NULL, rationale TEXT NOT NULL, created_at
)
```

**约束/索引**：

```sql
-- 复合 owner 唯一（FK 目标）
uq_board_signal_discovery_id_owner  UNIQUE (id, semantic_board_id, granularity, period)  ON discovery
uq_board_signal_candidate_id_owner  UNIQUE (id, semantic_board_id, granularity, period)  ON candidate
-- 复合外键（owner/周期一致性 DB 级强制，直写也拒绝）
fk_board_signal_candidate_discovery: candidate(discovery_id,board,gran,period) → discovery(id,……) ON DELETE RESTRICT
fk_topic_enrichment_result_signal_candidate: result(source_signal_id,board,gran,period) → candidate(id,……) ON DELETE RESTRICT
-- result kind/形状 CHECK：DROP+re-ADD，signal_report 分支
chk_topic_enrichment_result_kind: 枚举加 'signal_report'（5 值）
chk_topic_enrichment_result_parent_shape: signal_report 分支 = board scope + board 非空 + topic/parent/question 全空
  + granularity IS NOT NULL AND period IS NOT NULL（NULL 显式拒绝，防 CHECK UNKNOWN 放行）
  + granularity IN ('month','year') + 月/年日历形状正则 + year∈[2000,2100] + source_signal_id IS NOT NULL；
  其余四个 kind 分支追加「三列必须 NULL」
-- 索引
idx_board_signal_discovery_board_period (semantic_board_id, granularity, period, id DESC)
idx_board_signal_candidate_board_period (semantic_board_id, granularity, period, id DESC)
idx_board_signal_candidate_discovery (discovery_id, id DESC)
idx_topic_enrichment_result_signal_board_period (…, id DESC) WHERE result_kind='signal_report'
idx_topic_enrichment_result_source_signal (source_signal_id, id DESC) WHERE source_signal_id IS NOT NULL
```

`topic_enrichment_result` 新增 nullable 列 `granularity VARCHAR(10)` / `period VARCHAR(12)` / `source_signal_id BIGINT`，旧行不回填（迁移有防御校验：非 signal 行带值即拒绝迁移）。result payload 仍放现有 `sectors` jsonb（`{schema_version:2, signal_snapshot, report, appendix, generation_meta}` 形状由阶段2 写入，本阶段不加列不加约束）。

## ③ EIA/JODI 新参数契约（给阶段2 消费）

**Go 签名**（wiring 接口同步，均为尾随变参=可选显式）：

```go
(*sources.EIA). Fetch(ctx, section string, weeks ...int) (map[string]any, error)
(*sources.JODI).Fetch(ctx, geo, flow, unit, month string, years ...int) (map[string]any, error)
```

**EIA weeks**
- 显式整数 1~12；不传/传空=旧行为（当周+上周成对形状，`prior_period/prior_value` 键保留）；13/0/负数/多值→INVALID_ARGUMENT 且零网络。
- weeks 模式响应：`weeks`(回显 int)、`periods`（升序 YYYY-MM-DD）、`observations` 扁平数组（按 flow 分组、period 降序；键：geo/product/flow/label/frequency/unit/period/value/raw_value/source_row/source_edition，缺失=null+missing_reason，**无 prior_* 键**）、`documents`（current+每个归档版次的 url/retrieved_at/last_modified/source_sha256）。
- 取数路径：现行文件仍走 `ir.eia.gov`（给出最新两周，重叠周期以现行文件为最新修订）；更早周走官方归档 `www.eia.gov/petroleum/supply/weekly/archive/{YYYY}/{YYYY_MM_DD}/csv/table1.csv`（无 key，禁 api.eia.gov v2）；版次定位=周结周五+5/6/4 天探测（周三→周四→周二），版次不含期望周→SOURCE_UNAVAILABLE（不静默留洞）。
- 缓存键：缺省 `eia:table1`（不变）；weeks=N → `eia:table1:weeks=N`（整包响应缓存，命中保留原 retrieved_at/sha256，TTL 900s）。**阶段2 cutoff 过滤**：观测 period 均为周结日，直接按 `period < cutoff` 过滤即可（cutoff 语义=发现 started_at）。

**JODI years**
- 显式整数 1~5；与 month 互斥（同传→INVALID_ARGUMENT 发网前）；不传=旧行为（最新月，null 不回退）；显式 month 行为不变（404 不回退）。
- years 模式响应：`years_requested`/`years_used`（实际取到文件的年，降序）/`year_gaps`（[{year,reason}]）/`year_strategy`/`documents`（每文件溯源）；observations=窗口内**所有可得月份**（升序，键同旧+`source_year`）；本年 404→回退上一年一次（N≥2 时上一年本就在窗口内，去重不重复请求）；历史年 404→gap 记录不级联；非 404（网络/5xx）→整呼 SOURCE_UNAVAILABLE（不伪装 gap）。
- 缓存键不变：`jodi:primary:{year}`（month/years 是过滤维度）。

**wiring 工具层**：`eia_wpsr_table1` schema 增加可选 `weeks`(integer 1~12)，`jodi_oil_primary` 增加可选 `years`(integer 1~5)，均**不在 required**（旧调用者参数集合法）；JSON 非整数/字符串/越界在 wiring 层即拒（error_code=INVALID_ARGUMENT）。四工具名/数量不变。

## ④ signal_material 公开 API（给阶段2）

```go
// 周期（PC-1/PC-2）
func ParseSignalPeriod(granularity, period string, now time.Time) (SignalPeriod, error)
  // SignalPeriod{Granularity, Period, From(含), To(排他)}；业务时区 Asia/Shanghai；
  // 仅 month|year；非法/未来→error（handler 直接转 400，无需起 job）；
  // 不改现有 ParsePeriodRange（其 UTC 边界供 lifeline 消费方继续使用）
func SignalCutoff(p SignalPeriod, now time.Time) time.Time
  // 当前周期→now（发现 started_at）；已结束周期→p.To（周期结束）
func SignalAnalysisMode(p SignalPeriod, cutoff time.Time) string
  // cutoff≥To → "retrospective"，否则 "current"

// 材料（PC-3/4/5，纯函数已测，DB 装配为薄组合）
func filterSignalSections(sections []TimelineSectionNode, from, cutoff time.Time) []SignalArticleSlice
  // PeriodDate ∈ [from, cutoff)，升序——冻结边界，cutoff 后新新闻不进材料
func selectSignalPeriodSummary(rows []LifelineArchiveRow, granularity, period string, cutoff time.Time) *string
  // 仅取目标周期且 AsOfDate≤cutoff 的摘要；as-of 晚于 cutoff（可能混后期事实）→ nil，回落原切片
func selectSignalBackgroundSummary(rows []LifelineArchiveRow, granularity, period string, cutoff time.Time) *SignalBackgroundSummary
  // 严格早于目标周期且 AsOfDate≤cutoff 的最新归档摘要；无→nil（如实标注）
func appendSignalMembershipGap(m *SignalMaterial)
  // 历史窗口归属不可还原的诚实 gap（不猜历史全貌）

type SignalMaterialBuilder struct{ /* db + LifelineReader */ }
func NewSignalMaterialBuilder(db *gorm.DB, reader LifelineReader) *SignalMaterialBuilder
func (b *SignalMaterialBuilder) AssembleSignalMaterial(ctx, boardID uint, p SignalPeriod, cutoff time.Time) (*SignalMaterial, error)
  // 全状态泳道（候选泳道来自周期材料，不只用今天 active）；创建晚于 cutoff 的泳道跳过；
  // 零材料泳道省略；historical 模式追加 membership gap；SignalMaterial.IsEmpty() 为 true 时
  // 阶段2 免 detect LLM 直接保存零候选批次
```

SignalMaterial 可直接 JSON 序列化存入 `BoardSignalDiscovery.InputSnapshot`（含 from/to/cutoff/lanes/gaps 全量）；候选研究时读取同一快照，不重新装配（PC-5 冻结）。

## ⑤ 测试结果（命令+退出码）

| 命令（cwd=backend-go） | 退出码 | 包 |
| --- | --- | --- |
| `go test ./internal/datasources/... -count=1` | 0 | datasources / sources / wiring（14 新用例+既有回归） |
| `go test ./internal/dataenrichment/service/ -short -count=1` | 0 | service（signal_material 10 用例+全包单测回归） |
| `go test ./internal/dataenrichment/repository/ -count=1` | 0 | repository（隔离 PG，13 新用例+既有回归） |
| `go test ./internal/platform/database/ -count=1` | 0 | database（隔离 PG，迁移 3 新用例+既有迁移回归） |
| `golangci-lint run ./...` | 0 | 0 issues |
| `go vet ./...` | 0 | — |
| `go build ./...` | 0 | — |

隔离 PG 经 testcontainers（pgvector/pgvector:pg18-trixie，Docker 已在跑）；repository/database 全程禁 SQLite（`grep glebarez/sqlite internal/dataenrichment/repository/` 零命中）；未连业务库（testutil 无默认 DSN 红线保持）。

## ⑥ 未做项/残余风险

- **阶段2/3 辖区零涉及**：service 编排（detect/research/compose/policy）、handler/路由、`analysis_runner` 扩展、wire.go 装配、前端、文档（7.1~7.4）均未动。signal_report 的 payload 形状（sectors 内 schema_version=2）本阶段只保证列/CHECK/FK/索引就绪。
- **S1 完整故事**不在本层闭合：候选「刷新/重启保留」的端到端属阶段2 handler 测试+人工验收；本层交付的是持久化、原子性与查询原语。
- **EIA 归档版次步进依赖周结=周五**的 WPSR 惯例（步进从实际解析出的周结日回退 7 天，不假设今天日期）；若 EIA 未来改变发布结构，错误会显式暴露（版次错配守卫），不会静默给错窗口。
- **EIA weeks 响应缓存为整包缓存**（键含 weeks）：不同 N 之间不共享归档版次下载（每个 N 独立拼装），TTL 900s 内重叠下载成本可接受；JODI 为按文件缓存天然共享。已在③注明。
- **迁移幂等重跑**已测；生产库执行时 `fk_...` 依赖顺序已处理（2BP01 路径），但与树上其他 change 的迁移并发合并时仍需主线程统一收口版本号。
- **真实外网核定**依赖阶段0 fixture（2026-08-26/2019-01-04 真实字节）；步进逻辑用 httptest 合成版次（测试内联构造，与既有 jodi 测试同风格），未新补真实 fixture（无需——窗口拼装的正确性由真实 fixture 作 live 路径+合成步进覆盖）。

## ⑦ 给 controller 的裁决请求

无阻塞裁决项。两点备案：
1. **EIA weeks 响应形状**为「扁平逐周观测（无 prior_* 键）+periods+documents」——design §5 只规定了参数与窗口语义，未钉死响应形状；此形状已在③固化为本阶段合同，阶段2 的 research 工具消费与 SV/OB 测试按此对齐即可，若 controller 希望改成「逐周重复成对结构」需在阶段2 开工前定夺。
2. **JODI 非 404（网络/5xx）在多年窗口中整呼失败**（不记 gap）：design 只规定了 404 分链；本实现选择「瞬时故障不伪装成数据缺口」，与约束#18 的 gap 语义（检索不到=确认缺失）一致。若希望单年瞬时失败也降级为 gap 继续，需要放宽——建议维持现状。
