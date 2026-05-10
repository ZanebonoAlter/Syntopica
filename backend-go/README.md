# Backend Go

后端基于 Go、Gin、GORM 和 SQLite。

## 当前入口

- 服务入口：`backend-go/cmd/server/main.go`
- 路由装配：`backend-go/internal/app/router.go`
- 运行时装配：`backend-go/internal/app/runtime.go`
- 运行时共享状态：`backend-go/internal/app/runtimeinfo/schedulers.go`
- 配置文件：`backend-go/configs/config.yaml`
- 数据库逻辑：`backend-go/internal/platform/database/db.go`

## 开发命令

```bash
go mod tidy
go run cmd/server/main.go
go test ./...
go run cmd/migrate-digest/main.go
go run cmd/test-digest/main.go
```

## 架构文档

- 后端架构：`docs/reference/architecture/backend.md`
- 后端运行与接口：`docs/reference/architecture/runtime.md`
- 数据流：`docs/reference/architecture/data-flow.md`
- 数据库说明：`docs/reference/database/DATABASE_FIELDS.md`
- 开发流程：`docs/reference/development.md`

## 说明

- `docs/` 里的文档现在是正式维护入口
- `backend-go/ARCHITECTURE.md` 和 `backend-go/DATABASE.md` 适合当历史参考，不再当作现状真相
- 后端当前已经落到 `app / platform / domain / jobs` 四层
- 当前目录说明见 `docs/reference/architecture/backend.md`
