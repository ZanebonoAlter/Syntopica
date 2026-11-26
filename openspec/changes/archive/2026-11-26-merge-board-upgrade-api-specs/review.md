# 审查报告：merge-board-upgrade-api-specs

- **审查者**：MiniMax M3.1 Flash（只读）
- **结论**：需改后进
- **放行判断**：内容守恒与逐字一致性做得干净（32/32 ADDED 逐字零差异、REMOVED 标题 32/32 对齐），但按 tasks 现状归档跑不通——被退役的 capability 实为 **5 个**（tasks 4.3 / design D5 写「4 个」），少摘一个 `<!-- domain: -->` 就会让 `openspec archive` 以 `ArchiveBlockedError` 整次中止、零文件改动；另有 board-api 主 spec 的 Purpose 落成 TBD 占位、4 处 capability 活引用变死链、跨族重复未消。

## 总评

| 清单项 | 结论 | 要点 |
|---|---|---|
| A business/impl 分类 | 1 条 impl 判定成立；判据未成文 | 唯一 impl `API type updates` 判 impl **成立**（validate 亦警告它缺 SHALL/MUST，是纯类型定义）。A1 `Board rematch API` 判 business **成立**（断言端点 + 可观测效果 + 容错行为），但点名内部函数 `MatchTopicTag`，属「契约 + 机制」混合；A2 `升级建议生成路径单一化` 判 business **成立**（持续行为半边由红线 8 背书），但夹带一次性迁移记录；A3 `手动回填队列` / `Board embedding backfill API` 均为**可重复调用的幂等运维端点**（只补 NULL / 幂等重算），判 business 成立，但与 board-api 的「回填触发 API」跨族重复（R-3）；A4 `updateBoard refreshes embedding` 判 business **成立**（红线 35 背书的正确性契约），但红线 35 要求的「同一请求内失效 board embeddings 匹配缓存」无条款，只剩一句指向已退役 capability 的死引用（R-5）；A5 反向查出 2 处遗留施工记录 + 1 处实现约束混入 business（R-12、R-10） |
| B 归族边界 | 边界未贯彻，跨族重复未消 | B1 `board-editing-ui` 归 `board-api` **勉强成立**（71 行 4 条独立成族不成比例），但与 `board-diagnostics` 有实质重复：`getBoardArticles supports direction_mismatch filtering`（board-api:162）↔ `Board articles API direction_mismatch filtering`（board-diagnostics:552）、`API type updates` 的 `direction_sim` 字段（board-api:478）↔ `Match detail API returns direction_sim`（board-diagnostics:566）；B2 **升级建议 API 跨族重复成立且未消**，另有 3 组同族重复（R-3）；B3 **前端时间窗口选择器错位**——D3 自定「board-api 含前端交互契约」，但 board-upgrade 仍留 3 条前端契约（R-7） |
| C 归档可行性 | **不成立（阻断）** | 5 个被退役 spec 的 marker 全部必须摘（探针实测每个的 `unaccountedContent` 恰为那一行 marker），tasks 4.3 只写「4 个」；board-api 的 marker `semantic-board, api, ui` 与成员并集**完全吻合、无丢域**（management-api/article-api = `semantic-board, api`，editing-ui = `semantic-board, ui`），且 3 域恰在 J 段上限内；board-upgrade 就地扩容后 Purpose 不被 delta 覆盖已被 8.3b 覆盖，但 board-api 的 Purpose 断言错了（R-2） |
| D 遗漏 | 3 类缺口 | ①路径引用：`docs/reference/` 内 **0 处** `openspec/specs/<旧cap>` 路径引用，tasks 7.3 结论成立；②但 **capability 名引用有 4 处活链**（board-concept-management ×2、board-diagnostics ×1、board-upgrade ×1 自引用），本 change 亲手制造，tasks 无覆盖（R-5）；③tasks 缺失项见 R-1/R-2/R-3/R-5/R-13 |
| E 风险 | 可控但论据失实 | D6「本仓第一次把 `ui` 与 `api` 同时挂上」**不成立**（`client-poll-budget` 已是 `domain: api, ui`）；`ui`/`api` 是工具域，反查时 board-api 会成为 `ui` 域里唯一带版块契约的条目（R-8）。备选切法见 R-7 建议 |

红线复核（brief 附带的 1–8）：红线 1/2/3/4/5 的落点在 `tag-matching` / `tagging` / `auxiliary-label` / `board-diagnostics` 族，本次 6 个成员不含这些条款，合并前后不受影响。红线 6/7（suggestion_hash 幂等、dismiss 冷却）**只在 `board-upgrade` 的「co-tag 高频共现对产出组合标签建议」一条里以半句「复用现有机制」出现 + 2 个 compose 场景**，既无独立条款、也未覆盖 create_new / merge_into_existing 路径、未写 14 天默认值与冷却起算点——**覆盖不足，报 R-6（Medium）**。红线 8（watch 观察池退役）覆盖充分：「升级建议生成路径单一化」+ 3 个 Scenario（单例簇不产建议 / 存量 watch 清理 / 存量高置信合并保留）逐条对应。

## 发现

