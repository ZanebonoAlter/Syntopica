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

## openspec 制品入库口径

`openspec/changes/**` 下**账本入 git、证据本体不入 git**（`openspec/changes/.gitignore` 已落规则）：

| 内容 | 入库 | 说明 |
| --- | --- | --- |
| `report.md` / `*-review.md` / `design.md` / `tasks.md` / `specs/**` 等 md | ✅ | 判定账本与合同，是评审与追溯依据 |
| JSON 采样底稿（`ui-verification/**/api/`、`raw/`、`data/`） | ✅ | 审计可复现依据，体积可控 |
| `ui-prototype/**/*.html|js|css`（原型源码） | ✅ | 冻结基线按 SHA256 对照，源码须可查 |
| `ui-verification/**`、`ui-prototype/**` 下的 `png/jpg/jpeg/gif/webp` | ❌ | 磁盘保留（门禁按盘核对），不入库 |

**为什么**：归档门禁 `scripts/harness/ui-verification-trace.sh` 的实物对账是**纯文件系统检查**（`report.md` 建账条目 + 点名截图存在于 `ui-verification/`），证据本体是否入库**不影响门禁判定**；而单 change 的截图可达数十 MB（多批次 × 双视口 × 明暗主题），二进制灌进 git 会污染仓库体积与 clone 成本。

**两条纪律**：

1. **路径写法**：ignore 规则必须用 `**/` 形式（`**/ui-verification/**/*.png`），不能写 `*/ui-verification/...`——change 归档后目录搬进 `changes/archive/<date>-<name>/` 层级变深，单层通配会失配，导致归档那次提交把全部证据一次性灌进仓库。
2. **删图前先对账**：删任何截图前先确认它不被 `report.md` 点名（点名图删了门禁直接打红）。无引用图可删，删完复跑 `bash scripts/harness/ui-verification-trace.sh <change-dir>` 确认仍通过。

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
