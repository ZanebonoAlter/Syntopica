# Test Cases — heal-dangling-article-refs

> 单元 = 一个 Requirement 的用户故事，本文件把 spec Scenario 串成完整故事。
> 主线故事：**用户在 2026-09-16 的日报里点开线索 10264「做了个给终端 agent 配上立绘和语音的插件」，展开的 2 篇来源文章里不再出现「文章 #129447」，剩下的 129446 能正常打开**。

## 主链路（节拍表）

| 步 | 动作 | 来源 Scenario | 期望 | 层 | 落点 |
| --- | --- | --- | --- | --- | --- |
| 1 | 去重归并把 loser 副本 `X` 合并进保留条 `Y`（同事务） | 引用数组：归并副本时引用改指保留条 | `related_article_ids` 中 `X`→`Y`，无重复 `Y`，其余保序 | PG 迁移测试 | `backend-go/internal/platform/database/dedupe_rss_articles_migration_test.go` + `backend-go/internal/platform/articlerefs/rewire_test.go` |
| 2 | 归并前数组内已同时含 `X`/`Y` | 引用数组：保留条已在数组内时只去重 | 只留第一个 `Y`，长度减一 | PG 迁移测试 | 同上 |
| 3 | 删除订阅源（`fk_feeds_articles` 级联删其文章） | 引用数组：删除订阅源时引用被剔除 | 引用被剔除、保序、空则 `[]` | PG 仓储测试 | `backend-go/internal/reader/repository/article_refs_test.go` |
| 3b | 删除分类（`fk_categories_feeds` 两级级联：分类→feed→文章） | 引用数组：删除时引用被剔除 | 该分类下全部 feed 的文章引用被剔除；相邻分类/feed 不受影响；零命中零写入 | PG 仓储测试 | `backend-go/internal/reader/repository/article_refs_test.go` |
| 4 | 被删文章无任何 thread 引用 | 引用数组：无引用命中时不产生写操作 | `RowsAffected=0`，无 UPDATE | 单元测试（helper） | `backend-go/internal/platform/articlerefs/rewire_test.go` |
| 5 | 迁移在含 4774 行历史悬空的库上执行 | 存量修复：#1/#2 | 悬空 id 全部剔除；`null`/SQL NULL 变 `[]`；日志计数正确 | PG 迁移测试 | `backend-go/internal/platform/database/heal_dangling_article_refs_migration_test.go` |
| 6 | 迁移在已修复库上重跑 | 存量修复：重复执行不改变已修复数据 | 命中 0 行、零 UPDATE（`xmin` 未变） | PG 迁移测试 | 同上 |
| 7 | 生成日报：候选 id 在写库前被删（TOCTOU，含 Step7.5 物化轨） | 写路径：候选文章在写库前被删除 / 全部候选都已消失 | 写入数组不含被删 id；全删则写 `[]`；thread 仍落库 | PG 单元测试（orchestrator 过滤函数） | `backend-go/internal/topicgraph/service/daily_report_article_filter_test.go` |
| 8 | 存在性校验查询报错 | 写路径：存在性校验查询失败时降级 | 记 Warn，仍按候选写入，不阻断 | PG 单元测试（同上） | 同上 |
| 9 | 日报 job 收尾 | 可观测：日报 job 收尾记录悬空计数 | 计数器 0/>0/nil db/不可达 DSN 四态；job 里探针失败仍返回成功结果（SQLite 环境无 `daily_report_threads`，天然走降级分支） | 单元测试 | 计数器：`backend-go/internal/platform/articlerefs/repair_test.go`；job 胶水降级：`backend-go/internal/admin/scheduler/job_daily_report_test.go` |

## 继承与调整（改契约的旧资产反查）

本 change 为纯 ADDED（新 capability `article-reference-integrity`），未 MODIFIED/REMOVED 既有 Requirement，无契约变更 → 旧资产无需逐行处置。但触碰到两处既有实现，回归资产必须跑：

| 既有资产 | 处置 | 理由 |
| --- | --- | --- |
| `backend-go/internal/platform/database/dedupe_rss_articles_migration_test.go` | 跑（并新增 1 个「归并维护 thread 引用」用例） | 归并迁移新增 rewire 步骤，不得破坏原 keeper 选择/边重指语义 |
| `backend-go/internal/reader/service/feed_service_test.go` | 跑 | 删源路径改走仓库集体事务，不得破坏既有删源行为 |
| `backend-go/internal/reader/handler/feed_handler_test.go` / `category_handler` 相关测试（如有） | 跑 | 两个 handler 的删除入口改调仓库事务方法，不得改变 HTTP 响应形状 |
| `backend-go/internal/topicgraph/service/*_test.go`（daily_report 系列） | 跑 | orchestrator 新增存在性过滤，不得改变 thread 内容/顺序 |

## 变体走查