### R-1 2026-11-26【High】被退役的 capability 实为 5 个不是 4 个：tasks 4.3 少摘一个 marker，`openspec archive` 会整次中止且零文件改动
- **状态**：已修复
- **处置**：tasks 4.3/4.4/5.1/验收 C 与 proposal Impact 全部改为 **5 个**（upgrade-expand / time-window / management-api / article-api / editing-ui）；board-upgrade 就地扩容不退役
- **位置**：`openspec/changes/merge-board-upgrade-api-specs/tasks.md:29`（4.3）、`:31`（4.5）、`:37`（5.1）、`:73`（验收表）；`design.md` D5（摘 4 个）/ D7（4 个被删 capability）；change 目录下 `specs/`（REMOVED delta 实为 5 个目录）；`openspec/specs/{board-management-api,board-article-api,board-editing-ui,board-upgrade-expand,upgrade-candidate-time-window}/spec.md`
- **问题**：change 的 `specs/` 下有 5 个只含 `## REMOVED Requirements` 的 delta：`board-management-api`(15)、`board-article-api`(6)、`board-editing-ui`(4)、`board-upgrade-expand`(3)、`upgrade-candidate-time-window`(4) = 32 条。归档时每个都会被**删除 main spec**（退役），删除前必须由 `retire_capabilities: true` 授权，且该 spec 的全部内容必须「能被合并清点」——`<!-- domain: ... -->` 这一行属于**清点不到的内容**，留着就拒绝退役。但 tasks 4.3 与 design D5 都写「摘掉 **4 个**被退役 spec 的 `<!-- domain: -->`」：实施者照字面执行漏掉 1 个，openspec 抛 `ArchiveBlockedError`（`the spec holds content the merge cannot safely account for and deleting the file would take with it: "<!-- domain: semantic-board -->"`），**整个归档零文件改动**。连带数字全错：spec 总数应为 `156 − 5 + 1 = 152`（proposal 的「152」对，括号里「退役 4 个 + 新建 1 个」错；tasks 4.5 同一行先写「152 全过」又写「156 − 4 + 1 = 153」，下面「修正」段仍写 153）。
- **证据**：
  - `ls openspec/changes/merge-board-upgrade-api-specs/specs/` → board-api、board-upgrade、board-article-api、board-editing-ui、board-management-api、board-upgrade-expand、upgrade-candidate-time-window（7 个目录，其中 5 个只有 REMOVED）
  - 只读探针（openspec 自身 `buildUpdatedSpec` + `isRetirableSpec`，不落盘，`/tmp/probe/probe.mjs`）：5 个被退役 capability 均 `noRequirementBlocks=true`、`isRetirableSpec=true`，但 `unaccountedContent` 都是长度 1 的数组 —— `board-management-api → ["<!-- domain: semantic-board, api -->"]`、`board-article-api → ["<!-- domain: semantic-board, api -->"]`、`board-editing-ui → ["<!-- domain: semantic-board, ui -->"]`、`board-upgrade-expand → ["<!-- domain: semantic-board -->"]`、`upgrade-candidate-time-window → ["<!-- domain: semantic-board -->"]`
  - 源码：`@fission-ai/openspec/dist/core/specs-apply.js:642-700`（`contentTheMergeCannotName`：Purpose 之前的非标题行一律计入 leftovers）与 `dist/core/archive.js:1541-1551`（`retirementDeclared && unaccountedContent.length > 0 && isRetirableSpec` → `ArchiveBlockedError('archive_spec_validation_failed', …, 'No files were changed.')`）
  - 条数守恒（自行复核）：15/15、6/6、4/4、3/3、4/4，REMOVED 合计 32 = ADDED 32
  - `ls openspec/specs | wc -l` → 156；`openspec validate --specs` → `Totals: 156 passed, 0 failed`
- **建议**：tasks 4.3 改为「摘掉 **5 个**被退役 spec 的 `<!-- domain: -->`（board-management-api / board-article-api / board-editing-ui / board-upgrade-expand / upgrade-candidate-time-window），与 8.2 同一提交」；design D5 / D7、tasks 5.1、验收表 C 的「4 个」全改 5；spec 总数口径统一写 **152**（156 − 5 + 1），删掉 tasks 4.5 那句自相矛盾的「152 全过（… = 153）」与下方「修正」段；4.3 执行后加一步 `git diff --stat openspec/specs/` 确认 5 个文件的 marker 行都被摘掉再归档。

### R-2 2026-11-26【Medium】两族 delta 的 `## Purpose` 为空：board-api 主 spec 归档后落成 TBD 占位，tasks 8.3c 的断言不成立
- **状态**：已修复
- **处置**：`merge-spec-family.sh` 的 PURPOSE 字典补 `board-upgrade` 与 `board-api` 两族文案，重新生成后 delta Purpose 非空（board-upgrade 就地扩容仍以手改为准，tasks 8.3b）
- **位置**：`specs/board-api/spec.md:1-4`、`specs/board-upgrade/spec.md:1-4`；`tasks.md:60`（8.3c）；`design.md` D2/D5；`scripts/harness/merge-spec-family.sh:71-97`（`PURPOSE` 字典）、`:193`（`purpose = PURPOSE.get(fam, "")`）
- **问题**：两个 delta 的 `## Purpose` 下面只有空行。`merge-spec-family.sh` 的 `PURPOSE` 字典只有 `tagging` / `auxiliary-label` / `tag-matching` / `board-diagnostics` / `board-research` 五个键，**`board-upgrade` 与 `board-api` 都没登记**，delta 分支取 `PURPOSE.get(fam, "")` 得到空串。后果分两种：board-upgrade 是**已存在**的 capability，delta 的 Purpose 被忽略（8.3b 已安排手改，方向正确）；board-api 是**新建** capability，openspec 用 `buildSpecSkeleton(specName, changeName, deltaPurpose)` 生成骨架，Purpose 为空时直接写占位串 `TBD - created by archiving change merge-board-upgrade-api-specs. Update Purpose after archive.`。tasks 8.3c 却写「board-api 的 Purpose **由 delta 带入**，核验非空」——与事实相反，届时核验到的不是族级文案而是占位串。**同一脚本、同一作者、同一天的上一族 change（merge-board-diagnostic-specs）的 review R-1 正是这个问题，处置方式是给脚本 PURPOSE 字典补族级文案，本族漏做。**
- **证据**：
  - `sed -n '1,5p' openspec/changes/merge-board-upgrade-api-specs/specs/board-api/spec.md` → `## Purpose` + 3 空行 + `## ADDED Requirements`（board-upgrade 同）
  - 只读探针重建稿首 8 行（`/tmp/probe/probe2.mjs`）：`# board-api Specification` / `` / `## Purpose` / `` / `TBD - created by archiving change merge-board-upgrade-api-specs. Update Purpose after archive.` / `` / `## Requirements` / ``
  - 源码：`dist/core/specs-apply.js:248`（`deltaPurpose = extractPurposeSection(changeContent)`）、`:284`（`buildSpecSkeleton(specName, changeName, deltaPurpose)`）、`:1242-1248`（`purpose?.trim() || PURPOSE_PLACEHOLDER_PREFIX + changeName + PURPOSE_PLACEHOLDER_SUFFIX`）；常量见 `dist/core/validation/constants.js:17-18`
  - 对照上一族（已修）：`openspec/changes/archive/2026-11-26-merge-board-diagnostic-specs/specs/board-diagnostics/spec.md:3` 有族级 Purpose 文案；`scripts/harness/merge-spec-family.sh:85-96` 字典已补 `board-diagnostics` / `board-research`
  - 放行影响：`openspec validate --specs`（非 strict）对占位符只报 WARNING（`PURPOSE_IS_PLACEHOLDER` 在 constants 的「Warnings」段），不阻断归档 → 定 Medium 而非 High
