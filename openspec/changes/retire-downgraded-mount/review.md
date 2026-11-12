# Review — retire-downgraded-mount

> 审查类型：制品 + diff 聚焦审查（只读）。审查范围：`proposal.md` / `design.md` / `tasks.md` /
> `specs/tag-to-board-matching/spec.md` / `ui-design.md` + commit `4c40fb74`（后端 + 前端 + 文档全量）。
> 审查日期：2026-11-12。结论按 High/Medium/Low 分级；**处置（修/不修）由主线程拍板后回填本表**。

## 0. 总评

- **核心承诺「挂载行为完全不变」成立**：`min(2,N)` 放宽准入逐分支核对无改动，挂载行数/分数不变，
  单 aux 实体标签（Claude/SGLang 等）照常挂载。最高风险点未发现回归。
- **但「唯一用户可见变化 = 徽标消失」不成立**：`MatchTier` 归并会连带改版块文章质量序与分页、
  日报 30 标签截断集合、section `best_tier` 排序（派生排序口径变化，非文章消失）。
- **5 个交付阻塞项**（H1-H5），其中 H1 直接阻塞 openspec 归档，H4 是已实测失败的集成测试，
  H5 会打断 dump-sanitizer 与公开 demo seed 同步链路。

| 级别 | 数量 | 编号 |
| --- | --- | --- |
| High | 5 | H1 H2 H3 H4 H5 |
| Medium | 4 | M1 M2 M3 M4 |
| Low | 4 | L1 L2 L3 L4 |

---

## 1. High

### H1 · `openspec validate --strict` 不通过，change 无法归档
- **文件**：`openspec/changes/retire-downgraded-mount/specs/tag-to-board-matching/spec.md:35/43/54`
- **问题**：delta 把 3 个场景**改名**而非保留主 spec 原名。OpenSpec 视场景改名为「删旧增新」，
  MODIFIED requirement 整块替换时 archive 会拒绝丢弃旧场景。
- **证据**：
  ```
  $ openspec validate retire-downgraded-mount --strict
  ✗ [ERROR] MODIFIED "间接匹配三规则" omits scenario(s) the current spec still has:
    "N=1 时降级匹配", "加权综合分挂载（无降级概念）" ...
  ✗ [ERROR] MODIFIED "Tag 可属于多个 Board" omits scenario(s) the current spec still has:
    "多视角挂载含降级标记"
  ```
  主 spec 原名：`openspec/specs/tag-to-board-matching/spec.md:85` / `:93` / `:123`。
- **建议处置**：**修**。把 delta 的场景名改回主 spec 原名（正文里的 downgraded 断言改为「无标记」即可），
  或用 `## RENAMED Requirements` 显式登记改名。

### H2 · 5 个其它 capability 的主 spec 仍以 downgraded 为规范条款，归档后与实现直接冲突
- **文件/行号**：
  - `openspec/specs/match-detail-ondemand/spec.md:18-24` — SHALL 包含 `downgraded: true` / `downgraded: false`
  - `openspec/specs/board-article-api/spec.md:67`（DTO SHALL 附 `downgraded`）、`:88`（weak 口径含 `downgraded=true`）
  - `openspec/specs/match-score-visualization/spec.md:14`（降级 SHALL 降视觉权重）、`:25`（chip SHALL 加 "↓"）、
    `:28-37`（降级 chip Scenario）、`:81-90`（Requirement「匹配详情面板降级说明」SHALL 标注 "⚠ 降级匹配（原阈值 2…）"）
  - `openspec/specs/workshop-article-archive/spec.md:50`、`:60`、`:67-68`、`:161-162`（弱证据口径含「已降级」、popover SHALL 显示「已降级 ↓」）
  - `openspec/specs/daily-report-system/spec.md:110`、`:170`（BestTier = match_reason + downgraded 映射）
- **问题**：本 change 已删除对应实现（DTO 字段、`matchReasonColor` 双参、"↓" 后缀、popover 徽标、
  `MatchTier` 双参），但 delta 只覆盖 `tag-to-board-matching` 一个 capability。归档后主 spec 仍 SHALL
  要求已不存在的字段与 UI。
- **建议处置**：**修**。补 5 个 capability 的 delta（MODIFIED 对应 requirement 全量重写），
  否则 specs 与代码双轨漂移。

