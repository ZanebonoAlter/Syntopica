# Review — retire-heuristic-tag-fallback

> 审查类型：制品 + diff 聚焦审查（只读）。审查范围：`proposal.md` / `design.md` / `tasks.md` /
> `test-cases.md` / `specs/tagging-domain/spec.md` / `specs/auxiliary-label/spec.md`
> + commit `2f6c55d1`（代码）/ `9e9bb417`（文档）/ `af286985`（清理脚本与执行记录）
> + `scripts/db/cleanup-heuristic-tags-20261112.sql`。
> 审查日期：2026-11-12。结论按 High/Medium/Low 分级；**处置（修/不修）由主线程拍板后回填本表**。

## 0. 总评

- **核心承诺「挂载行为不变」成立**：三个提交只触及 `internal/tagmanagement` + `internal/models` 注释 +
  `internal/platform/aihealth` 注释，`semantic_board_matching.go`（`min(2,N)` 放宽准入、`max_sim` 判定）
  与日报消费侧代码**逐字未动**；`aggregate → mono` 回落编排（`article_tagger_aggregate.go`）**逐字未动**，
  只有 mono 侧的提取错误分支由「兜底写泛词」改为「上抛错误」。
- **失败语义闭环成立**：`ExtractTags`（`extractor_enhanced.go:53`）→ `tagArticle`（`article_tagger.go:197`）
  → `tag_queue.go:306 MarkFailed` → `tag_job_queue.go:160`（pending + `available_at=now+backoff`，
  `attempt_count >= max_attempts` → `failed`）→ `Claim` 重新领取，无一环吞错。
- **`ExtractionResult.Source` 删除安全**：全仓仅 `extractor_enhanced.go` 内部构造，无外部消费方；
  `topic_tags.source` 写点收敛为 `llm` / `manual` / `llm_suggest`，`article_topic_tags.source` 实测只剩
  `llm`(16802) / `reuse`(11)。
- **测试没有被改弱**：4 处「降级 heuristic」断言全部换成更强的「返错 + 不产出任何标签 + 文章零标签行 +
  DB 无 heuristic 行」；3 个 `ExtractTopics` 用例随函数退役删除（不可避免），由
  `TestNoHeuristicFallbackPathRemains` 静态守护替代——实测该守护**非空断言**（/tmp 探针验证会红）。
- **但有 2 个 High 阻塞项**：① 用户报障的主症状（board 1980 挂 `Coding` / `AI Agent`）**并未消除**，
  制品自己的验收条款被真库状态证伪；② 工作树混入另一个 change 的**未提交**代码，
  任务 18 的部署与「门禁已绿」的记录口径均已失真。

| 级别 | 数量 | 编号 |
| --- | --- | --- |
| High | 2 | H1 H2 |
| Medium | 8 | M1 M2 M3 M4 M5 M6 M7 M8 |
| Low | 7 | L1 L2 L3 L4 L5 L6 L7 |

---

## 1. High

### H1 · 真库里 `Coding` / `AI Agent` 仍挂在 board 1980 —— 本 change 的核心验收条款被证伪
- **文件/行号**：
  - `openspec/changes/retire-heuristic-tag-fallback/test-cases.md:39`（§1 节拍 6 期望「board 1980 **不再挂** `Coding`/`AI Agent`」）
  - `openspec/changes/retire-heuristic-tag-fallback/test-cases.md:73`（§3「版块泛词消失」核对项）
  - `openspec/changes/retire-heuristic-tag-fallback/tasks.md:33`（任务 17「board 1980 挂载标签**不再含泛词**」）
  - `openspec/changes/retire-heuristic-tag-fallback/proposal.md:19`（「存量必须一起清：只改代码不动存量，`Coding` / `AI Agent` 仍挂在版块上…用户报障不会消失」）
  - 真库：`topic_tag_board_labels` / `topic_tags`
- **问题**：清理口径只删了 `article_topic_tags` 的 heuristic 行（1727）与**零引用**的 heuristic 标签（14 个），
  `Coding`(id=27) / `AI Agent`(id=185089) 因为「已被 LLM 认领」而被保留并 `source→llm`，
  **它们的 `topic_tag_board_labels` 挂载行、aux labels、embedding 全部原样存活**。
  board 的标签列表正是 join 这张挂载表渲染的（`backend-go/internal/tagmanagement/handler/board_crud_handler.go:894-895`），
  所以用户报障里「board 1980 大量显示 Coding / AI Agent」在**挂载层**依然成立，
  只有背后的文章数塌了（722→1 / 85→16）。
  更关键：`Coding` 的 aux 标签只有 1 条，`min(2, N)` 放宽准入下 `minHits = min(2,1) = 1`，
  其 `max_sim = 0.8811 ≥ 0.8`、`hitRate = 1/1 = 1 ≥ 0.3` —— **匹配作业随时可以把它重新挂回 board 1980**，
  单靠解挂不解决问题。
