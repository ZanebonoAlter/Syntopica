# 贡献指南

## 文档索引

| 你想做什么 | 参考文档 |
|----------|----------|
| 搭建开发环境 | [docs/getting-started.md](docs/getting-started.md) |
| 了解开发规范、构建、测试 | [docs/reference/development.md](docs/reference/development.md) |
| 了解系统架构 | [docs/reference/architecture/overview.md](docs/reference/architecture/overview.md) |

## 提交前检查

- **前端**: `cd front && pnpm exec nuxi typecheck && pnpm test:unit && pnpm build`
- **后端**: `cd backend-go && go test ./... && go build ./...`

## 许可证

[GNU General Public License v3.0](LICENSE)
