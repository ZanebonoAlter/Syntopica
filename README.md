<!-- generated-by: gsd-doc-writer -->

# RSS Reader

基于 Go + Nuxt 4 的个人 RSS 阅读器，三栏阅读界面，支持 AI 智能增强与主题图谱。

你知道的，我一直想追踪一些事件的蛛丝马迹，比如事件之间的关联、事件的时间线发展（比如伊朗战争）
互联网没有记忆，很多事情会随着时间沉淀在互联网的大海深处，打捞非常困难
但是对于我们现在来说，使用AI去重、整理、打标签、梳理事件链路是一件相对来说有意义、并且有可行性的事情
让垃圾信息见ai去吧！你只需要看结果（ps.此情况只针对广告较多但是还是有真金白银的rss）

![主界面截图](img/image-main.png)

## ✨ 核心功能

### 主题图谱
- **图谱可视化**：日/周双视图，事件/人物/关键词三类节点与关联边，支持权重计算与时间窗口切换
![主题图谱](img/image-topic.png)
- **AI 主题分析**：按标签类型（事件/人物/关键词）生成 AI 分析，含时间线、人物画像、关键词云等
![category](img/image-category.png)
- **叙事线追踪**：主题演变状态（新出现/持续/分裂/合并/结束）与时间线回溯
![story](img/image-story.png)


![主题图谱文章](img/image-topic-article.png)

### 📰 订阅管理
- Feed 管理：添加、编辑、删除、手动刷新、全量刷新
- 分类管理：自定义名称、图标、颜色
- OPML 导入导出
- 可配置自动刷新间隔

![订阅管理界面](img/image-feed.png)

### 📖 文章阅读
- FeedBro 风格三栏布局
- 收藏、已读标记、全屏阅读
- 预览模式与 iframe 模式切换
- 上一篇/下一篇快速导航

![文章阅读界面](img/image-article.png)

### 🤖 智能增强
- Firecrawl 全文抓取，补全 RSS 摘要内容
- AI 内容整理，生成结构化正文
- 内容源切换：原始内容 / Firecrawl 全文 / AI 整理稿

![内容增强状态面板](img/image-improve.png)

### ⚙️ 全局配置
- **AI Provider 路由**：多模型管理，按能力（总结/正文补全/主题提取/嵌入）分配不同 Provider，支持主备与拖拽排序
![router](img/image-router.png)
- **Firecrawl 服务**：配置 API 地址、Key、抓取模式、超时与内容长度限制
![fircrawl](img/image-fircrawl.png)
- **调度器监控**：查看 AI 总结、Feed 刷新等定时任务状态，支持手动触发与间隔调整
![fircrawl](img/image-scheduler.png)
- **队列管理**：实时监控标签打标队列、Embedding 队列的任务状态与失败重试
![queue](img/image-queue.png)
- **Feed 级设置**：单独配置每个订阅源的刷新间隔、最大保留文章数、AI 摘要开关
![queue](img/image-feed-global.png)

### 📊 阅读偏好
- 自动追踪阅读行为（打开、关闭、滚动、收藏）
- 偏好分数计算，优化排序
- 阅读统计展示
![queue](img/image-prefrence.png)

## 🛠 技术栈

| 层级 | 技术 |
|------|------|
| 前端 | Nuxt 4 + Vue 3 + TypeScript + Pinia + Tailwind CSS v4 |
| 后端 | Go + Gin + GORM + Postgres |
| AI | OpenAI 兼容 API |

## 🚀 快速开始

### 前置条件

- [Node.js](https://nodejs.org/) >= 18
- [pnpm](https://pnpm.io/) >= 10
- [Go](https://go.dev/) >= 1.25
- [Docker](https://www.docker.com/)（可选，用于容器化部署）

### Docker Compose（推荐）

咳咳，pg这个版本的我还没改
```bash
cp .env.example .env
docker compose -f docker-compose.yml up --build
```

- 前端默认地址：`http://localhost:3000`
- 后端默认地址：`http://localhost:5000`
- SQLite 文件默认落在仓库根目录 `data/rss_reader.db`
- 如需自定义端口或代理，在 `.env` 中配置 `FRONT_PORT`、`BACKEND_PORT`、`GOPROXY`、`NPM_CONFIG_REGISTRY` 等

如需 PostgreSQL（支持 pgvector 向量搜索），先启动数据库：

```bash
docker compose up -d
```

### 前端

```bash
cd front
pnpm install
pnpm dev
```

前端开发服务器默认运行在 `http://localhost:3000`。

### 后端

```bash
cd backend-go
go mod tidy
go run cmd/server/main.go
```

后端默认运行在 `http://localhost:5000`。

## 📂 项目结构

```
ZanebonoRssReader/
├── front/                    # Nuxt 4 前端（Vue 3 + TypeScript + Pinia）
├── backend-go/               # Go + Gin 后端（GORM + SQLite）
├── docs/                     # 项目文档
├── tests/                    # Python 集成测试
├── docker/                   # Docker 构建配置
├── img/                      # 截图和图片资源
├── data/                     # SQLite 数据库文件（运行时生成）
├── docker-compose.sqlite.yml # Docker Compose（SQLite 模式）
└── docker-compose.yml        # Docker Compose（PostgreSQL + pgvector）
```

## 📚 文档

### 架构
- [项目总览](docs/reference/architecture/overview.md) — 架构与运行关系
- [前端架构](docs/reference/architecture/frontend.md) — Nuxt 4 前端结构
- [后端架构](docs/reference/architecture/backend.md) — Go 后端结构
- [数据流](docs/reference/architecture/data-flow.md) — 数据流转与处理流程

### 操作指南
- [快速上手](docs/getting-started.md) — 环境搭建与首次运行
- [配置说明](docs/reference/configuration.md) — 环境变量与配置项
- [开发指南](docs/reference/development.md) — 本地开发、构建、测试
- [测试指南](docs/reference/testing.md) — 测试框架与运行方式
- [部署指南](docs/reference/deployment.md) — 容器化部署与生产配置

### 功能说明
- [内容处理](docs/reference/content-processing.md) — Firecrawl 与 AI 增强流程
- [主题图谱](docs/v1.2-tag-intelligence/user-guide/topic-graph.md) — 主题图谱功能说明
- [阅读偏好](docs/reference/reading-preferences.md) — 偏好追踪与排序

### API
- [API 参考](docs/reference/api/_index.md) — 后端 API 接口文档
- [主题图谱 API](docs/reference/api/topic-graph.md) — 主题图谱接口说明

## 🤝 贡献

参见 [CONTRIBUTING.md](CONTRIBUTING.md) 了解贡献指南。

## License

[GNU General Public License v3.0](LICENSE)