### H3 · proposal 的 Modified Capabilities 指向不存在的 capability
- **文件**：`openspec/changes/retire-downgraded-mount/proposal.md`（## Capabilities 段）
- **问题**：声明 `semantic-board`（`openspec/specs/semantic-board/spec.md`），但
  `ls openspec/specs/` 无 `semantic-board/` 目录；本 change 的 delta 实际在 `specs/tag-to-board-matching/`。
  `semantic-board` 是 constraints-index 的 **flow 域 basename**，不是 openspec capability 名。
- **建议处置**：**修**。改为 `tag-to-board-matching` + H2 的五个 capability。

### H4 · `internal/platform/database/constraints_test.go:109` 集成测试失败（`-short` 门禁漏检）
- **文件**：`backend-go/internal/platform/database/constraints_test.go:109`
  `{"topic_tag_board_labels", "downgraded", true, "false"},`
- **问题**：golden schema 由 AutoMigrate + 迁移构建（`internal/platform/testutil/testutil.go:179`），
  模型已无 `Downgraded` 字段 → 列不再建；迁移 `20260723_0001` 的 `constrain` 有列存在性守卫
  （`postgres_migrations.go:1476-1482`）会跳过 → `information_schema` 查 0 行 → `Row().Scan` 报 no rows。
- **证据**（实测）：
  ```
  $ go test ./internal/platform/database/ -run TestModelTagConstraints_MaterializedInDB -count=1
  --- FAIL: TestModelTagConstraints_MaterializedInDB (24.10s)
      constraints_test.go:125: Received unexpected error: sql: no rows in result set
      Messages: lookup topic_tag_board_labels.downgraded
  ```
- **门禁盲区**：该用例 `if testing.Short() { t.Skip(...) }`（:36-38），且本次只跑了
  `go test -short ./internal/tagmanagement/...`（包未覆盖 + short 跳过，双重漏检）。
- **建议处置**：**修**（删该行用例）。

### H5 · `cmd/dump-sanitizer` 仍 SELECT 已删列 → 演示 seed 链路断裂
- **文件**：`backend-go/cmd/dump-sanitizer/tables.go:137`
  `Columns: []string{..., "match_reason", "downgraded", "direction_mismatch", ...}`
- **问题**：`main.go:264` `buildSelect` → `SELECT ... downgraded ... FROM topic_tag_board_labels`
  → 真库已无该列 → `column "downgraded" does not exist`。
- **连带**：仓库内已提交的 `demo/seed/seed.sql`（999990 行起共 1436 处）
  `INSERT INTO topic_tag_board_labels (...,downgraded,...)` → 在新库回放同样报列不存在；
  `openspec/specs/demo-seed-sync/spec.md` 的每周同步链路依赖该产物。
- **建议处置**：**修**（删 `tables.go:137` 的 "downgraded" + 重新生成 `demo/seed/seed.sql`）。
  该文件不在 design D1 范围表，tasks 也无对应条目。

---

## 2. Medium

### M1 · MatchTier 归并带来用户可见的排序/分页/截断变化，与「行为全部不变」承诺冲突
- **代码位置**：`backend-go/internal/tagmanagement/handler/board_match_handler.go:638-648`（`MatchTier`）、
  `:613-619`（ORDER BY tier CASE）、`:210`（`Order("match_best_tier ASC, ...")`）、
  `backend-go/internal/topicgraph/service/daily_report_orchestrator.go:241`（section `bestTier`）、
  `:775-776`（`filterTagsByQuality` 截断排序）、`backend-go/internal/topicgraph/service/daily_report_rebucket.go:367`
- **问题**：
  1. 版块文章列表：`match_best_tier` 决定全局质量序，且排序后分页 → 仅以降级 max_sim 为证据的文章
     从 tier 3 升 tier 2，**位置前移、分页内容变**（用户可见）。ORDER BY 已同步——同步即意味着一起变。
  2. 日报：`filterTagsByQuality` 按 (MatchTier, Score) 排序后截断 30 → 原降级 max_sim 独占 tier3 末位，
     现入 tier2 参与竞争 → **日报入选 tag 集合变化** → 进 LLM 聚类的 tag 集合变化 → section 归属可能漂移
     （先例见 `docs/v1.3.4-easy-use/changes/observablity/2026-06-28-quality-scoring-observability/design.md:141`）。
  3. `daily_report_sections.best_tier` 由 3 变 2 → 前端 `sortDailyReportSections`
     （`front/app/features/tags/components/daily-report/dailyReportMagazine.ts:68`）→ **section 顺序变**。
