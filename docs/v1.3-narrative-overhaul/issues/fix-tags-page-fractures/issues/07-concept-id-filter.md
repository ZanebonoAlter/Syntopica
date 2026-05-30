Status: ready-for-agent

## Parent

fix-tags-page-fractures

## What to build

后端 `GetArticles` handler 添加 `concept_id` 查询参数过滤。当前前端在点击板块时发送 `concept_id` 参数但后端完全忽略，导致板块时间线显示全量文章而非板块相关文章。

实现：解析 `concept_id` 参数，通过 JOIN `article_topic_tags` + `topic_tags` 过滤出标签属于指定 concept 的文章。count 查询同步支持 `concept_id`。

同时补充 sector diff 执行链路的结构化日志。

## Acceptance criteria

- [ ] `GetArticles` 解析 `concept_id` query param
- [ ] `concept_id > 0` 时 JOIN 过滤：`article_topic_tags` → `topic_tags.concept_id = ?`
- [ ] count 查询同步支持 `concept_id`（使用 `COUNT(DISTINCT articles.id)`）
- [ ] 新增测试 `TestGetArticlesFiltersByConceptID` 验证过滤逻辑
- [ ] sector regenerate/confirm/execute 三个环节添加 `logging.Infof` 日志
- [ ] `go test ./internal/domain/article -run TestGetArticlesFiltersByConceptID` 通过
- [ ] `golangci-lint run ./...` + `go build ./...` 通过

## Blocked by

None - can start immediately（纯后端任务，无前端依赖）

## Reference

- Plan: `docs/plans/2026-05-17-fix-phase14-16-bugs.md` Task 4 + Task 5
- `backend-go/internal/domain/article/handler.go:63-220`
- `backend-go/internal/domain/narrative/sector_handler.go:229-275`
- `backend-go/internal/domain/tagging/sector_generation.go:393-554`
