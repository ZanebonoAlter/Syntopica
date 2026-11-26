## 1. 数据层迁移

- [x] 1.1 迁移 A：`postgres_migrations.go` 新增迁移——`CREATE EXTENSION IF NOT EXISTS pg_trgm` + `articles.title`/`articles.description` 两列 `gin_trgm_ops` GIN 索引；testcontainer PG 跑迁移后 `\d articles` 断言索引存在、`show_trgm('特斯拉电池')` 返回多 trigram
- [x] 1.2 迁移 B：DROP `articles.search_vector` 列 + `articles_search_vector_trigger` 触发器 + `idx_articles_search_vector` 旧索引；同步移除 `feed_service.go` 归档 updates 中 search_vector 置 NULL 分支；testcontainer 迁移后断言列/触发器不存在，`go test ./internal/reader/service -run TestCleanup` 通过

## 2. 后端搜索服务

- [x] 2.1 `internal/reader/search/` 建包：词典缓存（3~8 字 `topic_tags.label`，启动加载 + 写入失效重载）与正向最大匹配切词；函数单测覆盖多词命中/最长优先/空串/纯英文（design D2）
- [x] 2.2 查询预处理：L0 空白拆分（含全角）→ L1 词典切词 → L2 整串 → L3 trigram（阈值 `search_trigram_threshold` 默认 0.10，非法回退+warn）；LIKE 通配符转义（% _ \\）与 200 字符截断；函数单测覆盖 spec「空与纯空白查询拒绝」「特殊字符与超长查询」两 Scenario
- [x] 2.3 关键词路径 repository：trigram 索引 LIKE + `similarity()` SQL（testcontainer PG 断言：词重组查询命中换序标题、trigram 分数区间 0.1~0.3、归档行含入与 active-only 过滤两分支）
- [x] 2.4 语义路径：query embedding（airouter 接口 mock）→ identity 向量 cosine 双约束（≥`search_semantic_min_cosine` 默认 0.55 ∧ top `search_semantic_top_n` 默认 8）→ `article_topic_tags` 召回 + max 聚合；单测覆盖 spec「双重约束边界」「embedding 不可用降级」两 Scenario（3s 超时 context）
- [x] 2.5 RRF 融合 + 命中层级加权（整串 1.0/词典多词 0.85/单词 0.6/trigram 原始分 × 字段权重 title 1.0/desc 0.7）；函数单测含「多词命中 > 单词命中」排序断言
- [x] 2.6 `GET /api/search` handler：q/archived/expand/page/per_page 参数校验（spec 非法分页 400、空 q 400）、hit_reasons 与 semantic 块结构；handler 测试覆盖响应契约
- [x] 2.7 LLM 扩展路径：expand=on 时 airouter chat 产出 5~8 扩词 JSON、补跑 L1/L2、type=expand 区隔；10min TTL 缓存；LLM 失败主结果正常（单测 mock 失败分支）
- [x] 2.8 `article_handler.go` 的 tsquery 分支切换为 D2 管线（仅活跃集）：`bash scripts/harness/test-assets.sh article-list-projection` 反查旧测试资产，继承与调整逐行处置进 test-cases.md；handler 测试断言中文 search 命中与 LIKE 口径一致、归档默认排除不变

## 3. 前端

- [x] 3.1 `features/search/composables/useSearch.ts`：fetch + 状态机（initial/loading/empty/error/success/降级）+ 扩展开关 + offset 分页；Vitest 覆盖状态迁移
- [x] 3.2 `front/app/pages/search.vue` + 工具条 + 结果列表 + 状态矩阵（骨架屏/空态扩展引导/错误重试/降级提示条）；组件测试断言各状态渲染与「开启 LLM 扩展再试」一键重查（spec「无结果引导扩展」「语义降级提示」「双主题渲染」Scenario）
- [x] 3.3 `ArticleCardView.vue` 新增可选 props `hitReasons`/`archived`（缺省行为不变，回归现有列表测试）；Vitest 断言 chip 三型渲染与归档角标
- [x] 3.4 `ArticlePreviewModal.vue` 新增可选命中词高亮 prop（不改 tags 域现有行为，回归其现有测试）；Vitest 断言正文高亮与关闭返回
- [x] 3.5 `AppSidebarView.vue` 新增「文章搜索」入口（发现订阅源之后）+ mdi:magnify 重新生成 iconify-subset.json；组件测试断言入口顺序与跳转
- [x] 3.6 agent-browser 端到端主链路：入口→输入连续短语→结果渲染（命中 chip/归档角标存在性）→点卡片预览弹窗开（命中词高亮）→阅读全文跳转→Esc 返回原位（spec「点击卡片打开预览」「关闭返回原位」Scenario），静态托管 :5100 上跑

## 4. 测试