- **建议**：在 `merge-spec-family.sh` 的 `PURPOSE` 字典补 `board-upgrade` 与 `board-api` 两段族级文案（board-upgrade：升级建议管线 = 候选收集 → 时间窗 → 预聚类 → LLM 裁决 → 用户确认 → 冷启动，含创建/扩充两方向与单标签/组合标签两来源；board-api：对外契约 = 版块 CRUD/配置、版块文章列表（筛选/分页/归档口径/按日计数/AI 判定）、升级建议读写与面板交互），重跑脚本让两个 delta 的 Purpose 非空；tasks 8.3c 改为「board-api 的 Purpose 由 delta 带入，核验**非 TBD 占位**（`grep -c 'TBD - created by archiving' openspec/specs/board-api/spec.md` == 0）」；8.3b 同样补一条「board-upgrade 的 Purpose 手改后 grep 确认不含占位串」。

### R-3 2026-11-26【Medium】跨族重复一条未消：升级建议 API / 确认执行 / 回填 / 面板四组功能各落两族，改接口仍要开两个文件，与本 change 的核心判据冲突
- **状态**：已采纳
- **处置**：design 新增 D7「已知跨族重复 4 组」表（触发/生成、确认执行、回填、面板各列两族落点）+ 分工约定（改接口只看 board-api、改算法只看 board-upgrade）+ 收口登记 tasks 7.5。本 change 不改正文以保 32=32 守恒
- **位置**：`specs/board-api/spec.md:58`（升级建议 API）、`:74`（升级建议 DTO）、`:104`（升级建议面板支持逐项处理和重新生成）、`:122`（回填触发 API）、`:136`（Board embedding backfill API）、`:147`（Board rematch API）；`openspec/specs/board-upgrade/spec.md:66`（用户确认后执行升级建议）、`:81`（ConfirmSuggestion API 合并路径保持不变）、`:88`（手动回填队列）、`:224`（前端渲染 compose 建议）、`:265`（生成入口模式选择）；`design.md` D1
- **问题**：proposal 的判据是「改升级阈值/冷却期/召回策略 → 只碰 `board-upgrade`；改接口字段/分页/前端交互 → 只碰 `board-api`」。但升级建议这条链路恰恰两边都落：①**触发/生成**——board-api「升级建议 API」写「系统 SHALL 提供查看当前升级候选（ref_count ≥ 5 的辅助标签 + 聚类结果）和触发 LLM 升级建议的 API」+「执行聚类 + LLM 判断流程，返回建议结果供用户确认/拒绝」，与 board-upgrade 的「辅助标签聚类升级触发」「预聚类压缩候选」「LLM 判断升级/跳过」是同一管线的两种说法；②**确认执行**——board-api DTO 的 Scenario「确认执行升级建议 → 写入 SemanticBoard 和 board_composition」与 board-upgrade「用户确认后执行升级建议」「ConfirmSuggestion API 合并路径保持不变」重复；③**回填**——board-api「回填触发 API」（all/unassigned/board 三模式 + 进度查询）与 board-upgrade「手动回填队列」（同一组模式 + 入队）是同一功能的两条；④**同一个升级建议面板**——board-api「升级建议面板支持逐项处理和重新生成」与 board-upgrade「生成入口模式选择」「前端渲染 compose 建议」是同一个面板的三条契约，分在两族。结果：改升级建议端点的字段/参数，仍要同时开 board-upgrade 与 board-api 两个文件，**这正是本 change 声称要消除的碎片**。tasks 3.3「检查跨 spec 重复」已勾选 [x]，3.4 只登记了「升级建议 API 在两族各有描述」一条，未列具体落点、未给处置。
- **证据**：
  - board-api delta L58-72（升级建议 API）与 board-upgrade main L9-19 / L20-29 / L38-48（触发 / 预聚类 / LLM 判断）文本对照
  - board-api delta L96-99（Scenario 确认执行升级建议）与 board-upgrade main L66-79、L81-86 对照
  - board-api delta L122-134（回填触发 API，all/unassigned/board + total/processed/failed/status）与 board-upgrade main L88-94（手动回填队列，同一组模式 + 异步逐个执行）对照
  - board-api delta L104-120（升级建议面板）与 board-upgrade main L224-233、L265-278 对照
- **建议**：二选一并写进 design D1/D7「已知跨族重复」段：①**保留现状 + 登记**——明确这 4 组为「算法侧落点」与「接口侧落点」的分工，规定接口字段只改 board-api、算法判定只改 board-upgrade，并在 board-api 的 4 条重复 Requirement 末尾各加指针（`> 算法口径见 board-upgrade 的〈X〉`），把 3.4 的登记升级为可执行清单；②**收口**——把 board-api 的「升级建议 API」/DTO 里的生成侧描述降为指针（只留端点、DTO 字段、面板行为），把 board-upgrade 的「手动回填队列」并入 board-api 的「回填触发 API」，但方案 ② 会改正文、打破本 change「不改 spec 内容」的守恒前提（32=32），须另立 change 做。

### R-4 2026-11-26【Medium】合并后 board-upgrade 族内同时出现两条互相矛盾的升级建议端点路径（`/upgrade/suggest` 与 `/upgrade-suggest`），代码里只有后者
- **状态**：已采纳
- **处置**：两条矛盾端点路径属原 spec 既有内容问题，合并后同族更显眼；登记 tasks 7.5 与收口 change 同批修，本 change 不改语义
- **位置**：`openspec/specs/board-upgrade/spec.md:106-141`（`suggestUpgrades API 支持 mode 参数`，写 `POST /api/semantic-boards/upgrade/suggest`）；`specs/board-upgrade/spec.md:122-130`（`升级建议 API 支持时间窗口参数`，写 `POST /api/semantic-boards/upgrade-suggest`）；`specs/board-api/spec.md:116`（面板重新生成引用 `/upgrade-suggest`）；`design.md` D1/D2
- **问题**：合并前两条路径分处两个文件（`board-upgrade` 与 `upgrade-candidate-time-window`），矛盾是跨文件的；合并后 `upgrade-candidate-time-window` 的「升级建议 API 支持时间窗口参数」被追加进 **board-upgrade 自己**，矛盾变成**族内自相矛盾**：同一族里第 108 行说端点是 `/upgrade/suggest`（带 direction/source/target_board_id），第 399 行说是 `/upgrade-suggest`（带 days）。参数集也各说各的——direction/source/target_board_id 在前一条、days 在后一条，读者无法判断 `days` 传给哪个端点。代码只有 `/upgrade-suggest`（`board_crud_handler.go:138`），`/upgrade/suggest` 不存在——即 board-upgrade 的原条款记的是**不存在的路径**（存量缺陷，非本次引入），但本次合并把它和正确路径塞进同一文件，使缺陷从「分散」变「显眼且无人认领」。
- **证据**：
  - 探针重建稿（`/tmp/probe/rebuilt-board-upgrade.md`，即归档后的 board-upgrade main spec）：L108/L119/L124/L128/L132 写 `/upgrade/suggest`；L399/L402 写 `/upgrade-suggest`
  - `grep -rn "upgrade-suggest\|upgrade/suggest" backend-go/ --include=*.go` → 仅 `backend-go/internal/tagmanagement/handler/board_crud_handler.go:138` 的 `boards.POST("/upgrade-suggest", handler.suggestUpgrades)`；`/upgrade/suggest` 无路由
  - `grep -n "upgrade-suggest\|upgrade/suggest" openspec/changes/merge-board-upgrade-api-specs/specs/board-api/spec.md` → L116（面板重新生成调用 `/upgrade-suggest`）