- **design D2 只承认「排序变化可接受」，未承认分页集合/截断集合/section 排序变化；
  proposal Impact 却写「日报、聚类、归档全部不变」。**
- **建议处置**：**修（改口径文档，不改代码）**。proposal Impact 改为「挂载集合/分数不变；
  派生排序口径（MatchTier 归并）变化：版块文章质量序与分页、日报 30 标签截断、section best_tier 排序随之变化」，
  design D2 补影响面清单 + 真库抽样核对（迁移前后按 board 比对 tier 分布 / 前 30 标签集合差异）。

### M2 · `ArchiveEvidenceRank` 档位重排未在 design/tasks 中披露
- **文件**：`backend-go/internal/tagmanagement/service/board/archive.go:167-179`
- **问题**：改前 `case "max_sim" && !downgraded: return 4` / default 5；改后 `case "max_sim": return 4` / default 5。
  → 原「降级 max_sim」在归档「首条标签」排序里由 rank5（末位）升到 rank4，与普通 max_sim 同档，
  改由 score/id tiebreak（`board_match_handler.go:280-290`）。同一文章同时有普通与降级 max_sim 标签时，
  归档行内标签顺序变。design D1 只写「签名去 downgraded 参数」。
- **建议处置**：**修（文档）**。design D1 补一句档位重排说明；proposal 的「归档全部不变」改为「归档弱证据成员不变、
  行内标签次序按证据档+分数统一排序」。

### M3 · `docs/reference/standard/backend/testing.md:171` 指向已重命名的测试
- **文件**：`docs/reference/standard/backend/testing.md:171`
  「回归守卫样本：`TestCollectBoardTags_PGHonorsDowngradedColumn`」
- **问题**：该测试已改名为 `TestCollectBoardTags_PGHonorsGroupBy`（`collect_board_tags_test.go:29`）→ 读者按图索骥找不到。
- **建议处置**：**修**（1 行改名）。:166 的事故根因叙述属历史上下文，不改。

### M4 · 残留引用未闭环（清理类）
- `backend-go/internal/platform/database/postgres_migrations.go:1558` `{"topic_tag_board_labels","downgraded","false",true}`
  —— 有列存在性守卫（:1476-1482）不会报错，但已是死条目。
- `backend-go/internal/topicgraph/repository/daily_report_quality_breakdown_test.go:47-49, :85` —— fixture JSON 仍带
  `downgraded` 键、断言 `result[0]["downgraded"] == false`；因 fixture 自带该键测试仍绿，但语义已漂移
  （实为「历史 JSON 含 downgraded 键仍可读」的兼容用例，无注释说明）。
- `backend-go/internal/topicgraph/service/daily_report_rebucket_test.go:79` —— 同类 fixture。
- `backend-go/internal/tagmanagement/handler/board_article_handler_test.go:136` —— slug 仍叫 "downgraded"，误导。
- **建议处置**：**修（低成本的清理）**：删迁移死条目；给两个 fixture 加「历史兼容」注释或改名；
  测试 slug 改名。

---

## 3. Low

### L1 · `MatchDetailPanel.vue` 整文件 CRLF→LF，644 行 diff 噪音淹没 1 行真实改动
- **证据**：`git show 4c40fb74 --numstat` → `644 644 front/app/features/tags/components/MatchDetailPanel.vue`；
  `git show -w` 仅剩 1 行语义改动（`:207` 去掉 `⚠降级匹配（原阈值…）`）；父提交版本为 CRLF。
- **建议处置**：**修（仅整理）**：行尾归一拆成独立 commit 或还原 CRLF。

### L2 · 前端死代码残留
- `front/app/features/tags/components/console/boardArchiveView.css:435` `.ar-tag.is-deg` —— `:class` 已删 `is-deg`，规则成死样式。
- `front/app/features/tags/components/daily-report/SectionQualityExplore.vue` —— 删 `:class` 后模板留下一行空行。
- **建议处置**：**修（顺手清理）**。

### L3 · design D1 的删除范围表不完整
- design D1 前端表只列 ArchiveMatchPopover.vue / archiveMatchFlow.ts / useBoardArchive.ts / 测试文件；
  实际还改了 `front/app/utils/matchQuality.ts`(+test)、`console/BoardArchiveView.vue`、
  `daily-report/SectionQualityExplore.vue`、`MatchDetailPanel.vue`、`api/dailyReports.ts`、`api/semanticBoards.ts`；
  后端漏了 `internal/platform/database/postgres_migrations.go`、`cmd/dump-sanitizer/tables.go`（后者是漏改，见 H5）。
