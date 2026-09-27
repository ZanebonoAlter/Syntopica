# apply-report: dedupe-rss-articles

> 实现由子线程（worker）启动、额度耗尽后由主线程接手完成。以下为最终实现账本。

## 改动文件

| 文件 | 内容 | 任务 |
| --- | --- | --- |
| `backend-go/internal/reader/service/feed_service.go` | `RefreshFeed` 判重快照 titleSet → linkSet；命中 link 走 `refreshExistingArticle`（内容未变跳过 / 变化则 UPDATE 内容 + 状态按 `buildArticleFromEntry` 重置 + 清衍生字段 + 删旧标签 + `enqueueArticleProcessing` 重走链）；并发冲突注释说明 | 1.1-1.3 |
| `backend-go/internal/reader/service/feed_service_test.go` | +317 行：判重、跨 feed 保留、更新语义、状态重置矩阵、旧标签清理 | 1.1-1.3 |
| `backend-go/internal/tagmanagement/service/core/article_tagger.go` | 跨 feed 复用分支（前置，补齐语义，`maxArticleTags` 上限，`source='reuse'`，raw SQL 维护 tag_count） | 2.1-2.2 |
| `backend-go/internal/tagmanagement/service/core/article_tagger_reuse_test.go` | +8 个测试：复用 0 AI 调用、冲突防御、未命中原路径、Force 不复用、空 link 防御、上限截断、饱和副本、更新后重打标 | 2.1-2.2, 1.4 |
| `backend-go/internal/models/article.go` | `Link` 加 `index` tag（测试环境 AutoMigrate 建索引；生产由迁移建同名索引） | 2.3 |
| `backend-go/internal/platform/database/postgres_migrations.go` | 迁移 `20260917_0001` + `mergeDuplicateArticleGroup`：idx_articles_link → 重复组归并（打分保留条 / 标签合并 / 行为改指向 / jobs 改指向或删 / 删副本 / tag_count 重算）→ 唯一部分索引 | 3.1 |
| `backend-go/internal/platform/database/dedupe_rss_articles_migration_test.go` | +4 个 PG 迁移测试 | 3.2 |
| `openspec/changes/dedupe-rss-articles/verification.sql` | 部署后 6 条核对查询 | 3.3 |

## 门禁结果

| 项 | 命令 | 结果 |
| --- | --- | --- |
| build | `go build ./...` | ✅ |
| lint | `golangci-lint run`（reader/tagmanagement/database/models） | ✅ 0 issues |
| vet | `go vet`（同上四包） | ✅ |
| 影响包测试 | `go test ./internal/tagmanagement/service/core/... ./internal/platform/database/... ./internal/reader/service/...` | ✅ 三包全绿（14.1s / 36.3s / 17.2s） |
| 复用单测 | 8 个 reuse/retag 用例 | ✅ |
| 迁移测试 | 4 个 PG 用例（归并/归档优先/幂等/唯一索引） | ✅ |

> 测试需 PG：本机 `TESTCONTAINERS_RYUK_DISABLED=true` 绕过 Docker Hub 拉取 ryuk 超时（pgvector 镜像本地已有）。质量门禁本会话因 WSL interop 故障整轮短路，上述命令为主线程手动补跑。

## 与 design/制品的偏差（均已在 design.md / spec.md 同步）

1. **复用分支位置**：design 初稿写"在 `existingCount` 检查之后"，同时又要求"跳过本副本已存在的 `(article_id, topic_tag_id)`"——两者自相矛盾（跳过后本副本必无标签）。**改为前置**（幂等补齐：部分标签副本向同 link 完整集合收敛），并在 design D3 记录理由。spec 增补 Scenario「部分标签副本补齐到上限」。
2. **上限保护**：补齐不得突破 `maxArticleTags=6`（按 score 降序取高分标签）。
3. **tag_count 维护**：`gorm:"->"` 只读字段导致 GORM `Update()` 静默跳过 → 改 raw SQL `Exec`（迁移内本就是 raw SQL，故迁移测试先通过）。
4. **迁移打分判据**：用 `EXISTS(article_topic_tags)` 而非 `tag_count>0` 列（后者是冗余计数，EXISTS 更强且不依赖可能过期的列）。
5. **reading_behaviors 全量改指向**（design 写"冲突则删"）：该表无唯一约束、每行是一次真实行为事件，合并保留完整阅读历史更符合语义。

## 真实库执行与端到端核对（tasks 4.2）

迁移随用户后端重启在真实库自动执行（`schema_migrations` 记录 `20260917_0001` @ 2026-09-16 23:28:59），主线程随后核对：

| 核对项 | 结果 |
| --- | --- |
| 同 feed 重复组 | **409 → 0** |
| 跨 feed 重复组 | 126（预期保留） |
| `tag_count` 与实际标签数 | 0 不一致 |
| 索引 | `idx_articles_link` + `uq_articles_feed_link WHERE link <> ''` 均在 |
| 悬挂 job | tag_jobs / firecrawl_jobs 均 0 |
| 运行时判重（刷新 feed 24 两次） | 第一次 +19 篇新文章且重复组为 0；第二次 **0 新增、0 重复**（同 link 命中走更新/跳过） |