- **建议**：在 design D1 的「已知代价」里登记这条族内矛盾并明确处置口径：把 `suggestUpgrades API 支持 mode 参数` 的路径更正为 `/api/semantic-boards/upgrade-suggest`（与代码一致），并把 `days` 明确写成同一端点的可选参数——但这会改正文、破守恒，建议**登记为后续 change 的必做项**（与 R-3 的收口方案一起做）；本次至少在 tasks 里新增一条「4.x 记录 upgrade-suggest 端点路径两说 + 参数集分散」，避免下一波人误以为两族各自的参数清单是完整的。

### R-5 2026-11-26【Medium】退役 5 个 capability 会在 3 个 main spec 里留下 4 处活引用变死链，tasks 7.3 只查路径引用、不查 capability 名引用
- **状态**：已采纳
- **处置**：实测 3 处能力名引用（board-diagnostics:222、board-concept-management:5,26），非 4 处；登记 tasks 7.4「跨族引用清扫」change，与上族 tag-matching 死引用合并一批修（改 main spec 需 MODIFIED delta，会打破各族守恒账）
- **位置**：`openspec/specs/board-concept-management/spec.md:5`、`:26`（引用 `board-management-api`）；`openspec/specs/board-diagnostics/spec.md:222`（引用 `board-upgrade-expand`）；`openspec/specs/board-upgrade/spec.md:161`（引用 `board-upgrade-expand`，且该行落在**本次要重写的同一个文件**里）；`tasks.md:51`（7.3）
- **问题**：本 change 亲手退役 `board-management-api` / `board-upgrade-expand`，但 3 个 main spec 用**能力名**（不是路径）指着它们，归档后全部落空：①board-concept-management 的 Purpose 说「已被 `semantic-label-model` + `board-management-api` + `board-upgrade` 替代」，其第 26 行又说「该能力已由 `board-management-api` 中的板块 CRUD API 替代」——brief 只提到第 5 行，实际有 **2 处**；②board-diagnostics 第 222 行说「候选来源见 board-upgrade-expand 能力的扩充候选召回要求」，而那条要求**正是本次被吸收进 board-upgrade 的内容**（指针应改指 `board-upgrade`）；③board-upgrade 第 161 行说「召回规则见 board-upgrade-expand」，而这条规则在本 change 后**就在它自己文件里**（重建稿 L161 仍写着「见 board-upgrade-expand」，读者按图索骥找不到该能力）。tasks 7.3 只让检查 `openspec/specs/<旧cap>` 形式的**路径引用**，覆盖面不足；brief 也确认 `board-concept-management` 的 DEPRECATED 说明会变死引用——它不是「别人未提交的改动挡着」，而是**本 change 造成的**。附带事实：proposal 说 `board-concept-management`「有并发会话未提交的改动（marker 规范化）」，但该改动已在 `f199cae4`（2026-11-26，*chore(specs): 补齐剩余 3 个 main spec 的 domain 标记*）提交，`git status` 干净——**不动它的理由已不成立**。
- **证据**：
  - `grep -rn "board-management-api\|board-article-api\|board-editing-ui\|board-upgrade-expand\|upgrade-candidate-time-window" openspec/specs/*/spec.md` → 4 行命中：board-concept-management:5、board-concept-management:26、board-diagnostics:222、board-upgrade:161
  - `grep -n "board-upgrade-expand" /tmp/probe/rebuilt-board-upgrade.md` → L161（活引用）+ L282/L296/L359（`<!-- from: -->` 来源注释，属历史出处，可保留）
  - `grep -rn "board-management-api\|board-article-api\|board-editing-ui\|board-upgrade-expand\|upgrade-candidate-time-window" docs/reference/` → **0 命中**（tasks 7.3 的检查结论成立）
  - `git log -1 --format='%H %ad %s' -- openspec/specs/board-concept-management/spec.md` → `f199cae4 2026-11-26 chore(specs): 补齐剩余 3 个 main spec 的 domain 标记（并把 CRLF 归一为 LF）`；`git status --porcelain` → 空
- **建议**：新增 task「7.4 归档同一提交修正 4 处 capability 名活引用」：`board-concept-management:5/26` 的 `board-management-api` → `board-api`（并按实际内容核对措辞）；`board-diagnostics:222` 的 `board-upgrade-expand` → `board-upgrade`；`board-upgrade:161` 的「召回规则见 board-upgrade-expand」→「见本 spec 的〈扩充候选召回〉」（自引用）。同时把 proposal「不做的事」里 `board-concept-management` 那条的理由从「并发会话有未提交改动」改为「本 change 不动它，但归档同一提交须修它的 2 处死引用」。