### 输入
| 变体 | 答案 | 落点 |
| --- | --- | --- |
| 空数组 `[]` | 无命中，零 UPDATE | helper 单测 |
| 单元素 = 被删 id | 变 `[]`（jsonb array） | helper 单测 + PG |
| 单元素 ≠ 被删 id | 原样不动 | helper 单测 |
| 多元素含被删 id | 替换/剔除后保序 | helper 单测 |
| 同一 id 在数组中出现两次 | 去重为一次（首次位置） | helper 单测 |
| JSON `null` / SQL NULL | 规范化为 `[]` | PG 迁移测试 |
| 超长数组（>10 元素，模拟 `slice(0,10)` 消费） | 保序，前 10 个顺序不变 | helper 单测 |
| 脏元素（非数字文本如 `"abc"`、空串） | 视作不存在 → 剔除；**实现必须避免 `::bigint` 强转**（用 `a.id::text = elem` 文本比对） | helper 单测 |

### 前置
| 变体 | 答案 | 落点 |
| --- | --- | --- |
| 空集（没有任何 thread 引用被删文章） | 不产生 UPDATE | helper 单测 |
| 单元素 / 重复 / 越界引用（指向不存在 id） | 覆盖（见输入组） | helper 单测 |
| 部分满足（一批 id 中部分存在） | 只剔除不存在的那部分 | helper 单测 |
| 保留条已在数组内 | 去重（节拍 2） | PG 迁移测试 |

### 时间窗口
不适用：本 change 无时间窗口/日历天语义（留痕）。唯一时间相关形态是「生成读候选 → 写库前被删」的 TOCTOU，已由节拍 7 覆盖。

### 幂等
| 变体 | 答案 | 落点 |
| --- | --- | --- |
| 修复迁移重复执行 | 第二次命中 0 行、零 UPDATE | PG 迁移测试（节拍 6） |
| 部分失败重试 | 迁移在事务内，Up 失败整体回滚且版本不记录，下次启动重跑（沿用既有 migrator 语义） | PG 迁移测试（人为注入错误用例可选，至少断言事务性） |
| 并发（迁移 vs 日报生成） | 非线程安全声明；靠写路径存在性校验兜底（节拍 7） | 单测 + 说明 |

### 可用性（UI）
不适用（`ui-impact: none`，本 change 不改前端）。补检一项：前端既有降级分支（`文章 #id`）保留不动，修复后不应再被触发（人工：打开今日日报线索展开列表抽查，落点见「验证」节）。

## 效果核对

| 核对项 | 触发原因 | 方法 | 预期 | 结论（实现后填） |
| --- | --- | --- | --- | --- |
| 今日 9 条悬空引用清零 | 修复迁移对存量数据的效果不能只靠单测 | 对真库跑 `SELECT count(*) ... 悬空引用` | 0 | 待实现后回填 |
| 历史 5595 个悬空引用清零 | 同上 | 同上 | 0 | 待实现后回填 |
| 修复后再跑一次日报生成不产生新悬空 | 验证写路径过滤生效 | 重跑今日日报 + 复查悬空计数 | 悬空数仍为 0 | 待实现后回填 |

## 白盒附加（复杂档：分支 + 边界）

### `RewireArticleRefs` / `PruneArticleRefs` 分支表
| 分支 | 条件 | 期望 |
| --- | --- | --- |
| B1 | 命中行数 0 | 返回 0，无 UPDATE |
| B2 | 数组含 old 1 次、不含 keeper | old→keeper，长度不变 |
| B3 | 数组含 old 多次 | 全部替换为首个 keeper 位置，其余丢弃 |
| B4 | 数组含 old 且含 keeper | 保留首个 keeper，丢弃后续重复 |
| B5 | 数组 = `[]` / JSON null | `[]` 不为 null；PruneAggregate 不报错 |
| B6 | 脏元素（非数字） | 按不存在处理并剔除，无 cast 错误 |
| B7 | prune ids 为空 | 直接返回 0，不发起查询 |

### `PruneDanglingRefs` 分批
| 边界 | 期望 |
| --- | --- |
| batch <= 0 | 用默认批次 500 |
| 行数恰好整除 batch | 最后一批为空后终止，不死循环 |
| 行数有余数 | 余数批处理后终止 |
| 单行 JSON null | 先被 `NormalizeThreadRefs` 处理，不进入 prune 查询 |

### 关键不变量（grep 验证，实现后跑）
- 删除文章行的两条活路径都调用维护器：`grep -rn 'PruneArticleRefs\|RewireArticleRefs' backend-go/internal` 命中 `postgres_migrations.go`（归并）与 `reader/repository`（删源）。
- 无新增 `DELETE FROM articles`：`grep -rn 'DELETE FROM articles' backend-go/internal` 仍只有归并一处。
- thread 写路径不再裸 `json.Marshal`：`grep -rn 'json.Marshal(th\.\|json.Marshal(mergedTagIDs' backend-go/internal/topicgraph/service` 无输出。