- **证据**（只读 SQL，真库 `syntopica`）：
  ```
  select 'b1980_mounts=' || count(*) from topic_tag_board_labels where semantic_board_id=1980;   -- 941
  select 'coding_articles=' || count(*) ... where t.slug='coding';                              -- 1
  select 'aiagent_articles=' || count(*) ... where t.slug in ('ai-agent','ai agent');           -- 16
  -- 挂载行仍在：
  -- mount: 27      slug=coding  label=Coding   src=llm score=0.8811008084955101 reason=max_sim
  -- mount: 185089  slug=ai agent label=AI Agent src=llm score=1               reason=hit_rate
  -- aux / emb / links:
  -- tag=coding   aux_count=1 emb=2 article_links=1
  -- tag=ai agent aux_count=3 emb=2 article_links=16
  ```
  对照制品自己的记录：`tasks.md:32`（任务 15）写「board 1980 挂载 944」——**清理前后不变**，
  即制品其实已经知道挂载没动，但 §1 节拍 6 / §3 / 任务 17 仍按「挂载会消失」写验收。
- **建议处置**：**修**（两件事，二选一或都做）：
  1. **改口径**（低成本、必须做）：把 `test-cases.md:39`、`:73` 与 `tasks.md:33` 的验收改成与真库一致的表述——
     「board 1980 的 `Coding` / `AI Agent` **文章数**塌到 1 / 16 篇；**挂载行保留**」；
     §3「版块泛词消失」一行填实测答案（仍在挂载列表）。
  2. **补动作**（若要让报障症状真的消失）：对 `Coding` / `AI Agent` 两个标签做**退役级清理**——
     清 `topic_tag_semantic_labels`（aux）、`topic_tag_embeddings`、解 `topic_tag_board_labels`，
     并把 `topic_tags` 置 `status='merged'`/`merged_into_id` 或直接删（此时引用已归零，可安全删）；
     否则匹配作业会按 `min(2,N)` 把它重新挂回版块。
  这两条属**新增范围**，建议单开一个 change（可与 `tolerate-schema-envelope-extraction-output` 一起排期），
  不要在本 change 里仓促扩范围。

### H2 · 工作树混入另一个 change 的未提交代码，任务 18 部署与「门禁已绿」记录口径均已失真
- **文件/行号**：
  - `backend-go/internal/tagmanagement/service/core/extractor_enhanced.go`（未提交改动：`unwrapSchemaEnvelope` + `buildExtractionUserPrompt` 末句改写）
  - `backend-go/internal/tagmanagement/service/core/extractor_test.go`（未提交：6 个 schema 外壳用例）
  - `backend-go/internal/tagmanagement/service/core/extractor_enhanced_test.go`（未提交：1 个 prompt 格式守护用例）
  - `openspec/changes/tolerate-schema-envelope-extraction-output/`（未跟踪目录）
  - `openspec/changes/retire-heuristic-tag-fallback/tasks.md:35`（任务 18「重启后端 + 部署前端」）
- **问题**：`git status` 显示本 change 三个提交之外的**未提交改动**恰好落在本 change 的审查面文件上
  （`extractor_enhanced.go` / `extractor_test.go` / `extractor_enhanced_test.go`），
  内容是 `tolerate-schema-envelope-extraction-output`（另案）的解析器容错实现。
  后果有三：
  1. **部署污染**：任务 18 若现在执行，编译进二进制的是另一个 change 的**未提交、未过门禁**的半成品；
  2. **门禁记录失真**：制品记录的「build / vet / lint 0 issues / 测试全绿」是在 `2f6c55d1` 的代码上跑的，
     工作树现在的代码**没有**跑过同样门禁（`buildExtractionUserPrompt` 末句改动会直接影响模型输出形态）；
  3. **审查口径**：`extractor_enhanced.go` 的当前状态已不等于本 change 的审查对象，
     任何在当前工作树上的复跑都测的是「两个 change 叠加」的产物。
- **证据**：
  ```
  $ git status --porcelain
   M backend-go/internal/tagmanagement/service/core/extractor_enhanced.go
   M backend-go/internal/tagmanagement/service/core/extractor_enhanced_test.go
   M backend-go/internal/tagmanagement/service/core/extractor_test.go
  ?? openspec/changes/tolerate-schema-envelope-extraction-output/
  ```
- **建议处置**：**修**（流程纪律，零代码成本）：
  1. 任务 18（部署）**推迟到另案的解析器容错提交落地并单独过门禁之后**，
     或先把另案的改动 `git stash` / 提交到独立分支再部署；
  2. 在 `tasks.md` 的任务 6（后端门禁）旁补一行「门禁结论对应 commit `2f6c55d1`；工作树另有
     `tolerate-schema-envelope-extraction-output` 未提交改动，未纳入本 change 门禁」；
  3. 本 change 归档前确保 `git status` 干净（或明确记录哪些改动不属于本 change）。

