## 1. 品牌定义

- [x] 1.1 确认 tagline 为 "Where feeds become topics"，写入 README 和文档
- [x] 1.2 确认一句描述："AI-powered feed reader that organizes information into knowledge. Feed in, topics out."
- [ ] 1.3 生成 icon（用户自行使用 AI 图像工具生成）

## 2. 仓库与基础设施

- [ ] 2.1 GitHub 仓库改名：`ZanebonoAlter/my-robot` → `ZanebonoAlter/Syntopica`
- [x] 2.2 更新 docker-compose.yml：容器名 `zanebono-rssreader-pgvector` → `syntopica-postgres`、默认数据库名 `rss_reader` → `syntopica`
- [x] 2.3 更新 docker-compose.sqlite.yml：服务名 `front` → `syntopica`、默认 DB 文件名 `rss_reader.db` → `syntopica.db`
- [x] 2.4 更新 Dockerfile：二进制名 `rss-reader` → `syntopica`、默认 DSN 中 `rss_reader.db` → `syntopica.db`
- [x] 2.5 更新 `.dockerignore`：`rss_reader` / `rss-reader` → `syntopica`
- [x] 2.6 更新 Docker 相关文档中的引用

## 3. 后端名称更新

- [x] 3.1 go.mod：module `my-robot-backend` → `syntopica-backend`
- [x] 3.2 批量替换所有 Go 文件中 import 路径 `my-robot-backend/` → `syntopica-backend/`（~124 文件）
- [x] 3.3 提取 tracing service name 为全局常量：`const ServiceName = "syntopica"`，替换 7 个文件中 19 处硬编码 `"rss-reader-backend"`
- [x] 3.4 `router.go`：API root endpoint `"name": "RSS Reader API (Go)"` → `"Syntopica API"`
- [x] 3.5 `config.go` + `configs/config.yaml`：默认 `dbname=rss_reader` → `dbname=syntopica`
- [x] 3.6 运行 `go mod tidy` 清理残留
- [x] 3.7 `go vet ./...` + `go build ./...` 验证无报错

## 4. 前端名称更新

- [x] 4.1 package.json：`name: "front"` → `name: "@syntopica/web"`
- [x] 4.2 重新运行 `pnpm install` 生成新 lockfile
- [x] 4.3 `AppHeaderView.vue`：logo 文本 `"RSS Reader"` → `"Syntopica"`
- [x] 4.4 `ArticleContentView.vue`：浏览器 tab title `" - RSS Reader"` → `" - Syntopica"`
- [x] 4.5 `pnpm lint` + `pnpm exec nuxi typecheck` 验证无报错

## 5. 文档与配置文件

- [x] 5.1 README.md：产品名称、描述、仓库地址、文档树中的虚拟根目录
- [x] 5.2 AGENTS.md（根目录）、`backend-go/AGENTS.md`、`front/AGENTS.md` 中的项目名引用
- [x] 5.3 docs/reference/ 下所有活文档中的项目名引用（`rss_reader`、`RSS Reader`、`zanebono-rssreader-pgvector`、`my-robot-backend` 等）
- [x] 5.4 openspec/specs/ 中所有 spec 文件中的项目名引用（仅文本替换，不改功能描述）
- [x] 5.5 `.env.example`：`SQLITE_DB_FILE=rss_reader.db` → `syntopica.db`
- [x] 5.6 `.gitignore`：`rss_reader.db`、`rss_reader_backup.db` → `syntopica.db`、`syntopica_backup.db`
- [x] 5.7 LICENSE 无需修改（标准 GPLv3 模板，无项目名）
- [x] 5.8 docs/v1.x/ 历史里程碑文档保留不改

## 6. 验证

- [x] 6.1 全项目 grep 旧名 `my-robot`、`rss-reader`、`rss_reader`、`RSS Reader`、`zanebono-rssreader` 确认无遗漏（排除 `docs/v1.x/`、`node_modules/`、`.git/`）
- [ ] 6.2 `gitnexus_detect_changes()` 确认无意外影响符号
- [ ] 6.3 前后端全量构建验证