- **建议处置**：**修（补全 design D1 清单）**。

### L4 · `effective_min_hits` 已无生产消费方
- `board_match_handler.go:418` 仍返回该字段，`front/app/api/semanticBoards.ts:415` 仍有类型声明；
  但 `MatchDetailPanel.vue` 第 ④ 步文案改用前端自算 `minHits`（`archiveMatchFlow.ts:251`），
  后端字段实际无消费方（grep 仅命中测试 fixture 与类型声明）。
- **建议处置**：**不修（观察）**——留着无害；若后续清理 `match-detail-ondemand` spec 时一并退役。

---

## 4. 已核实通过（无问题）

| 核对项 | 结论 | 证据 |
| --- | --- | --- |
| `min(2,N)` 放宽准入保留 | ✔ | `semantic_board_matching.go:475` `minHits := min(config.DirectMaxSimMinHits, len(tagAuxiliaries))` 原样 |
| max_sim 判定条件未改 | ✔ | `semantic_board_matching.go:480` 条件逐字保留 |
| **是否有行因删降级标记而少写** | ✔ **不会** | 降级从来不是写入条件，只是随行携带的布尔位；`if matchReason != ""` 写入分支（:488-490）与 `replaceTopicTagBoardLabels`（:834-847）未变 |
| 挂载分数 | ✔ 不变 | score 计算分支未动 |
| 弱证据列表成员（生产口径） | ✔ 不变 | `downgraded ⇒ match_reason='max_sim'`（降级只在 max_sim 分支置位）⇒ pre/post 的 strong 与 weak 判定等价（`archive.go:149/186`） |
| N=1 单 aux 仍照常挂载（有测试守） | ✔ | `semantic_board_matching_unit_test.go:462-474` 仍断言 `Len==1` + `MatchReason=="max_sim"` |
| 迁移幂等 | ✔ | `scripts/db/drop-downgraded-20261112.sql:6` `DROP COLUMN IF EXISTS`，不删行 |
| topicgraph 三处 SELECT/GROUP BY 同步 | ✔ | `daily_report_orchestrator.go:576/588/608`、`daily_report_models.go:324`、`daily_report_rebucket.go:191/201/344` |
| GROUP BY 契约的 PG testcontainer 守护仍在 | ✔ | `collect_board_tags_test.go`（改名后仍存在） |
| 前后端 DTO/类型/fixture 同步 | ✔ | `board_match_handler.go:31/65-69` ↔ `front/app/api/semanticBoards.ts:356/411`、`dailyReports.ts:154` |
| 反向守卫（字段不存在）到位 | ✔ | `daily_report_orchestrator_test.go:41/70/165` `assert.NotContains(e,"downgraded")` |
| 文档三处对齐 | ✔ | `docs/reference/flow/semantic-board.md:26`、`docs/reference/api/semantic-boards.md:932`、`docs/reference/database/tables/semantic-labels.md:64` + `_conventions.md:234` |
| 主 spec `tag-to-board-matching` 条款被 delta 覆盖 | ✔（但见 H1 阻塞） | delta 全量重写了「间接匹配三规则」「Tag 可属于多个 Board」两个 requirement 的正文与场景 |

---

## 5. 遗留遗漏扫描（排除无关语义）