---

## 2. Medium

### M1 · `aggregate-tagging` capability 无 delta，归档后主 spec 与实现直接冲突
- **文件/行号**：`openspec/specs/aggregate-tagging/spec.md:134`、`:149`；
  本 change 的 delta 目录 `openspec/changes/retire-heuristic-tag-fallback/specs/` 只有 `tagging-domain/`、`auxiliary-label/`
- **问题**：主 spec 仍写「回落 mono 提取路径（单次调用 LLM 提取，**含 heuristic 兜底**），
  SHALL NOT 让聚合型文章以 0 标签结束打标」。代码里兜底已删（`article_tagger.go:197` 返回错误），
  聚合文章在 mono 也失败时**会以 0 标签 + 任务失败**结束——正是该 SHALL 禁止的形态。
  本 change 没有 `specs/aggregate-tagging/spec.md` delta，归档同步时该 capability 不会被改动。
- **证据**：
  ```
  $ grep -rn "heuristic" openspec/specs/aggregate-tagging/spec.md
  openspec/specs/aggregate-tagging/spec.md:134:…SHALL 回落 mono 提取路径（单次调用 LLM 提取，含 heuristic 兜底）…
  openspec/specs/aggregate-tagging/spec.md:149:…该文章走 mono 提取路径打标（单次调用 LLM 提取，含 heuristic 兜底）…
  ```
- **建议处置**：**修**。补 `openspec/changes/retire-heuristic-tag-fallback/specs/aggregate-tagging/spec.md`，
  MODIFIED 该 requirement：去掉「含 heuristic 兜底」，并把「最终标签数不为 0」改为
  「mono 成功则不为 0；mono 亦失败 → 任务失败重试，文章保持无标签」。

### M2 · `tagging-domain` 主 spec 的 exemplar-guard requirement 仍把「heuristic 兜底」列为一条路径，未被 MODIFIED
- **文件/行号**：`openspec/specs/tagging-domain/spec.md:310`
- **问题**：末句「heuristic 兜底与跨 feed 标签复用路径不经 LLM 解析出口，不受本护栏影响」——
  heuristic 路径已整文件退役，该表述指向一条不存在的路径。本 change 的 delta MODIFIED 了
  「泛词标签黑名单拦截」「LLM 空 keyword 结果不触发规则回填」「Tagging domain package structure」，
  独独漏了这条。
- **证据**：`grep -rn "heuristic" openspec/specs/tagging-domain/spec.md` → `:237/:239/:244/:246/:249`（已由 delta 接管）
  与 `:253`（已接管）、`:310`（**未接管**）
- **建议处置**：**修**（一行）。delta 里补 MODIFIED 该 requirement 的末句为
  「跨 feed 标签复用路径不经 LLM 解析出口，不受本护栏影响」。

### M3 · 「提取失败 → 退避重试 → failed 态」这条新承诺全仓零守护用例
- **文件/行号**：
  - `openspec/changes/retire-heuristic-tag-fallback/test-cases.md:38`（§1 节拍 3，落点写明「**需守护用例**」）
  - `openspec/changes/retire-heuristic-tag-fallback/test-cases.md:83-84`（§4 白盒表列 `MarkFailed` 两行分支）
  - `openspec/changes/retire-heuristic-tag-fallback/tasks.md:41`（任务 24 未勾选）
  - `backend-go/internal/tagmanagement/repository/tag_job_queue.go:160`（`MarkFailed`，**无测试**）
  - `backend-go/internal/tagmanagement/repository/tag_job_queue_test.go`（只有 Enqueue / Claim 三例，无 `MarkFailed`）
- **问题**：本 change 的**核心新承诺**是「失败被可见、可重试」——但这条链路的**唯一**守护不存在。
  `MarkFailed` 的两条分支（`attempt < max` → `pending` + `available_at=now+backoff`；
  `attempt >= max` → `failed` 终态）在 tag job 队列上**至今没有任何测试**；
  仓里唯一的 `MarkFailedBackoffSchedule` 是 **embedding 队列**的（`service/core/embedding_queue_retry_test.go:35`），
  不是 tag_jobs。`TestClaimSkipsBackoffJob` 只覆盖「未来 `available_at` 不被 lease」，不覆盖 `MarkFailed` 本身。
  若后续有人把 `MarkFailed` 改成恒 `pending`（忘了 `attempt >= max` 分支），测试**全绿**。
