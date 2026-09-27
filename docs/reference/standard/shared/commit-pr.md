# 提交前检查与 PR 流程（Shared）

> **权威源**：本文件是提交前自检命令、分支/PR 约定的唯一权威。
> 门禁的完整定义（哪些命令、什么顺序）见《开发执行规范》§4.1（后端）/ §5.1（前端）；本文件聚焦"怎么跑"。

## 提交前检查

### 前端改动

```bash
pnpm lint                      # ESLint（全平台可用）
pnpm build                     # 生产构建
pnpm exec nuxi typecheck       # TypeScript 类型检查
pnpm test:unit                 # 单元测试
```

### 后端改动

```bash
golangci-lint run ./...        # 综合静态分析
go vet ./...                   # Go 官方静态检查
go build ./...                 # 编译检查
go test ./internal/reader/service -v   # 针对范围的单元测试
go test ./...                  # 全量测试
```

### 文档改动

改动涉及功能/接口/结构时，同步更新对应文档：

- `docs/reference/architecture/*.md`
- `docs/reference/flow/*.md`
- `docs/reference/database/*.md`

## ⚠️ 跨平台执行注意

前端 `node_modules` 按宿主平台安装，执行方式随之不同：

- **Linux / macOS（含树莓派）**：本机直接跑上表全部命令。
- **Windows + WSL**：`pnpm lint` 可在 WSL 跑；**typecheck / build / test:unit 必须经 Windows cmd**（WSL 缺 Linux native binding，如 `@oxc-parser/binding-linux-x64-gnu` 会失败）：

```bash
cmd.exe /C "cd /d D:\project\Syntopica\front && pnpm exec nuxi typecheck"
cmd.exe /C "cd /d D:\project\Syntopica\front && pnpm build"
```

## Branch 规范

- 主分支为 `main`
- 项目无文档化的分支命名规范

## PR 流程

- 项目无预配置 PR 模板（无 `.github/PULL_REQUEST_TEMPLATE.md`）
- 提交 PR 时确保：
  - 前端改动通过 `pnpm build` 或 `pnpm exec nuxi typecheck`
  - 后端改动通过 `go build ./...` 和对应单元测试
  - 文档与代码变更保持同步
  - PR 描述说明改动范围和原因

## 编码注意事项

- 前端源码必须 **UTF-8** 编码；PowerShell 写文件要显式保持 UTF-8
- 构建报 Vue/Vite 编码错误时，先检查文件编码，别先怀疑业务逻辑
- 后端 handler 返回 `gin.H{"success": bool, "data"|"error"|"message": ...}`
- 不要新增 linter / formatter / 其他工具，除非明确要求（已配置 golangci-lint 和 ESLint）

## 资料来源

收敛自原 `development.md` §提交前检查 / §Branch 规范与 PR 流程 / §编码注意事项。