| 位置 | 残留 | 性质 | 处置 |
| --- | --- | --- | --- |
| `internal/platform/database/constraints_test.go:109` | 断言列存在 | **失败** | H4 必删 |
| `internal/platform/database/postgres_migrations.go:1558` | 约束表列 downgraded | 死条目（守卫跳过） | M4 清理 |
| `cmd/dump-sanitizer/tables.go:137` | SELECT 已删列 | **运行期报错** | H5 必删 |
| `demo/seed/seed.sql`（1436 处） | INSERT 含 downgraded | **回放报错** | H5 重生成 |
| `internal/topicgraph/repository/daily_report_quality_breakdown_test.go:47-49,85` | fixture/断言 | 仍绿、语义漂移 | M4 加注释 |
| `internal/topicgraph/service/daily_report_rebucket_test.go:79` | fixture | 同上 | M4 |
| `internal/tagmanagement/handler/board_article_handler_test.go:136` | slug "downgraded" | 命名误导 | M4 |
| `internal/tagmanagement/service/board/semantic_board_matching.go:436`、`semantic_board_matching_test.go:186/202`、`semantic_board_upgrade_test.go:438` | "direct_hit downgraded to 0.7" | 无关（分数降级） | 不动 |
| `internal/platform/middleware/compress_test.go:47` | gzip q-value 降级 | 无关 | 不动 |
| `internal/dataenrichment/**`（`board_investigation_synthesis.go:1016` 等） | `downgraded` 局部变量 / `invSynthesisLLMDowngraded` | **AI 路由降级，无关** | 不动 |
| `internal/topicgraph/service/daily_report_lane_test.go:92`、`daily_report_lane_pipeline_test.go:18/44/73` | "downgraded to L2" | 泳道降级，无关 | 不动 |
| `internal/reader/service/feed_service_unit_test.go:276` | "downgraded to fallback" | 无关 | 不动 |
| `front/.../SectionQualityExplore.test.ts:48`、`front/app/utils/matchQuality.test.ts:18-19` | 反向守卫 + 注释 | 保留 | 不动 |
| `docs/v1.3.4-easy-use/**`、`docs/archive/plans/**` | 历史归档文档 | 历史记录，改动不合理 | 不动 |
| `docs/reference/standard/backend/testing.md:166` | 事故根因叙述 | 历史上下文 | 不动（只改 :171，见 M3） |

---

## 6. 核实记录（本次实际执行的验证命令）

| 命令 | 结果 |
| --- | --- |
| `git show 4c40fb74 --stat` / `git show 4c40fb74`（后端+前端+文档全量） | 已逐文件核对 |
| `openspec validate retire-downgraded-mount --strict` | **FAIL（2 ERROR）** → H1 |
| `openspec validate --changes --strict` | 本 change FAIL；`add-home-pipeline-status-page` 的 FAIL 为存量，非本 change |
| `go test ./internal/platform/database/ -run TestModelTagConstraints_MaterializedInDB -count=1` | **FAIL**（`sql: no rows in result set`）→ H4 |
| 全仓 grep `downgraded` / `已降级` / `降级匹配`（排除 dataenrichment AI 路由降级、泳道降级等无关语义） | 见 §5 |
| `git show 4c40fb74 --numstat` + `file` | 确认 MatchDetailPanel.vue CRLF→LF → L1 |

未重跑（brief 已提供全绿结论，未重复消耗）：`go build ./...`、`go vet ./...`、
`go test -short ./internal/tagmanagement/...`、`golangci-lint`、前端 `pnpm lint` / `nuxi typecheck` / 受影响单测。
未重跑真库迁移（brief 已提供：3627 行零删除、列已删、`/health` 200）。

---

## 7. 处置栏（主线程回填，2026-11-12）