- **证据**：
  ```
  $ grep -rn "MarkFailed" --include=*_test.go backend-go/
  backend-go/internal/tagmanagement/service/core/embedding_queue_retry_test.go:35:func TestMarkFailedBackoffSchedule(t *testing.T)   # embedding 队列，非 tag_jobs
  $ grep -n "^func Test" backend-go/internal/tagmanagement/repository/tag_job_queue_test.go
  TestEnqueueTagJobUpgradesForceRetag / TestClaimOrderNewestFirst / TestClaimPriorityPreemptsFreshness / TestClaimSkipsBackoffJob
  ```
- **建议处置**：**修**。在 `repository/tag_job_queue_test.go` 补两例：
  ① `MarkFailed` 在 `attempt_count < max_attempts` → `status=pending` 且 `available_at > now()`；
  ② `attempt_count >= max_attempts` → `status=failed`；另在 `service/core/tag_queue_test.go`
  补一例「`TagArticle` 失败 → `processJob` 调 `MarkFailed` 且 `last_error` 可读」的编排级用例。
  成本低（不碰生产代码），收益是把本 change 的核心承诺钉死。

### M4 · 语义板红线 19 在被改的那一行里仍写「heuristic 三路径」
- **文件/行号**：`docs/reference/flow/semantic-board.md:419`
- **问题**：该行前半句已改成「提取调用失败同样不降级规则词——直接判失败」，但同一行后半句仍写
  「`filterGenericLabels` 为 **mono/aggregate/heuristic 三路径**汇聚拦截点」。
  heuristic 写入路径已不存在，实际只有两条（mono / aggregate）+ reuse（不走 persist 入口）。
  这是**同一次编辑留下的自相矛盾**，且红线句是会被 constraint-injection 注入给模型的强约束文本。
- **证据**：`grep -n "三路径" docs/reference/flow/semantic-board.md` → `:419`（唯一命中）
  代码侧：`persistArticleTags` 的调用方只有 `article_tagger.go:207`，`source` 只有 `llm`。
- **建议处置**：**修**（一行）。改为「`filterGenericLabels` 为 mono/aggregate **两路径**汇聚拦截点」。

### M5 · design D5「规范与文档同步点」表漏 3 处实际改动
- **文件/行号**：`openspec/changes/retire-heuristic-tag-fallback/design.md`（D5 表，5 行）
  vs 实际 `9e9bb417`（5 个文件 7 处）
- **问题**：D5 列了 5 处，**漏了**：
  - `docs/reference/flow/reading.md`（提取容错节，「回落链路不再含 heuristic 兜底」）
  - `docs/reference/database/DATA_LIFECYCLE.md`（术语口径）
  - `openspec/specs/tagging-domain/spec.md` 的 **MODIFIED「Tagging domain package structure」**
    （提交信息明写「一并拉回实际结构」，delta 里也确实有，但 D5 没登记）
  登记不全会让后续维护者误判「没改过的文件不需要复核」。
- **证据**：`git show 9e9bb417 --stat` → 5 files changed（D5 只覆盖其中 3 个文件的 2 处）
- **建议处置**：**修**。D5 表补 3 行；顺带把「Tagging domain package structure」纳入 D5 并注明
  「该 requirement 此前停留在 `internal/domain/tagging/` 旧布局且引用已删除的 `extraction.ExtractTopics`」。

### M6 · 存量数字四处口径不一致，且「删 28 个标签」与实际「删 14 + 改 14」不是一回事
- **文件/行号**：
  - `openspec/changes/retire-heuristic-tag-fallback/proposal.md:30`、`:44`、`:53`（1724 行 / 28 个标签 / -1724 / -28）
  - `scripts/db/cleanup-heuristic-tags-20261112.sql:7`、`:10`、`:27`、`:28`、`:71`（1724 / 18530 / 1141 篇）
  - `openspec/changes/retire-heuristic-tag-fallback/tasks.md:28`、`:30`（18533 / 1727 / 1142 篇）
  - `openspec/changes/retire-heuristic-tag-fallback/design.md`（D3 清理范围，14 删 + 14 改 llm）
- **问题**：三套数字并存：
  | 口径 | 文章标签行 | topic_tags | 受影响文章 |
  | --- | --- | --- | --- |
  | proposal（`:30`/`:44`） | 1724 | 删 28 个 | — |
  | SQL 脚本头注释（`:7`/`:27`/`:28`） | 1724（总 18530） | 28 个（总 13888） | 1141 |
  | tasks 12/14/15（真执行） | **1727**（总 18533） | 28 个 = **删 14 + 改 14 llm**（总 13882） | **1142** |
  更要紧的是**语义差**：proposal 写「删 `topic_tags` 中 `source='heuristic'` 的 **28 个**标签」，
  实际只删了 **14 个**零引用标签，另 **14 个被保留并改成 llm**（它们是 LLM 真实产出，删了会丢真标签）。
  按 proposal 字面执行会误删 `OpenAI` / `NVIDIA` / `GPT-6` 等正常标签。