### R-6 2026-11-26【Medium】红线 6/7 的 spec 覆盖不足：只有 compose 场景下的半句「复用现有机制」，未覆盖 create_new / merge_into_existing、未写 14 天默认值与冷却起算点
- **状态**：已采纳
- **处置**：design 新增 D8：列出覆盖不足的 4 点（只覆盖 compose / 无 14 天默认值 / 无冷却起算点 / 指向已删旧条款）+ 代码侧已完整实现的事实；补独立条款登记 tasks 7.5
- **位置**：`openspec/specs/board-upgrade/spec.md:157-191`（「co-tag 高频共现对产出组合标签建议」内的 L163 半句 + Scenario「同 hash 建议幂等」L187-189 +「dismissed 冷却期内拦截」L191-193）；代码侧 `backend-go/internal/tagmanagement/repository/board_upgrade_suggestion.go:64-70`（冷却查询）、`:28-41`（`InsertPending` 的 `ON CONFLICT DO NOTHING` + `uq_board_upgrade_suggestions_hash`）、`backend-go/internal/tagmanagement/service/board/board_upgrade_suggestion_persist.go:35-72`（冷却闸 + skipped 判定）、`:115-127`（`LoadDismissCooldownDays`，默认 14）
- **问题**：brief 的待裁决问题——**覆盖不够**。三条理由：①**适用范围被窄化**：唯一那半句写在「co-tag 高频共现对产出组合标签建议」里，读者按标题检索「幂等 / 冷却」只会落到 compose 上，会以为 create_new / merge_into_existing 不受约束；而代码层的幂等与冷却闸是**全 decision 通用**的（`GenerateAndPersist` 对每条非 skip 建议统一算 hash → 查冷却 → `InsertPending`）；②**关键参数缺失**：条款只写「冷却期」，没写红线 7 明确要求的**默认 14 天**，也没写冷却期**自 dismiss 时刻起算**（代码是 `resolved_at >= NOW() - N day`，`resolved_at` 由 `MarkDismissed` 写入）；③**配置面缺失**：冷却天数在代码里可配（`semantic_board_upgrade_suggestion_dismiss_cooldown_days`，缺省 14），spec 无对应条款（红线 6/7 之外，红线 5 的阈值族同样有配置面，那些在 board-api「匹配参数配置 API」里有落点——冷却期没有）。
- **证据**：
  - `grep -rn "suggestion_hash" openspec/specs/*/spec.md` → **仅 1 处**：`openspec/specs/board-upgrade/spec.md:163`（半句「suggestion_hash 幂等与 dismissed 冷却期复用现有机制（扩充方向 hash 计算包含 target_board_id）」）
  - 同 spec 的 Scenario「同 hash 建议幂等」限定在「compose 建议」；「dismissed 冷却期内拦截」限定在「某 compose 建议」
  - 代码：`board_upgrade_suggestion_persist.go:41-51`（冷却闸，对所有非 skip 建议生效）、`:66-71`（`InsertPending` 返回 false → `skipped++`）、`:119`（`const defaultDays = 14`）、`:121`（配置键 `semantic_board_upgrade_suggestion_dismiss_cooldown_days`）；`board_upgrade_suggestion.go:67`（`resolved_at >= NOW() - (? * INTERVAL '1 day')`）；`backend-go/internal/platform/database/postgres_migrations.go:1384`（`CREATE UNIQUE INDEX IF NOT EXISTS uq_board_upgrade_suggestions_hash`）
  - 红线 8 覆盖对照（成立）：`openspec/specs/board-upgrade/spec.md:239-252`（「升级建议生成路径单一化」+ 3 个 Scenario 逐条对应）
- **建议**：本次是纯结构重构，直接补条款会把 ADDED 从 32 变 33、打断本 change 精心维持的「32 = 32 / 48 = 32 + 16」守恒，因此建议**登记后续 change**（在 tasks 里新增一条，明确承接方与验收口径），补一条独立 Requirement「升级建议幂等与冷却期」，内容至少包含：(1) 全部 decision（create_new / merge_into_existing / compose）的建议按 `suggestion_hash` 幂等，同 hash 已有 pending 行则 `skipped` 不入库（唯一 partial unique index `uq_board_upgrade_suggestions_hash ... WHERE status='pending'`）；(2) 同 hash 的 dismissed 建议在冷却期（**默认 14 天**，可配 `semantic_board_upgrade_suggestion_dismiss_cooldown_days`）内不得重新生成，冷却期自 dismiss 的 `resolved_at` 起算，超期才可重生；(3) 扩充方向的 hash 计算包含 `target_board_id`（现条款已覆盖，保留）。对应 Scenario：同 hash create_new 被跳过、dismiss 后 15 天内同 hash 不生成、冷却期改 7 天后第 8 天可重生。若坚持本次补条款，须同步改 tasks 1.1 / 2.3 / 9 的守恒断言（ADDED 33 = REMOVED 32 + 1 条新）。

### R-7 2026-11-26【Medium】前端契约归族不一致：D3 自定「board-api 含前端交互契约」，但 board-upgrade 仍留 3 条前端契约，「前端时间窗口选择器」留在 board-upgrade 属错位
- **状态**：已修复
- **处置**：design 新增 D9「前端契约归族判据」：前端控件配置算法 → 归算法族（时间窗口选择器留 board-upgrade），展示接口数据 → 归接口族（编辑对话框归 board-api）；与 D1 同源
- **位置**：`design.md` D3（board-api 的三类对外契约之三 =「前端交互契约：editing-ui（对话框字段、direction_mismatch 默认隐藏、MatchDetailPanel 展示）」）；`specs/board-upgrade/spec.md:132-148`（前端时间窗口选择器，来自 upgrade-candidate-time-window）、`openspec/specs/board-upgrade/spec.md:224-233`（前端渲染 compose 建议）、`:265-278`（生成入口模式选择）；`specs/board-api/spec.md:104`（升级建议面板支持逐项处理和重新生成）、`:414`（Board editing dialog）、`:448`（Default hide direction_mismatch tags）、`:463`（MatchDetailPanel direction check display）、`:478`（API type updates）
- **问题**：D3 给 board-api 的定位里明确包含「前端交互契约」，但合并后**同一种契约（前端面板/对话框/展示规则）被按主题拆在两族**：board-api 收 5 条前端契约，board-upgrade 留 3 条（其中「前端时间窗口选择器」正是本次新吸收进来的）。B3 的问法——「前端时间窗口选择器」放在 board-upgrade 是否错位——按 D3 自己写的口径，**是错位**：它与 board-api 的「升级建议面板支持逐项处理和重新生成」描述的是**同一个升级建议弹窗**（前者管参数区下拉，后者管列表区逐项处理与重新生成），一个在算法族、一个在接口族。更严重的是三处面板契约跨族并存：「前端时间窗口选择器」+「生成入口模式选择」+「前端渲染 compose 建议」（board-upgrade） vs 「升级建议面板支持逐项处理和重新生成」（board-api）——**同一个面板被切成两族**，change 的判据「改前端交互 → 只碰 board-api」对升级建议面板不成立。
- **证据**：
  - `grep -n '^### Requirement:' openspec/changes/merge-board-upgrade-api-specs/specs/board-upgrade/spec.md` → 含 `前端时间窗口选择器`（L132）；`openspec/specs/board-upgrade/spec.md:224` `前端渲染 compose 建议`、`:265` `生成入口模式选择`
  - `grep -n '^### Requirement:' openspec/changes/merge-board-upgrade-api-specs/specs/board-api/spec.md` → `升级建议面板支持逐项处理和重新生成`(104)、`Board editing dialog`(414)、`Default hide direction_mismatch tags`(448)、`MatchDetailPanel direction check display`(463)、`API type updates`(478)
  - 语义重叠：`specs/board-upgrade/spec.md:136`（时间窗下拉在「升级建议弹窗的生成参数区」）vs `specs/board-api/spec.md:106`（升级建议面板逐项处理）vs `openspec/specs/board-upgrade/spec.md:265`（升级建议面板两步模式选择）
