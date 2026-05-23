## Context

当前后端 `internal/domain/topicanalysis/` 包含 42 个源文件、~13000 行代码，同时承担主题分析 CRUD、embedding 向量化、标签合并/聚类、抽象标签层级体系、关注标签管理、8 个队列 worker 等 6 个独立职责。`topicextraction`（10 文件/2500 行）与 `topicanalysis` 之间存在双向依赖。`models/` 是 34 个 struct 的共享桶，所有域都 import。`runtime.go` 手动启动 5 个 queue worker 和 8 个 scheduler，通过 `runtimeinfo/` 全局变量传递实例。

约束：单用户本地项目，测试用真实 DB（不需要 mock/DI），可以一次性大改。

## Goals / Non-Goals

**Goals:**

- 将 `topicanalysis` 按 6 个独立职责拆入 `tagging/` 域的 6 个子包
- 将 `topicextraction`、`topictypes` 并入 `tagging/`，消除循环依赖
- 将 `models/` 中的 struct 按域分散到各自的 model 文件
- 统一包名为单数形式（`feed/`、`article/` 等）
- 5 个 queue worker 生命周期收敛到 `tagging/workers.go`
- 全部依赖方向单向化

**Non-Goals:**

- 不改变任何函数签名、业务逻辑、API 路由或数据库 schema
- 不引入 DI 框架或接口抽象（测试用真实 DB）
- 不改动 `platform/`、`jobs/` 的内部逻辑
- 不改动前端代码

## Decisions

### 决策 1: 共享函数按职责分发，不建 shared.go 垃圾桶

被多个子包调用的 9 个函数按职责分放：
- `MergeTags`、`DeleteTagEmbedding`、`MergeCandidateLists` → `tagging/helpers.go`（无 AI 调用的 DB 操作）
- `NewEmbeddingService` → `tagging/embedding/`
- `ExtractAbstractTag`、`BatchCallLLMForTagJudgment`、`PlaceTagInHierarchy`、`ThresholdsForCategory` → `tagging/hierarchy/`
- `ExpandEventCandidatesByArticleCoTags` → `tagging/`（cotag_expansion.go）

替代方案 A（全部放 `tagging/shared.go`）因会成为新垃圾桶而被否决。
替代方案 C（建 `tagging/core/` 子包）因多一层间接且名字模糊而被否决。

### 决策 2: TopicTag 归属 `tagging/types.go`

TopicTag 等标签核心实体由 `tagging/types.go` 持有。`articles` 包对 TopicTagRelation 的唯一只读查询改为调用 `tagging` 暴露的函数。`narrative`、`topicgraph` 直接 import `tagging/types`。

替代方案 B（独立 `tagmodels/` 包）因多一个包且零额外收益而被否决。
替代方案 C（留在 `models/`）因不解决波及面问题而被否决。

### 决策 3: tagger.go 提升为编排器

`tagger.go`（`findOrCreateTag` 及相关函数）从 `extraction` 提升到 `tagging/` 根层。`extraction` 子包只负责纯文本提取（输入文章文本，输出 `[]TopicTag`）。`tagger.go` 作为标签生命周期编排器，调用 embedding 匹配、LLM 判断、合并、层级放置。

替代方案 A（留在 extraction）因 extraction 名不副实（它不只是"提取"）而被否决。

### 决策 4: narrative 的 tag_feedback 和 watched_narrative 留原包

`tag_feedback.go` 和 `watched_narrative.go` 物理上留在 `narrative/` 包内，只改 import 路径。`feedbackNarrativesToTags` 是叙事生成流程的最后一步，触发源是"叙事生成了"而非"标签变化了"。`GenerateWatchedTagNarratives` 已废弃无外部调用。

### 决策 5: 5 个 queue worker 收敛到 `tagging/workers.go`

`tagging/workers.go` 暴露 `StartAllWorkers()` / `StopAllWorkers()`，内部按正确顺序启动 5 个 worker（TagQueue、EmbeddingQueue、MergeReembeddingQueue、AbstractTagUpdateQueue、AdoptNarrowerQueue）。`runtime.go` 从 10 行 worker 管理收敛到 2 行。

替代方案 C（WorkerGroup 结构体）因不需要"只启一部分"的场景而被否决。

### 决策 6: 一次性脚本改完

纯机械操作（文件搬移 + package 声明 + import 路径 sed），不改变任何逻辑。执行流：git mv → 批量改 package 声明 → 批量 sed import 路径 → `go build` 看报错逐个修 → `go test` → commit。

替代方案 B（分 3 步 + type alias 过渡）因本地项目不需要保持中间态而被否决。

### 决策 7: 包名单数化

`feeds` → `feed`、`articles` → `article`、`categories` → `category`、`contentprocessing` → `content`。符合 Go 惯例（`net/http` 不是 `net/https`）。

### 决策 8: models 按子包分散

`tagging/types.go` 持有核心标签实体（TopicTag、TopicTagRelation 等 8 个）。`tagging/embedding/models.go` 持有 EmbeddingConfig、EmbeddingQueue、MergeReembeddingQueue。`tagging/hierarchy/models.go` 持有 HierarchyConfig 及其版本、各种 Queue。`models/` 仅保留 SchedulerTask。每个业务域（feed、article、content、preferences、aiadmin、narrative）自持各自的 model 文件。

### 决策 9: SchedulerTask 留在 models/

`SchedulerTask` 被 `platform/database/migrator.go`（AutoMigrate）和 `jobs/` 使用。放到 `jobs/` 会造成 `platform → jobs` 的反向依赖。留在 `models/` 作为基础设施级 model。

## Risks / Trade-offs

- **风险**: 42 个文件拆包后，同包内的直接函数调用变成跨包 import，Go 编译器会报大量 undefined 错误 → 缓解：`go build ./...` 的报错信息精确定位每个缺失 import，逐个修复
- **风险**: topicanalysis 内部的 unexported 函数/变量拆包后需要决定是 export 还是留在原处 → 缓解：按需 export，用编译器报错驱动
- **风险**: 测试文件（16 个 _test.go）的 import 路径也要同步改 → 缓解：和源文件同一批 sed
- **权衡**: 一次性改完没有中间检查点 → 可接受：纯机械操作，不改逻辑，`go build` 就是检查点
- **权衡**: tagging/ 根层仍有 ~15 个文件 → 比 topicanalysis 的 42 文件好得多，且每个文件职责单一