- **证据**：真库 `select 'tt_total=' || count(*) from topic_tags` → 13878（清理后随打标继续增长）；
  `select 'att_heuristic=' || count(*) ... where source='heuristic'` → 0；
  `select 'att_source_values:' || string_agg(source||'='||c,' ') ...` → `reuse=11 llm=16802`
- **建议处置**：**修**（文档口径，不动已执行的数据）。以 tasks 12/14/15 的真值为准，把 proposal 的
  「1724/28 个（删）」改成「1727 行；28 个 heuristic 标签中删 14 个零引用者、另 14 个 LLM 已认领者改 source=llm」；
  SQL 脚本头注释同步改成真值（脚本本体是数据驱动的，不受影响，但注释会误导复跑者）。

### M7 · tasks 1–11 / 18–24 未勾选，test-cases §3 效果核对表仍停在「待 tasks 15 填」
- **文件/行号**：`openspec/changes/retire-heuristic-tag-fallback/tasks.md:6-16`（1–11 全 `[ ]`）、
  `:35-41`（18–24 全 `[ ]`）；`openspec/changes/retire-heuristic-tag-fallback/test-cases.md:68-75`（§3 四行「待 tasks 15/16/17 填」）
- **问题**：`2f6c55d1` / `9e9bb417` 明确已完成代码删除与文档同步，但任务 1–11 一条未勾；
  任务 15 已 `[x]`（含真库实测数字），而 test-cases §3「效果核对」表的四行仍写「待 tasks 15 填」。
  归档门禁（`openspec/changes/archive/…` 惯例）要求 tasks 全勾 + 制品自洽，当前两处都不过。
- **证据**：`git show 2f6c55d1 --stat`（6 个源文件 + 6 个测试文件已改）/ `git show 9e9bb417 --stat`（5 个文档已改）
  vs `tasks.md` 1–11 全未勾
- **建议处置**：**修**。按真值勾 1–11（并把 18/19/20 的完成情况如实回填），
  §3 的「污染面清零」一行填实测（`article_topic_tags` heuristic = 0、`topic_tags` heuristic = 0），
  「版块泛词消失」一行按 H1 的实测答案填（**仍在挂载**）。

### M8 · Force retag「先删旧标签、后提取」且无回滚，与任务 16 的重打计划叠加会吃掉存量好标签
- **文件/行号**：`backend-go/internal/tagmanagement/service/core/article_tagger.go:110-127`（Force 分支先 `Delete` 旧 link + `CleanupOrphanedTags`）；
  `backend-go/internal/tagmanagement/handler/tag_queue_handler.go:246-251`（`retag-today` 对**今日全部文章**投 `ForceRetag=true`）；
  `openspec/changes/retire-heuristic-tag-fallback/tasks.md:32`（任务 16）
- **问题**：`RetagArticle` 的删除发生在提取**之前**且没有事务包裹。旧行为下提取失败还有 heuristic 兜底可写，
  新行为下提取失败 = 旧标签已删、新标签没有、文章归零 + 任务失败重试。
  `retag-today` 是「今日全部文章」强制重打——**若在解析器修好之前跑，今日已有正常 LLM 标签的文章会被清空并停在 failed**。
  `CleanupOrphanedTags` 还会连带删掉那些标签的 `topic_tags` 行（其 aux / embedding / 板挂载随之 CASCADE），
  后续即便重打成功，标签也是新 id、板挂载需重新匹配才回来。
- **证据**：真库 `tag_jobs` 现存 `failed=14`（`last_error` 含 schema 外壳报错）、`pending=35`；
  任务 16 的阻塞说明与该现象一致。
- **建议处置**：**不修代码，但修执行纪律**——在 tasks 任务 16 补一条硬前置：
  「重打前必须先修 `tolerate-schema-envelope-extraction-output` 并用单篇验证成功率；
  在此之前**禁止**对存量带标签文章执行 `ForceRetag`（`retag-today` 会清空今日好标签）；
  若必须先跑，只对**当前无标签**的文章做（`Force=false` 路径不会删旧标签）」。
  （`Force=false` 路径在 `article_tagger.go:145` 的「已打标则跳过」保护下天然安全。）

---

## 3. Low

### L1 · 「退避 1 分钟 → 30 分钟」的描述与实际曲线不符（30 分钟上限在 max_attempts=5 下不可达）
- **文件/行号**：`backend-go/internal/tagmanagement/service/core/tag_queue.go:326-334`；
  `backend-go/internal/tagmanagement/repository/tag_job_queue.go:11`（`defaultTagJobMaxAttempts = 5`）；
  引用处：`proposal.md:26`、`design.md` D2、`docs/reference/flow/semantic-board.md:419`、`test-cases.md` §2.3