- **建议**：二选一并写进 design D3：①**口径收敛到「族按能力域切、不按契约形态切」**——承认两族都含前端契约，把 D3 的第 3 类（前端交互契约）从 board-api 的定位里去掉，改写成「board-api = 版块域对外契约（HTTP + 前端消费）」，并在 design D1 的判据里补一句「前端契约按业务域归属，不单独成族」；②**把 3 条前端契约整体搬到 board-api**（board-upgrade 只留算法与管线），会使 board-upgrade 变纯粹、board-api 变 28 条，但同样要改正文、破守恒，须另立 change。无论选哪条，design.md 都必须显式回答 B3，并把「前端时间窗口选择器」的归属写进理由（现在的 D3 文字与实际产物相反）。

### R-8 2026-11-26【Low】design D6「本仓第一次把 `ui` 与 `api` 同时挂上」不成立（`client-poll-budget` 已是 `domain: api, ui`）；`ui`/`api` 是工具域，域语义会被稀释
- **状态**：已修复
- **处置**：design D6 更正：并非本仓首次（client-poll-budget 已是 api, ui）；并写明代价（工具域纯度被稀释，后续族无同样跨度不应照抄）
- **位置**：`design.md` D6（`board-api` → `semantic-board, api, ui`，「这是本仓第一次把 `ui` 与 `api` 同时挂上，原因是这族确实横跨前后端边界」）；`openspec/specs/client-poll-budget/spec.md:1`（`<!-- domain: api, ui -->`）
- **问题**：D6 用「本仓首次」作为保留三个域的论据之一，但仓库里已有先例：`client-poll-budget` 的 marker 就是 `domain: api, ui`。因此「首次」这一新颖性论据不成立（论据本身——「改这族必须读哪些」——仍成立，但表述要改）。另一处风险：`ui` / `api` 在 `domain-history.sh` 里属**工具域**（`TOOL_DOMAINS="harness ui dev api db"`，无对应 flow 文档、无线约束正文），`board-api` 挂上后，`domain-history.sh ui` / `api` 反查会首次出现「版块/升级建议族」这种业务型条目，工具域语义会混入业务族条目；`constraint-injection` 也会把 `api` 判为「未知域」忽略（本次注入原文即「忽略未知域：api」），说明 marker 的域在约束通道里并不承载约束正文——D6 说这两个次域是「改这族必须读哪些」的真实来源，与约束注入的实际行为并不一致（注入只送红线，不按 marker 路由文档）。
- **证据**：
  - `for f in openspec/specs/*/spec.md; do m=$(sed -n '1,8{/<!--[[:space:]]*domain:/{p;q}}' "$f"); case "$m" in *ui*api*|*api*ui*) echo "$f: $m";; esac; done` → `openspec/specs/client-poll-budget/spec.md: <!-- domain: api, ui -->`（唯一先例）
  - `grep -l "domain:.*\bui\b" openspec/specs/*/spec.md | wc -l` → 25；`grep -l "domain:.*\bapi\b" openspec/specs/*/spec.md | wc -l` → 7
  - `scripts/harness/domain-history.sh:31-32` → `BUSINESS_DOMAINS`（= flow basename）与 `TOOL_DOMAINS="harness ui dev api db"`；本次注入正文写「声明域：semantic-board（忽略未知域：api）」
  - marker 合法性本身成立：`scripts/harness/check-standards.sh` J 段词表含 `ui`/`api`，且限 3 个域 → `semantic-board, api, ui` 恰 3 域、**成员并集零丢域**（management-api/article-api = `semantic-board, api`，editing-ui = `semantic-board, ui`）
- **建议**：design D6 删掉「本仓第一次」这一论据，改用「这族横跨前后端边界，两个次域是改这族必须同步的合同面」；并在 D6 补一句「`ui`/`api` 为工具域，域反查只用于定位、不承载约束正文」，避免下一波人误以为挂上 marker 就能拿到约束注入。

### R-9 2026-11-26【Low】行数口径混用：proposal/tasks 的 439 / 430 是「来源 spec 行数之和」，归档后实测为 444 / 481；tasks 2.2 的 490 也不是 delta 实测
- **状态**：已修复
- **处置**：tasks 行数口径注明「来源 spec 之和」；design 表格保留三口径（来源 439/430、delta 491/490、归档后待实测）
- **位置**：`proposal.md`「What Changes」表（board-upgrade 439 行 / board-api 430 行）；`tasks.md:7`（1.1）、`:14`（2.2）、`:66-74`（第 9 节验证表）
- **问题**：439 = 279+90+70（三个来源 spec 行数之和，`wc -l +1` 口径），430 = 211+148+71，同样是来源之和，不是族稿/归档后的实际体量。实测归档重建稿为 **board-upgrade 444 行、board-api 481 行**（`from` 注释、空行、新建 capability 的 `# board-api Specification` 标题与 Purpose 骨架都在里面，board-api 被放大 51 行 ≈ 11%）。tasks 2.2 写「board-api 490 行」，与 delta 文件实测 488 行、族稿 490 行都不对应（490 可能是脚本第一次生成的中间产物行数）。这类数字在归档核对时会被当断言值用。
- **证据**：
  - 探针重建稿行数：`/tmp/probe/rebuilt-board-upgrade.md` → 444 行（23 req / 74 sc）；`/tmp/probe/rebuilt-board-api.md` → 481 行（25 req / 70 sc）
  - delta 实测：`specs/board-upgrade/spec.md` → 170 行（7 req / 27 sc）；`specs/board-api/spec.md` → 488 行（25 req / 70 sc）
  - 来源 spec：`wc -l openspec/specs/{board-upgrade,board-upgrade-expand,upgrade-candidate-time-window,board-management-api,board-article-api,board-editing-ui}/spec.md` → 278/89/69/210/147/70（+1 即 279/90/70/211/148/71，合计 869，与 proposal 的 Why 表一致）
  - 场景守恒复核：47（board-upgrade 原有）+ 27（新增）= 74 ✓；70（board-api）✓；合计 144 ✓
- **建议**：统一成三口径并列（与上一族 merge-board-diagnostic-specs 的 review R-4 同口径）：「来源 spec 869 行 → 族稿 delta 170 / 488 行 → 归档后 main spec 444 / 481 行」；tasks 2.2 的「490」改成实测值；第 9 节验证表里若有行数断言同步改。