## 验收发现（本 change 范围外，已知项）

**日报线程的 jsonb 文章引用可能悬挂**：`daily_report_threads.related_article_ids`（jsonb 数组，无 FK）用文章 ID 引用文章，实测 5595 个引用查不到文章（影响 4774 个 thread）。判定：

- **以历史遗留为主**：悬挂 ID 中 5266 个 < 100000（老文章），且 `articles` 历史上 ID 缺口达 11 万（最大 ID 129570 vs 现存 18853 行）——早期版本曾大规模物理删除文章。
- **本次迁移贡献极小**：迁移只删了 432 行（409 组 × 平均 2.4 副本），实测悬挂 ID 中 > 118000 的仅 51 个。
- **消费端天然容错**：所有读取路径用 `JOIN articles ON a.id = aid.article_id::bigint`（INNER JOIN，`daily_report_repository.go:557/690`），悬挂 ID 被静默过滤，**不报错**，仅少一个封面图/来源候选。
- **design D4 未覆盖此项**（仅处理了 4 张有 FK 的关联表）。因本库迁移已执行且消费端容错，未在本 change 内追改；建议单独 change 做 jsonb 引用清理（可选，附清理 SQL 思路：`jsonb_agg(DISTINCT ...)` 过滤不存在的 ID）。

## 未尽事项

- 迁移代码未处理 jsonb 悬挂引用（见上）——未来在其它库部署时会留下同类悬挂（消费端仍容错）。
- migration 4.6（失败注入回滚）未单独测试：事务原子性由既有 `RunMigrationsList` 基础设施测试覆盖（`outside_tx_test.go`），迁移本身默认事务内执行。
- 生产 `articles` 表 12 万行建索引为事务内普通索引（非 CONCURRENTLY），单用户产品可接受。

## 归档前复核（2026-09-17 01:1x，主线程）

归档门禁复跑与三处补齐：

1. **代码修复（fixup）**：`refreshExistingArticle` 删除旧 `article_topic_tags` 后补 `tag_count` 重算（raw SQL，与 reuse 路径、归并迁移同一契约）。原实现会让迁移刚重算过的 `tag_count` 在同 link 内容变化的刷新后立刻陈旧——`verification.sql` ③ 的「tag_count 与实际标签数一致」断言即被自己打破。测试 `TestRefreshFeedUpdatesChangedEntryAndClearsDerivedState` 增加 `tag_count` 归零断言（种 1 再验证刷新后归 0）。
2. **tasks.md 门禁结构补齐**：补 `## 5. 测试` / `## 6. 文档` / `## 7. 验证` 尾三节；Scenario→测试文件映射表从「拟定」改为真实路径（12/12 自动映射，`scenario-trace.sh` 退出码 0）；验证节补实测命令与期望。
3. **活文档同步**：`flow/content-enrichment.md`（新增「feed 刷新入库判重与快讯更新语义」链节 + 业务约束红线 #11）、`flow/topic-graph.md`（业务约束红线 #8：跨 feed 打标复用）、`database/tables/content.md`、`database/tables/_conventions.md`、`database/DATA_LIFECYCLE.md`（入库框补判重/upsert 分支）。

复跑结果（本机 Linux，PG 走 testcontainer）：

| 项 | 命令 | 结果 |
| --- | --- | --- |
| 影响包测试 | `go test ./internal/reader/service/... ./internal/tagmanagement/service/core/...` | ✅ 13.1s / 12.5s |
| PG 迁移测试 | `TESTCONTAINERS_RYUK_DISABLED=true go test ./internal/platform/database/...` | ✅ 35.9s |
| 定向用例 | `go test -run 'TestDedupe\|TestRefreshFeed\|TestTagArticle\|TestRetag' -v`（三包） | ✅ 18 用例全 PASS |
| lint / vet / build | `golangci-lint run`（三包）+ `go vet`（三包）+ `go build ./...` | ✅ 0 issues / 无输出 / 成功 |
| 真实库核对 | `verification.sql` ①②④⑤ | ✅ 同 feed 重复组 0、跨 feed 126（预期）、两索引在、悬挂 job 0 |

**真实库 ③ 目前 1 行不一致**（`articles.id=125717`，archived 行 `tag_count=1` 无标签）：来源是 `EdgeGC`（归档文章边的时间窗回收）删边不维护 `tag_count`——既有路径、非本 change 引入，读路径按子查询重算，不影响展示。

**未改但已知的边界**：`linkSet` 含归档行，历史归档链接若重新出现在 feed 中会走「更新 + 重走处理链」（`archived` 标记保持 true）。实测未见（归档行早已滚出 RSS 窗口）；如需收紧可给 linkSet 查询加 `archived = false`（此时重复插入会被唯一索引吞掉，语义等价）。
