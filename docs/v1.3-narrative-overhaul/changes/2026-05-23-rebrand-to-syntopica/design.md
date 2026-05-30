## Context

项目目前命名不统一：产品名 "RSS Reader"、仓库名 "my-robot"、Go 模块 "my-robot-backend"、NPM 包 "front"、Docker 容器 "zanebono-rssreader-pgvector"。需统一为 "Syntopica"，同时保留 GitHub org "ZanebonoAlter" 不变。

此次变更是纯品牌/命名变更，不改动任何功能逻辑、API 接口、数据模型。

## Goals / Non-Goals

**Goals:**
- 产品名、仓库名、模块名、包名、Docker 命名全面统一为 Syntopica
- README、AGENTS.md、文档中项目引用更新
- 定义 tagline 和品牌一句话描述
- 物理目录结构（`front/`、`backend-go/`）不重命名，降低 diff 噪音

**Non-Goals:**
- 不改动 API 路由（仍为 `/api/...`）
- 不改动数据库表结构
- 不改动外部服务集成（crawl service 等）
- 不改动 Go 代码中的包结构或内部 import 路径
- 不涉及功能特性变更

## Decisions

| # | Decision | Rationale | Alternatives Considered |
|---|----------|-----------|------------------------|
| 1 | 物理目录 `front/`、`backend-go/` 不重命名 | 改名成本高（git history 断裂、CI 配置需改、所有开发者 remote 需更新），且对用户不可见 | 全量重命名目录（否决：收益低、成本高） |
| 2 | Go module 名改为 `syntopica-backend` | go.mod 的 module 名是项目标识的一部分，应在 README 和代码中一致 | 保持 `my-robot-backend`（否决：继续混乱） |
| 3 | NPM 包名改为 `@syntopica/web` | scoped package 表明是 Syntopica 生态的一部分 | `syntopica-front`（否决：不够清晰） |
| 4 | Docker 服务/容器使用 `syntopica-*` 前缀 | docker-compose.yml 中的服务名需要统一 | 保持 `zanebono-rssreader-*`（否决：遗留品牌） |
| 5 | 数据库名改为 `syntopica` | docker-compose 中的默认数据库名 | `syntopica_db`（否决：无必要冗余） |
| 6 | 品牌描述: "Where feeds become topics" | 简洁、有记忆点、准确描述产品价值 | 多个备选（见 proposal） |

## Risks / Trade-offs

| Risk | Mitigation |
|------|------------|
| GitHub 仓库改名为 URL 301 跳转，但本地 remote 需所有开发者手动更新 | 改名后发通知，README 加醒目提示 |
| Go module 改名后 `go.mod` 变更导致 `go mod tidy` 可能产生残留 | 改名后执行 `go mod tidy` 并确认无 stale reference |
| NPM 包重命名后 lockfile 变更 | 重新 `pnpm install` 生成新 lockfile |
| crawl service 等外部服务配置引用旧名 | proposal 阶段已确认无公开 API 消费者 |