### R-10 2026-11-26【Low】business/impl 判据未成文（D4 只列候选不给判据），且族内多条 business 直接点名 SQL / Go 内部实现，容易被下一波人按「点名实现即 impl」误翻
- **状态**：已修复
- **处置**：design D4 补 business/impl 判据表 + 明确「点名 SQL/Go 符号是定位信息，不构成 impl 判据」
- **位置**：`design.md` D4；`spec-classification.txt`（48 行纯 business/impl，无标题映射）；`specs/board-api/spec.md:294`（分页和排序）、`:349`（版块文章按日计数）、`:162`（getBoardArticles supports direction_mismatch filtering）、`:147`（Board rematch API）
- **问题**：D4 给了 3 条「预判候选」却没给**判据**，而唯一 impl（`API type updates`）恰好落在最易误判的一侧：它只描述 TS 类型字段新增，无 SHALL/MUST（openspec validate 也正好警告这条「should contain SHALL or MUST」），判 impl 站得住。但同族多条判 business 的条款同样点名实现细节：「分页和排序」写「排序在 Go 端内存中完成」、「版块文章按日计数」点名 `article_topic_tags ⋈ topic_tag_board_labels`、`feeds` 内连接、`DATE(articles.pub_date)` 归日与去重口径、「getBoardArticles…」Scenario 写 `filtered_tags SQL adds AND NOT COALESCE(tbl.direction_mismatch, false)`、「Board rematch API」点名内部函数 `MatchTopicTag`。若有人用「点名实现即 impl」这条直觉判据复核，会把半族翻成 impl。真正区分 impl 的判据是「**只描述实现载体（类型定义/字段名/代码组织），不含可观测行为契约**」——`API type updates` 恰是唯一一条。`spec-classification.txt` 只给 48 行 business/impl 而**不带标题映射**，复核者无法从分类文件反查第 48 条是「API type updates」还是别的（须自行复原 6 个 capability 的拼接顺序 [board-upgrade 16 / expand 3 / time-window 4 / management-api 15 / article-api 6 / editing-ui 4]，再取偏移 25 与 0）。
- **证据**：
  - `grep -c "kind: impl" openspec/changes/merge-board-upgrade-api-specs/specs/*/spec.md` → 仅 `board-api/spec.md:481`（`API type updates`）一处；board-upgrade delta 0 处
  - `openspec validate merge-board-upgrade-api-specs` → `Change 'merge-board-upgrade-api-specs' is valid`，其中 3 条 WARNING 精确指向 editing-ui 的 3 条（`Default hide direction_mismatch tags` / `MatchDetailPanel direction check display` / `API type updates` 缺 SHALL/MUST）
  - `spec-classification.txt`：`sed -n '1,3p;48p'` → `1 business` / `2 business` / … / `48 impl`，全文无标题
  - 实现细节原文：`specs/board-api/spec.md:303`（排序在 Go 端内存中完成）、`:360-364`（点名表与 JOIN）、`:170`（SQL 条件）、`:151`（`MatchTopicTag`）
- **建议**：design D4 补判据原文：「impl = 只描述实现载体（类型定义、字段名、代码组织）而不含可观测行为契约；点名表/SQL/内部函数但同时规定外部可观测结果的，仍为 business」，并把 A1/A2/A3/A4 的逐条裁定写进 D4；`spec-classification.txt` 增列「序号 → Requirement 标题」映射（48 行），让复核不必复原拼接顺序。

### R-11 2026-11-26【Low】board-api 族内两条 Requirement 逐字重复了两个 Scenario（「升级建议 API」与「升级建议 DTO」），合并前在源 spec 已有、合并后同族同文件更显眼
- **状态**：已采纳
- **处置**：两条 Requirement 逐字重复 Scenario 属源 spec 既有，登记 tasks 3.4d + 7.5 收口批次
- **位置**：`specs/board-api/spec.md:58-72`（升级建议 API）与 `:74-102`（升级建议 DTO 包含标签名称和板块名称）——Scenario「查看升级候选」与「触发 LLM 升级建议」两条在两处逐字重复
- **问题**：源 spec `board-management-api` 里这两条 Scenario 就各出现两次（「升级建议 API」下有「查看当前升级候选 / 触发 LLM 升级建议」，DTO 下又有一份「查看升级候选 / 触发 LLM 升级建议」，仅 Scenario 名少了「当前」二字）。本次逐字搬运把它原样带进同一族同一文件（中间只隔 2 行 Requirement 标题），读者会以为系统有两个「查看候选」端点。本次不改内容的原则下它无害，但属于应登记的重复项，且 tasks 3.4 的登记清单里没有它。
- **证据**：`grep -n "查看.*升级候选\|触发 LLM 升级建议" openspec/changes/merge-board-upgrade-api-specs/specs/board-api/spec.md` → L64/L86（两条 Requirement 各一份）；源 spec `openspec/specs/board-management-api/spec.md:54/59/79/83` 同形
- **建议**：登记进 tasks 3.4 的重复项清单（与 R-3 一起），或另立小 change 去重（去重会改 Scenario 文字，需同步 REMOVED/ADDED 守恒断言）。

### R-12 2026-11-26【Low】board-upgrade 保留条款里仍有两条一次性迁移记录（watch 清理 / discover_new 置 dismissed），判 business 可接受但应登记
- **状态**：已采纳
- **处置**：两条一次性迁移记录（watch 清理 / discover_new 置 dismissed）登记 tasks 3.4e；判 business 可接受因为同条含持续行为
- **位置**：`openspec/specs/board-upgrade/spec.md:239-252`（「升级建议生成路径单一化」：「存量 pending 的 watch 建议 SHALL 被一次性清理（删除行）」+ Scenario「存量 watch 建议被清理」）、`:165`（「存量 pending 的 discover_new 管线建议一次性置 dismissed（迁移 20260907_0001，旧 hash 格式永不被新幂等命中，永久混入列表干扰判断）」）
- **问题**：A2/A5 的直接答复——这两条是**一次性施工/迁移记录**而非持续行为契约（「本变更部署后执行数据清理」「迁移 20260907_0001」），混在 business Requirement 里，读者无法判断它是「每次运行都要做」还是「部署时做过一次」。判 business 仍可接受（前半段的持续行为「SHALL NOT 生成 watch 观察池建议」是红线 8 的正面表述，Scenario「单例簇不产建议」逐条对应红线 8），但**一次性迁移这一面应显式登记**，否则会被误读为可重复触发的不变量。
- **证据**：`sed -n '239,252p' openspec/specs/board-upgrade/spec.md`；`grep -n "20260907_0001" openspec/specs/board-upgrade/spec.md` → 1 处
- **建议**：在 tasks 3.4 的登记清单里加一条「board-upgrade 保留条款含 2 处一次性迁移记录（watch 清理、20260907_0001 置 dismissed），建议后续 change 迁到迁移记录文档」；本次不改（守恒与红线 8 覆盖都不受影响）。

