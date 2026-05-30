# 排障

## 前端起不来

- 确认 `front/` 里依赖已安装
- 运行 `pnpm exec nuxi typecheck`
- 检查是否有编码问题（前端源码必须为 UTF-8）
- 检查是否有错误 import

## 后端起不来

### 数据库连接问题

- 确认 PostgreSQL 容器已启动：`docker compose ps`
- 确认容器健康：`docker compose logs postgres`
- 检查 `backend-go/configs/config.yaml` 中的 `database.dsn` 配置
- 检查 `DATABASE_DRIVER` 和 `DATABASE_DSN` 环境变量是否覆盖了配置文件
- 手动测试连接：`docker exec -it syntopica-postgres psql -U postgres -d syntopica -c "SELECT 1"`

### 迁移失败

- 检查 `schema_migrations` 表中已应用的版本号
- 查看后端日志中具体的迁移错误信息
- 确认 pgvector 扩展已安装：`SELECT extname FROM pg_extension WHERE extname = 'vector';`
- 如果某个迁移失败，修复问题后需要删除 `schema_migrations` 中对应版本的记录才能重试

### pgvector 相关问题

- 确认使用的是 `pgvector/pgvector:pg18-trixie` 镜像而非普通 PostgreSQL
- 确认 `embedding` 列类型为 `vector(1536)` 而非 `text`
- 确认 HNSW 索引存在：`\di+ idx_topic_tag_embeddings_embedding`
- 如果日志出现 `topic_tag_embeddings` 外键错误（如 `fk_topic_tags_embedding` / `SQLSTATE 23503`），先检查是否是标签清理与异步 embedding 保存并发发生：`cleanupOrphanedTags()` 可能已删除 `topic_tags` 记录，而旧的 embedding 任务仍在尝试写入
- 当前后端会在 `SaveEmbedding()` 写入前重新确认父标签是否仍存在；若不存在，会返回 `topic tag not found` 并跳过落库，而不是继续触发数据库外键异常

### JSONB 字段编码问题

- 如果日志出现 `unable to encode ... into text format for unknown type (OID 0): cannot find encode plan`，通常是 Go 自定义 map 类型被直接作为 SQL 参数传给 pgx，而不是先序列化为 JSON
- `topic_tags.metadata` 使用 `models.MetadataMap`，该类型需要保持 `driver.Valuer` / `sql.Scanner` 支持，避免 `Updates(map[string]any{...})` 绕过 GORM JSON serializer 后写入失败

## 编译/测试问题

- 后端：运行 `go test ./...` 查看失败测试
- 前端：运行 `pnpm exec nuxi typecheck` 检查类型错误
- 运行 `go build ./...` 检查编译错误

## 连接池问题

如果出现大量数据库连接超时：

- 检查 `config.yaml` 中的 `database.postgres` 连接池配置
- `max_idle_conns` 默认不限制，`max_open_conns` 默认不限制
- 可调整 `conn_max_lifetime_minutes` 和 `conn_max_idle_time_minutes`

## 文档又漂移了

- 从 `README.md` 和 `docs/README.md` 开始核对
- 以当前代码中的模型定义（`backend-go/internal/domain/models/`）为准
- 删除失效路径引用
- 不要在根目录继续新增一次性说明文档
