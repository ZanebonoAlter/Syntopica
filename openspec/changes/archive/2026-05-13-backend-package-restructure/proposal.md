## Why

后端 `topicanalysis` 包膨胀到 42 文件 / ~13000 行，混杂了 6 个独立职责（分析、embedding、合并、层级、抽象标签、关注标签）。改一处波及面大，容易改漏或误伤旧功能。`models/` 共享桶（34 个 struct）导致任何 model 变动触发全域重编译。`topicextraction` 与 `topicanalysis` 之间存在循环依赖。测试和业务代码混在同包，难以区分和独立测试。

## What Changes

- **BREAKING**: 消灭 `topicanalysis`、`topicextraction`、`topictypes` 三个包，拆入新建的 `tagging/` 域（含 6 个子包）
- **BREAKING**: 重命名 `feeds/` → `feed/`、`articles/` → `article/`、`categories/` → `category/`、`contentprocessing/` → `content/`
- **BREAKING**: `models/` 瘦身，34 个 struct 按域分散到各自的 model 文件，仅保留 `SchedulerTask`
- 新建 `tagging/workers.go` 统一管理 5 个 queue worker 的生命周期（StartAll/StopAll）
- `tagger.go` 从 `extraction` 提升到 `tagging/` 根层，作为标签生命周期编排器
- `extraction` 子包只负责纯文本提取（输出 `[]TopicTag`）
- 依赖方向全部单向化：`extraction → helpers/embedding/hierarchy`，`hierarchy → helpers/embedding`

## Capabilities

### New Capabilities

- `tagging-domain`: 统合标签提取、分析、embedding、层级、合并、关注标签为独立子包的标签域

### Modified Capabilities

_(无既有 spec 的行为变更，纯内部重组)_

## Impact

- 所有 `internal/domain/` 下的包名和 import 路径变更
- `internal/app/router.go`、`runtime.go` 的 import 路径全部更新
- `internal/jobs/` 下 8 个 scheduler 的 import 路径更新
- `internal/platform/database/migrator.go` 引用 model 的 import 路径更新
- `internal/domain/narrative/` 中 `tag_feedback.go`、`watched_narrative.go` 的 import 路径更新
- 无 API 路由变更，无数据库 schema 变更，无前端变更