| 编号 | 处置 | 修复 commit | 备注 |
| --- | --- | --- | --- |
| H1 | **修** | 本次提交 | openspec 的 MODIFIED 块按**场景名**对齐，无 scenario 级 RENAMED 机制 → delta 场景名改回主 spec 原名（「N=1 时降级匹配」「加权综合分挂载（无降级概念）」「多视角挂载含降级标记」），正文改为「无标记」语义；历史命名保留作为身份锚，Requirement 正文加一句说明标题沿用历史命名。`openspec validate --strict` 通过。 |
| H2 | **修** | 本次提交 | 补 5 个 capability 的 delta：`match-detail-ondemand` / `board-article-api` / `match-score-visualization` / `workshop-article-archive` / `daily-report-system`。全部 MODIFIED 块场景名与主 spec 逐一对齐（机械脚本校验 14/14 通过），仅改正文去 downgraded 条款。 |
| H3 | **修** | 本次提交 | proposal 的 Modified Capabilities 改为真实 capability 全集 6 项（原 `semantic-board` 是 constraints-index 的 flow 域 basename，非 openspec capability 名）。 |
| H4 | **修** | 本次提交 | `constraints_test.go:109` 删 `topic_tag_board_labels.downgraded` golden schema 行（列已不存在 → information_schema 0 行 → `no rows in result set`）。`go test ./internal/platform/database/ -count=1` 通过（43.5s，testcontainer PG）。 |
| H5 | **修** | 本次提交 | ①`cmd/dump-sanitizer/tables.go` 的 `topic_tag_board_labels` 列清单去 `downgraded`；②顺带移除已被迁移 `restructure-settings-navigation` DROP 的 `analysis_methods` / `reference_roles` 两条 stale 表条目（该工具此前必然报 `relation does not exist`，是本项的真正阻断点）；③`demo/seed/seed.sql` 按新列清单重新生成（311525 行）；④静态校验：seed.sql 全部 INSERT 列清单对照真库 information_schema —— 零缺失。 |
| M1 | **修（改口径文档，不改代码）** | 本次提交 | proposal Impact 拆成「不变」与「变」两层：挂载集合/分数/文章可见性/日报入选文章/归档弱证据**成员**不变；派生排序口径变（版块文章质量序与分页、日报 top-30 标签截断集合、section best_tier 排序、归档行内标签次序）。design 新增 D2.1 影响面清单表 + D2.2 归档档位重排 + D2.3 真库核对。代码不改——档位合并本身是本 change 的既定决策。 |
| M2 | **修（文档）** | 本次提交 | design D2.2 明确 `ArchiveEvidenceRank` 由「`max_sim && !downgraded` → rank 4 / default 5」改为「`max_sim` → rank 4 / default 5」：行成员不变，仅行内次序由 score/id tiebreak 决定。proposal Impact 同步。 |
| M3 | **修** | 本次提交 | `docs/reference/standard/backend/testing.md` 的回归守卫样本名改为 `TestCollectBoardTags_PGHonorsGroupBy`（并注明原名含 Downgraded 的用例已随本 change 改名）。 |
| M4 | **修** | 本次提交 | ①`postgres_migrations.go` 约束表死条目删（`constrain` 有存在性守卫，不报错但是死代码）；②`daily_report_quality_breakdown_test.go` / `daily_report_rebucket_test.go` 的 fixture 保留 `downgraded` 键但加注释说明是历史 JSON 快照遗留键（读取端兼容用例，语义不再漂移）；③`board_article_handler_test.go` 的 slug `downgraded` → `hitrate`，变量 `downTag` → `hitRateTag`。 |
| L1 | **修** | 本次提交 | `MatchDetailPanel.vue` 转回 CRLF 行尾（原文件是 CRLF，Python 编辑把整文件转成 LF 造成 644 行噪音 diff）。现在相对 `4c40fb74^` 的净改动 = 1 行（真实改动行）。 |
| L2 | **修** | 本次提交 | `boardArchiveView.css` 的 `.ar-tag.is-deg` 死 CSS 删；`SectionQualityExplore.vue` 删 `:class` 后遗留的空行删。 |
| L3 | **修** | 本次提交 | design D1 表补全：后端加 `cmd/dump-sanitizer/tables.go` 与 `internal/platform/database/constraints_test.go`；前端加 `BoardArchiveView.vue` / `MatchDetailPanel.vue` / `SectionQualityExplore.vue` / `utils/matchQuality.ts` / `api/semanticBoards.ts` / `api/dailyReports.ts`。 |
| L4 | **不修（观察）** | — | `effective_min_hits` 仍由 `board_match_handler.go` 返回、前端类型仍声明，但生产消费方已改为前端自算门槛（`archiveMatchFlow.ts:251`）。保留字段（无功能危害、且是判定口径的可追溯线索），待 `match-detail-ondemand` 能力后续单独退役。 |

### 处置后门禁复检（2026-11-12）

| 命令 | 结果 |
| --- | --- |
| `openspec validate retire-downgraded-mount --strict` | 通过 |
| `go build ./...` / `go vet ./...` | 通过 |
| `golangci-lint run ./...` | 0 issues |
| `go test -short ./internal/tagmanagement/... ./internal/topicgraph/...` | 全绿 |
| `go test ./internal/platform/database/ -count=1` | 通过（testcontainer PG，43.5s） |
| `pnpm lint` | 0 error（8 warning 均为存量、他文件） |
| `pnpm exec nuxi typecheck` | 通过 |
| `pnpm test:unit <8 个受影响文件> --maxWorkers=2` | 112 passed |
| `bash scripts/harness/doc-impact.sh verify openspec/changes/retire-downgraded-mount` | 通过 |
| `bash scripts/harness/check-standards.sh` | 通过 287 / 失败 1（唯一失败为 2026-11-09 `fix-board-health-triage-degrade` 的既有溯源欠账，**非本 change**，不阻塞本 change 归档） |