- **问题**：`failureBackoff` = attempt1→1min、2→2min、3→4min、4→8min、5→16min，30 分钟封顶需 attempt≥6，
  而 `max_attempts=5` 意味着第 5 次失败即进 `failed` 终态——**30 分钟这一档永远不会被触发**。
  制品与红线句统一写成「1→30 分钟」，会误导运维以为退避会拉长到半小时。
- **建议处置**：**已采纳**（口径按实际改写）。四处统一改成「1/2/4/8/16 分钟（上限 30 分钟在 max_attempts=5 下不可达）」，
  或把红线句简化为「退避重试，上限 max_attempts=5」。

### L2 · design D1「`health_gate_compose_test.go` 命中的是无关英文词 heuristics」的判断有误，该文件注释现已失真
- **文件/行号**：`backend-go/internal/platform/analysispause/health_gate_compose_test.go:310`
  （「…the tag worker kept leasing — producing **heuristic-degraded tags**.」）；
  `openspec/changes/retire-heuristic-tag-fallback/design.md`（D1「**不改**」行）
- **问题**：该文件命中的是 `heuristic-degraded tags`，**正是本链路**（打标不可用时文章带降级标签入库），
  不是无关英文词。改完之后该注释描述的行为已不存在（现在是「只能以无标签入库」）。
- **建议处置**：**修**（一行注释）。改为「…kept leasing — articles would land untagged (retire-heuristic-tag-fallback)」。
  另在 D1 把「命中的是无关英文词」的判断说明改正（判断本身可保留「不改该文件」，但理由要写对）。

### L3 · `auxiliary-label` delta 的场景标题「单次调用整体失败回退 heuristic」未改名，与 THEN 自相矛盾
- **文件/行号**：`openspec/changes/retire-heuristic-tag-fallback/specs/auxiliary-label/spec.md:36`（场景标题）
  vs `:39`（THEN 已改成「SHALL 返回失败并把错误上抛…不回退 heuristic keyword 提取」）
- **问题**：标题仍说「回退 heuristic」，正文说「不回退」。OpenSpec 的场景名是主 spec 归档后的契约标识，
  标题与正文反向会造成检索/对照困惑。
- **建议处置**：**修**。标题改为「单次调用整体失败即判失败，不回退 heuristic」。

### L4 · 守护用例的扫描范围比 test-cases 的验收口径窄
- **文件/行号**：`backend-go/internal/tagmanagement/service/core/extractor_test.go:24-44`（`TestNoHeuristicFallbackPathRemains`）；
  `openspec/changes/retire-heuristic-tag-fallback/test-cases.md:39`（「**代码库**无 `source='heuristic'` 写入点」）
- **问题**：① 守护只 walk `filepath.Join("..","..")` = `internal/tagmanagement`，不覆盖 `internal/models` /
  `internal/platform` / 其他域；② 匹配的是带引号的字面量 `"heuristic"`，
  写成 `const src = "heur" + "uristic"` 或从外部常量引用则**逃逸**。
  实践中 `article_topic_tags.source` / `topic_tags.source` 的全部写点都在 tagmanagement 域内（已核），
  所以当前不漏；但「代码库级」这句口径比实现宽。
- **证据**（守护非空断言，/tmp 探针）：
  ```
  WOULD FAIL: /tmp/hprobe/internal/tagmanagement/service/core/zz.go   # 文件含 `"heuristic"` 即触发
  $ go test ./internal/tagmanagement/service/core/ -run TestNoHeuristicFallbackPathRemains -v
  --- PASS: TestNoHeuristicFallbackPathRemains (0.00s)
  ```
- **建议处置**：**修**（低成本）。把 test-cases §1 节拍 5 的口径改成「tagmanagement 域生产代码不得出现
  `source="heuristic"` 写入点」，与实现一致；或把守护的 walk root 提到 `internal/` 并改成
  「`heuristic` 出现在 `source` 赋值上下文」的正则。

### L5 · proposal 的 Capabilities / 「规范与文档」两段清单不全
- **文件/行号**：`openspec/changes/retire-heuristic-tag-fallback/proposal.md:36-38`（规范与文档，5 项）、
  `:42-46`（Capabilities，2 个 capability）
- **问题**：实际还改了 `docs/reference/flow/reading.md`、`docs/reference/database/DATA_LIFECYCLE.md`，
  以及 `tagging-domain` 的 MODIFIED「Tagging domain package structure」——
  三者都没进 proposal 的清单（`tagging-domain` 虽在 Capabilities 里，但没写包结构这条 requirement）。
- **建议处置**：**修**。补齐清单（与 M5 的 D5 表一起改）。