- [x] 4.1 编写 `test-cases.md`（用例先行，已前置至实现开工前完成）：以 spec 7 个 Requirement 为主链路表（步/动作/来源 Scenario/期望/层/落点），变体五组走查（输入：空串/纯空白/通配符/超长；前置：空库/无标签库；时间：无窗口语义；幂等：重复查询缓存；可用性：误输入反馈/空态/错误态）不适用划除留痕，效果核对节（中文召回修复前后对比：触发原因/方法/量化结果/结论），白盒附加（阈值边界值表：trigram 0.10/0.09/0.11、cosine 0.55 边界、top_n 边界）
- [x] 4.2 双视口视觉检查：1440×900 与 1920×1080 agent-browser 截图断言 contained ≤1120 居中、无横向溢出；证据落 `ui-verification/report.md`

## 5. 文档

<!-- doc-impact: flow, api, database, standard -->

- [x] 5.1 `docs/reference/flow/reading.md`：新增「站内搜索」节（端点契约/查询管线/归档口径/降级链）+ 变更溯源行；`flow/topic-graph.md` 变更溯源补「搜索只读消费标签向量与边」说明
- [x] 5.2 `docs/reference/api/search.md` + `docs/reference/api/_index.md`：搜索端点文档与导航行；`docs/reference/database/tables/content.md`：pg_trgm 索引与 search_vector 移除说明；`docs/reference/standard/backend/ai-logging.md`：AI 调用日志用途表登记 search 查询向量化与 LLM 扩词两用途

## 6. 验证

- [x] 6.1 `cd backend-go && golangci-lint run ./internal/reader/... && go vet ./internal/reader/...` → 0 error
- [x] 6.2 `cd backend-go && go test ./internal/reader/search/... ./internal/reader/handler/... ./internal/reader/service/... --tags=integration`（testcontainer 环境）→ 全绿，含迁移 A/B 与端点切换用例
- [x] 6.3 `cd front && pnpm lint && pnpm exec nuxi typecheck` → 0 error
- [x] 6.4 `cd front && pnpm test:unit features/search pages/search.test features/articles/components/ArticleCardView features/tags/components/ArticlePreviewModal --maxWorkers=2` → 全绿（不带 `--`）
- [x] 6.5 真库效果核对（部署后，2026-10-14 归档前复验）：`curl --noproxy '*' 'http://localhost:5100/api/search?q=特斯拉'` → `data.total=323 ≥ 50`（旧 tsvector 路径 2 篇；per_page 默认 20 故 `.data.results | length`=20，召回量以 total 为准），结果全部携带 `hitReasons`（keyword/semantic 两型实渲染）且响应含 `semantic` 块（标签+cosine 分数）
- [x] 6.6 双主题走查（agent-browser 机械断言替代人工，2026-10-14）：/search 页 editorial/dark 切换无未适配区块（四角实际绘制均主题色、文本白系 token、无实底白块），归档角标与命中 chip 色正确（chip teal 系语义色设计上不随主题）；证据 ui-verification/report.md A9-A11 + theme-editorial-search.png / theme-dark-search.png

Scenario→测试映射表（scenario-trace 对账，scenario-test-mapping-gate）：

| Scenario | 测试文件 |
| --- | --- |
| 关键词路径命中并返回解释 | backend-go/internal/reader/search/handler_test.go |
| 语义路径命中返回标签与分数 | backend-go/internal/reader/search/semantic_pg_test.go |
| 非法分页参数 | backend-go/internal/reader/search/handler_test.go |
| 词典词重组查询命中 | backend-go/internal/reader/search/repository_pg_test.go |
| 连续短语的 trigram 兜底 | backend-go/internal/reader/search/repository_pg_test.go |
| 空与纯空白查询拒绝 | backend-go/internal/reader/search/handler_test.go |
| 特殊字符与超长查询 | backend-go/internal/reader/search/preprocess_test.go |
| 双重约束边界 | backend-go/internal/reader/search/semantic_pg_test.go |
| embedding 不可用降级 | backend-go/internal/reader/search/semantic_pg_test.go |
| LLM 扩展结果区隔 | backend-go/internal/reader/search/expand_test.go front/app/pages/search.test.ts |
| 默认含归档 | backend-go/internal/reader/search/repository_pg_test.go |
| 仅活跃过滤 | backend-go/internal/reader/search/repository_pg_test.go front/app/pages/search.test.ts |
| 点击卡片打开预览 | front/app/pages/search.test.ts front/app/features/tags/components/ArticlePreviewModal.test.ts openspec/changes/add-article-search/ui-verification/report.md |
| 阅读全文跳转 | front/app/pages/search.test.ts openspec/changes/add-article-search/ui-verification/report.md |
| 关闭返回原位 | front/app/pages/search.test.ts openspec/changes/add-article-search/ui-verification/report.md |
| 无结果引导扩展 | front/app/pages/search.test.ts |
| 语义降级提示 | front/app/pages/search.test.ts |
| 双主题渲染 | 人工：agent-browser 双主题走查（tasks 6.6，证据 ui-verification/report.md） |
| 中文关键词不再漏召回 | backend-go/internal/reader/handler/article_handler_test.go |
| 归档排除保持 | backend-go/internal/reader/handler/article_handler_test.go |

## 7. Review

- [x] 7.1 派子线程聚焦 review（审查制品 + diff），意见按 High/Medium/Low 处置后 review.md 建账