### R-13 2026-11-26【Low】review 痕迹缺失：tasks 6.1/6.2 已勾选「已派 review 并处置」，但 change 目录没有 review.md，归档门禁④当前是红的
- **状态**：已修复
- **处置**：审查报告已落盘为本文件（13 条），`review-trace.sh` 通过
- **位置**：`tasks.md:44-45`（6.1 / 6.2 均已勾选）；change 目录（缺 review.md）；`scripts/harness/review-trace.sh`（门禁④）
- **问题**：tasks 声明已派 `zai-coding-cn/glm-5.4:max` 做 review 且「处置 review 发现」，但 change 目录只有 design/proposal/tasks/ui-design/spec-classification/specs/.openspec.yaml，**没有 review.md**——前一轮 review 的痕迹没有落盘。按 `review-trace.sh` 的判定（③ 两者皆无 → 退出码 1 + 「零痕迹」），`archive-readiness.sh` 的第 ④ 项现在就会红。
- **证据**：`bash scripts/harness/review-trace.sh openspec/changes/merge-board-upgrade-api-specs` → `✗ 未通过：缺 review.md 建账与 <!-- review: waived --> 豁免声明（零痕迹）`，exit=1；`ls -a openspec/changes/merge-board-upgrade-api-specs/` → design.md / .openspec.yaml / proposal.md / spec-classification.txt / specs / tasks.md / ui-design.md（无 review.md）
- **建议**：把本报告全文存为 `openspec/changes/merge-board-upgrade-api-specs/review.md` 即闭合（本线程为只读审查且未获写文件工具，故未代存）；若后续再派新一轮 review，请把每轮结论都写进同一份 review.md（R-N 递增），不要只勾 tasks。

## 核实记录

全部为只读命令；探针脚本写在 `/tmp/probe/`（仓库外），未改动任何仓库文件（`git status --porcelain` 为空）。

| 核查项 | 命令 / 方法 | 结果 |
|---|---|---|
| 变更可解析 | `openspec validate merge-board-upgrade-api-specs` | exit=0，`Change 'merge-board-upgrade-api-specs' is valid`，7 条 WARNING（3 条指向 editing-ui 缺 SHALL/MUST、2 条超长正文） |
| 全仓基线 | `openspec validate --specs` | `Totals: 156 passed, 0 failed (156 items)` |
| ADDED 逐字一致 | 自写比对脚本（32 块去掉 `from`/`kind` 注释与多余空行后与源 spec 逐字比） | 32/32 逐字一致，**0 处差异**；board-upgrade 7 条 + board-api 25 条 |
| REMOVED 条数/标题 | 5 个 REMOVED delta 的标题 vs 各自 main spec | 15/15、6/6、4/4、3/3、4/4，合计 32，标题逐字匹配、无漏无多 |
| 场景守恒 | delta 与重建稿计数 | board-upgrade 47+27=74；board-api 70；合计 144 ✓ |
| 退役数与 marker 阻塞 | openspec 自身 `buildUpdatedSpec` + `isRetirableSpec` 探针（不落盘） | 5 个被退役 capability 的 `unaccountedContent` 均只含那一行 `<!-- domain: ... -->` → 少摘 1 个即 `ArchiveBlockedError`（零文件改动） |
| 归档后产物形态 | 同探针取重建稿 | board-upgrade 444 行 / 23 req / 74 sc（Purpose 保留旧文案、族内并存两条端点路径）；board-api 481 行 / 25 req / 70 sc，**Purpose = `TBD - created by archiving change merge-board-upgrade-api-specs. Update Purpose after archive.`** |
| marker 并集 | `sed -n '1,8{…}'` 取 3 个成员 marker | `semantic-board, api` + `semantic-board, api` + `semantic-board, ui` → 并集 `semantic-board, api, ui`，**恰 3 域、零丢域**，在 check-standards J 段词表内 |
| 端点路径真伪 | `grep -rn "upgrade-suggest\|upgrade/suggest" backend-go/ --include=*.go` | 代码只有 `/upgrade-suggest`（`board_crud_handler.go:138`）；spec 里两条路径并存（board-upgrade 重建稿 L108 vs L399） |
| 红线 6/7 代码侧 | `board_upgrade_suggestion.go:34/67`、`board_upgrade_suggestion_persist.go:35-72/115-127`、`postgres_migrations.go:1384` | 幂等=partial unique index + `ON CONFLICT DO NOTHING`；冷却=全 decision 生效、默认 14 天、可配 `semantic_board_upgrade_suggestion_dismiss_cooldown_days`、自 `resolved_at` 起算 |
| 红线 6/7 spec 侧 | `grep -rn "suggestion_hash" openspec/specs/*/spec.md` | **仅 1 处**（board-upgrade:163 半句，compose 场景内），无独立条款 |
| 红线 8 覆盖 | `sed -n '239,252p' openspec/specs/board-upgrade/spec.md` | 3 个 Scenario 逐条对应，覆盖充分 |
| 旧 capability 引用 | `grep -rn "<旧cap名>" openspec/specs/*/spec.md` / `docs/reference/` | main spec **4 处活引用**（board-concept-management ×2、board-diagnostics ×1、board-upgrade ×1 自引用）；`docs/reference/` **0 处** |
| `ui`+`api` 先例 | 遍历所有 main spec marker | `client-poll-budget` 已是 `<!-- domain: api, ui -->`（D6「本仓首次」不成立） |
| domain 词表 | `domain-history.sh:31-32`、`check-standards.sh` J 段 | `ui`/`api` 属工具域（无 flow 文档、无线约束正文），限 3 域 → marker 合法 |
| 脚本 PURPOSE 字典 | `sed -n '71,97p' scripts/harness/merge-spec-family.sh` | 只登记 tagging/auxiliary-label/tag-matching/board-diagnostics/board-research；`board-upgrade`/`board-api` 缺失 → delta Purpose 空 |
| 并发改动状态 | `git log -1 -- openspec/specs/board-concept-management/spec.md`、`git status --porcelain` | 改动已在 `f199cae4`（2026-11-26）提交，工作区干净 → proposal「并发会话未提交改动」的理由已过期 |
| review 痕迹 | `bash scripts/harness/review-trace.sh <change>` | 报告存入前 exit=1「零痕迹」；按本报告存盘后应转绿（见 R-13） |