### L6 · `TagArticles` 批量 API 逐篇吞错并返回 nil（当前无生产调用方）
- **文件/行号**：`backend-go/internal/tagmanagement/service/core/article_tagger.go:442-460`
- **问题**：`for` 循环里 `logging.Warnf` 后继续，函数末尾 `return nil`——若将来被接到队列/handler 上，
  会把「提取失败」再次伪装成成功，正是本 change 要消灭的病根。
- **证据**：`grep -rn "TagArticles(" --include=*.go internal/` → 只有定义处，**无生产调用方**
  （队列走的是单篇 `TagArticle`/`RetagArticle`，`tag_queue.go:300/302`）。
- **建议处置**：**已驳回**（当前无生产调用方，不构成活缺陷；不改以免扩大本 change 范围）。
  建议在 `tolerate-schema-envelope-extraction-output` 或下一个 tagging 改动里顺手加一行注释
  「批量入口吞错，禁止接到队列路径」。

### L7 · test-cases §0 把 `article_tagger_test.go:158` 的 `link.Source == "heuristic"` 误记为「跨 feed reuse 路径」用例
- **文件/行号**：`openspec/changes/retire-heuristic-tag-fallback/test-cases.md:21`（§0 表格最后一行）
- **问题**：旧 `article_tagger_test.go` 的 `link.Source == "heuristic"` 断言在
  `TestTagArticleCallFailureFallsBackToHeuristic` 内（**失败兜底**用例），不是跨 feed reuse 用例；
  跨 feed reuse 实际由 `article_tagger_reuse_test.go` 覆盖（8 个用例，断言 `tagSourceReuse`），本次**未改**。
  §0 表格因此把同一条断言记了两遍、且把归属记错。
- **证据**：`git show 2f6c55d1^:backend-go/internal/tagmanagement/service/core/article_tagger_test.go | sed -n '145,165p'`
  → 该断言在失败兜底用例内；`grep -n "^func Test" article_tagger_reuse_test.go` → 8 个 reuse 用例，无改动。
- **建议处置**：**修**。§0 表格该行改为「跨 feed reuse 路径 | **保留**（`article_tagger_reuse_test.go`，本次未改）|
  旧测试：无（复用路径原就不依赖 heuristic）| 无需改动」。

---

## 4. 核实记录

### 4.1 只读验证命令

| 命令 | 结果 |
| --- | --- |
| `openspec validate retire-heuristic-tag-fallback --strict` | **通过**（`Change 'retire-heuristic-tag-fallback' is valid`） |
| `go test ./internal/tagmanagement/service/core/ -run TestNoHeuristicFallbackPathRemains -v` | PASS（0.00s）；并用 /tmp 探针证明该守护**非空断言**（含 `"heuristic"` 的生产文件会触发失败） |
| `git show 2f6c55d1 --stat` | 12 个源/测试文件 + design.md + delta spec；**未触碰** `semantic_board_matching.go` / 日报消费侧 / 前端 |
| `git show 9e9bb417 --stat` | 5 个文档文件 7 处 |
| `git show af286985 --stat` | design.md D3.1/D3.2 + tasks.md 12–17 + 清理 SQL 新增 |
| `git status --porcelain` | **3 个未提交修改 + 1 个未跟踪目录**（见 H2） |

### 4.2 真库只读核对（`docker exec syntopica-postgres psql`，只读 SELECT）

| 核对项 | 实测 | 与制品记录比对 |
| --- | --- | --- |
| `article_topic_tags` where `source='heuristic'` | **0** | tasks 15 ✓ |
| `topic_tags` where `source='heuristic'` | **0** | tasks 15 ✓ |
| `article_topic_tags.source` 取值分布 | `llm=16802`、`reuse=11` | 无 heuristic ✓（reuse 路径未被误改） |
| `topic_tags` 总数 | 13878 | tasks 15 记 13882（此后随打标继续增长，趋势合理） |
| board 1980 挂载行 | **941** | tasks 12/15 记 944（清理前后**不变** → 见 H1） |
| `Coding` 关联文章数 | **1** | tasks 15 记 722→1 ✓ |
| `AI Agent` 关联文章数 | **16** | tasks 15 记 85→16 ✓ |
| `Coding` / `AI Agent` 的 board 1980 挂载行 | **仍在**（score 0.8811 `max_sim` / 1 `hit_rate`） | **与制品验收相反** → H1 |
| `Coding` / `AI Agent` 的 aux / emb / links | 1/2/1、3/2/16 | aux 与 embedding 存活 → 匹配作业可重新挂回（H1） |
| `articles.tag_count` 全库不一致 | **1160** | tasks 15 记 1157（存量漂移，趋势合理） |
| 存量孤儿（未被越界清理） | `auxiliary_description_repair_items` orphan **53496**、`tag_merge_suggestions` orphan **10781** | 制品记 53488 / 10802 ✓（清理只清了本次 12 + 21） |
| `tag_jobs` 状态分布 | completed=1169 / failed=14 / pending=35；failed 中 14 条 `last_error` 含 schema 报错 | 与任务 16「试验批已置 failed 暂停」一致 ✓ |

