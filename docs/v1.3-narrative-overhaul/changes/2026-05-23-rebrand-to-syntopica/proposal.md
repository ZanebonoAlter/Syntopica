## Why

项目目前有严重的命名混乱：前端叫 "rss reader"，后端目录叫 "my-robot"，GitHub 仓库名 `ZanebonoAlter/my-robot`，Docker 容器名 `zanebono-rssreader-pgvector`，Go 模块名 `my-robot-backend`，NPM 包名 `front`。这导致维护困惑、新人上手困难、文档不一致，也无法对外传播品牌。

产品的本质早已超越 RSS 阅读器——核心是 AI 驱动的语义标签和主题板块系统（SemanticBoard）。需要一个统一的名字来承载这个身份。

## What Changes

- **产品名统一为 Syntopica**：所有面向用户的界面、README、文档使用 Syntopica
- **GitHub 仓库改名**：`ZanebonoAlter/my-robot` → `ZanebonoAlter/Syntopica`（保留 org 名不变）
- **Go 模块改名**：`my-robot-backend` → `syntopica-backend`
- **NPM 包改名**：`front` → `@syntopica/web`
- **Docker 容器/服务名**：统一为 `syntopica-*` 前缀
- **数据库名**：`rss_reader` → `syntopica`
- **README 和全项目引用**：更新所有文件中的项目名称引用
- **添加品牌标识**：定义 tagline、一句描述、icon 设计方向
- **目录结构不变**：物理目录 layout（`front/`、`backend-go/`）不重命名，降低迁移成本

## Capabilities

### New Capabilities

无。此次变更是品牌统一和命名重构，不引入新功能。

### Modified Capabilities

无。不改动任何 spec 级别的行为要求。

## Impact

- **全项目文件**：README、AGENTS.md、docker-compose.yml、package.json、go.mod 等数十个文件需更新名称引用
- **GitHub 仓库**：改名前需通知所有协作方，改后需更新 remote URL。关联 issue、PR 链接可能失效
- **Docker 镜像**：构建和推送需使用新 tag，旧 tag 保留兼容性
- **CI/CD**：可能需更新工作流配置中的仓库引用
- **现有开发环境**：`go.mod` 改名后需 `go mod tidy`，`package.json` 改名后需重装依赖
- **外部依赖方**：目前无公开 API 消费者，风险可控。如果之后 crawl 服务有集成，需同步更新
