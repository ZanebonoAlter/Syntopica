# Tasks — dedupe-rss-articles

## 文档

<!-- doc-impact: flow, database -->

### Scenario → 测试文件映射（scenario-trace）

| Scenario | 测试文件 |
| --- | --- |
| 快讯同 link 改标题不产生新文章 | backend-go/internal/reader/service/feed_service_test.go |
| 跨 feed 同 link 各自保留 | backend-go/internal/reader/service/feed_service_test.go |
| 同 feed 并发刷新不产生重复 | backend-go/internal/platform/database/dedupe_migration_test.go（拟定） |
| 内容未实质变化时跳过 | backend-go/internal/reader/service/feed_service_test.go |
| 内容实质变化时更新并重走处理链 | backend-go/internal/reader/service/feed_service_test.go |
| 处理链完成事件触发重打标 | backend-go/internal/tagmanagement/service/core/（tag job 处理测试，拟定） |
| 复制已有打标结果 | backend-go/internal/tagmanagement/service/core/article_tagger_test.go（拟定） |
| 手动重打标不复用 | 同上 |
| 保留条按处理链完整度选择 | backend-go/internal/platform/database/dedupe_migration_test.go（拟定） |
| 关联数据无损合并 | 同上 |
| 迁移幂等 | 同上 |
| 唯一索引随迁移建立 | 同上 |
| 人工：部署后重复组归零 SQL 核对 | tasks 3.3 验证 SQL（不设测试文件） |

## 1. 入库判重与更新语义（feed_service.go）

- [x] 1.1 `RefreshFeed` 判重快照 titleSet → linkSet（Pluck link），命中 link 的条目不再插入；单测：同 feed 同 link 不同 title（快讯滚动场景）不产生新行、跨 feed 同 link 各自插入——落点 `internal/reader/service/feed_service_test.go`
- [x] 1.2 link 命中后加载已存条目，title+description 均未变则跳过（不触发处理链）；单测覆盖未变跳过路径——落点同上
- [x] 1.3 内容变化时的 UPDATE 路径：内容字段更新 + 状态按 `buildArticleFromEntry` 同款规则重置（firecrawl/summary 开关组合四种）+ 衍生字段清空（firecrawl_content/firecrawl_error/ai_content_summary/completion_*）+ 旧 `article_topic_tags` 删除（复用 RetagArticle 清理模式含 CleanupOrphanedTags）+ `enqueueArticleProcessing` 重走链；单测验证状态重置矩阵与旧标签清理——落点同上 + `internal/tagmanagement/service/core/article_tagger*` 联动
- [x] 1.4 确认重打标由处理链事件接力（tag_jobs reason=firecrawl_completed/summary_completed/article_created 到达时 existingCount=0 正常 AI 打标）；集成测试模拟链条完成事件——落点 `internal/tagmanagement/service/core/`

## 2. 跨 feed 打标复用（tagmanagement）

- [x] 2.1 `tagArticle`（非 Force）在 existingCount 检查后插入复用分支：查同 link 其它副本的 article_topic_tags（JOIN articles），命中则逐条复制（防 (article_id, topic_tag_id) 冲突）、source='reuse'、score 沿用、tag_count 按实际插入数更新，不调 AI；单测：复用命中 0 次 AI 调用、冲突防御、未命中走原路径——落点 `internal/tagmanagement/service/core/article_tagger*`
- [x] 2.2 `RetagArticle`（Force）路径确认不复用（清旧标签后重新 AI 提取）；单测——落点同上
- [x] 2.3 `articles.link` 索引：生产由任务 3.1 迁移建立；测试环境（sqlite AutoMigrate）补建，复用查询不依赖全表扫——落点 model tag 或测试 setup

## 3. 存量归并迁移（postgres_migrations.go）

- [x] 3.1 新增版本化迁移 `20260917_0001`（单事务）：`idx_articles_link` 普通索引 → 重复组识别（feed_id+link, link != ''）→ 保留条打分（!archived×1000 + tag_count>0×100 + firecrawl_content×10 + ai_content_summary×10 + read|favorite×1，并列取 id 最小）→ 副本 article_topic_tags/reading_behaviors 归并（冲突删副本行）→ tag_jobs/firecrawl_jobs（completed 删、pending/processing 改指向）→ 删副本行 → 保留条 tag_count 重算 → `CREATE UNIQUE INDEX uq_articles_feed_link ... WHERE link != ''`；实现按 design D4
- [x] 3.2 迁移 PG 集成测试：混合 archived 组保留 active、标签冲突合并无损、pending job 改指向、幂等重跑 no-op、注入失败全回滚——落点 `internal/platform/database/`（仿既有迁移测试模式）
- [x] 3.3 归并效果验证 SQL（部署后人工核对）：同 feed 重复组归零、跨 feed 组保留、保留条 tag_count 一致

## 4. 收尾验证

- [x] 4.1 影响包测试全绿：`go test ./internal/reader/... ./internal/tagmanagement/... ./internal/platform/database/`（经 `scripts/change-scope.sh` 判定范围）
- [x] 4.2 本地端到端：启动后端（迁移自动执行）→ 刷新华尔街见闻快讯 feed 两次 → 确认无新增重复行、ai_call_logs 无同篇重复 extractor 调用；观察日志 `[cleanup]` 窗口计数正常
- [x] 4.3 完工汇报含"部署后影响 + 需要的操作"（迁移随启动自动跑、建议部署前 pg_dump、旧重复数据归并后打标记录不再重复展示）