### 4.3 逐条核对结论（对照 brief 审查重点）

| 重点 | 结论 | 依据 |
| --- | --- | --- |
| ① 挂载行为不变（`min(2,N)` / `max_sim` / 文章可见性 / aggregate→mono） | **成立** | 三个提交的文件清单不含匹配与日报消费侧；`article_tagger_aggregate.go` 未动；只有 mono 错误分支变了行为 |
| ② 失败语义闭环 | **成立**，但**无守护用例** | `extractor_enhanced.go:53` → `article_tagger.go:197` → `tag_queue.go:306` → `tag_job_queue.go:160`；唯一例外是 `TagArticles` 批量 API 吞错（无调用方，见 L6） |
| ③ `ExtractionResult.Source` 删除影响面 | **无残留消费方**；`topic_tags.source` 写点收敛为 `llm`/`manual`/`llm_suggest`；reuse 路径 `source=reuse` 未被误改 | `grep ExtractionResult` 仅命中定义处；真库 source 分布见上 |
| ④ 测试是否守住承诺 | **没有被改弱**（4 处改写全部变强）；3 个 `ExtractTopics` 用例随函数退役删除**可接受**（由静态守护替代）；新守护**真会红** | diff 逐条比对 + /tmp 探针 + 实跑 PASS |
| ⑤ 清理脚本正确性 | **幂等成立**、删除顺序安全（FK CASCADE）、作用域不越界（存量孤儿保留）、`tag_count` 两段式重算正确（先置 0 再按剩余关联回填） | 脚本逐条走查 + 真库实测 |
| ⑥ 规范与文档一致性 | **主 spec 3 处未随 delta 接管**（M1/M2）；红线句 1 处自相矛盾（M4）；design/proposal 清单与数字不全/不一致（M5/M6/L5）；变更溯源历史行**保持原样** ✓ | `git show 9e9bb417` 逐行 |
| ⑦ 遗漏扫描（全仓 `heuristic`） | 生产代码**无**残留写入点；`extractor_test.go:39` 守护覆盖 tagmanagement 域；残留均为注释/测试/另案产物 | `grep -rni heuristic backend-go/ front/ docs/ scripts/ openspec/specs/`（排除 logs / backups / node_modules） |

---

## 5. review-trace

### R-1 2026-11-12【High】board 1980 仍挂 Coding / AI Agent，核心验收条款被真库证伪
- **状态**：待处理

### R-2 2026-11-12【High】工作树混入未提交的 tolerate-schema-envelope 代码，部署与门禁口径失真
- **状态**：待处理

### R-3 2026-11-12【Medium】aggregate-tagging capability 缺 delta，归档后主 spec 与实现冲突
- **状态**：待处理

### R-4 2026-11-12【Medium】tagging-domain exemplar-guard requirement 未随 delta 接管
- **状态**：待处理

### R-5 2026-11-12【Medium】「失败→退避重试→failed 态」链路零守护用例
- **状态**：待处理

### R-6 2026-11-12【Medium】语义板红线 19 同一行仍写「heuristic 三路径」
- **状态**：待处理

### R-7 2026-11-12【Medium】design D5 同步点表漏 3 处实际改动
- **状态**：待处理

### R-8 2026-11-12【Medium】存量数字口径四处不一致（1724/1727、删 28/删 14+改 14）
- **状态**：待处理

### R-9 2026-11-12【Medium】tasks 1–11 / 18–24 未勾选，test-cases §3 效果核对表未回填
- **状态**：待处理

### R-10 2026-11-12【Medium】Force retag 先删后提无回滚，与任务 16 重打计划叠加会清空存量好标签
- **状态**：待处理

### R-11 2026-11-12【Low】「退避 1→30 分钟」口径与实际曲线（1/2/4/8/16）不符
- **状态**：已采纳

### R-12 2026-11-12【Low】health_gate_compose_test.go:310 注释失真，design D1 判断有误
- **状态**：待处理

### R-13 2026-11-12【Low】auxiliary-label delta 场景标题「单次调用整体失败回退 heuristic」未改名
- **状态**：待处理

### R-14 2026-11-12【Low】守护用例扫描范围窄于 test-cases「代码库级」口径
- **状态**：待处理

### R-15 2026-11-12【Low】proposal 的 Capabilities / 规范与文档清单不全
- **状态**：待处理

### R-16 2026-11-12【Low】TagArticles 批量 API 吞错（无生产调用方）
- **状态**：已驳回

### R-17 2026-11-12【Low】test-cases §0 把失败兜底断言误记为跨 feed reuse 用例
- **状态**：待处理
