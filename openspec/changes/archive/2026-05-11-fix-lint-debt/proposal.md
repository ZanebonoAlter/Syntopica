## Why

当前代码库存在 161 个后端 lint 告警 + 33 个前端 eslint warning，全部在质量门禁路径上但从未被清零。随着代码量增长，债务持续累积，增加了后续开发踩坑的概率，也让 lint 输出失去信号价值。

## What Changes

- 全量执行 `gofmt -w ./...`，消除 ~120 文件的格式偏差和 5 个 gofmt lint 告警
- 删除 49 个 unused 函数/变量/常量/类型（包括 `extractor_enhanced.go` 中 12 个未接入的 AI 消歧函数）
- 修复 17 个 gosec 告警（G118 context 泄漏、G115 整数溢出、G306 文件权限、G501/G401 md5 弱哈希）
- 修复 31 个 errcheck 告警（补全 `.Close()`/`json.Unmarshal` 等返回值检查，`logging.go` 加 nolint）
- 修复 36 个 staticcheck 告警（nil context → context.TODO、Sprintf 简化、deprecated API 替换等）
- 修复 18 个 gocritic 告警（if-else → switch、appendAssign、unlambda、assignOp）
- 修复 5 个 ineffassign 告警（无效赋值修正）
- 修复前端 33 个 eslint warning（28 个 `no-explicit-any` 补类型、2 个 `no-unused-vars` 清理）

## Capabilities

### New Capabilities

（无）

### Modified Capabilities

（无 — 纯代码质量修复，不改变任何 spec 级别的行为需求）

## Impact

- **后端**：`backend-go/` 下约 50+ 源文件和 10+ 测试文件有改动，全部为删除/替换，无新增逻辑
- **前端**：`front/app/` 下约 12 个 `.ts` 文件有改动，补类型定义和删未用导入
- **无 API 变更**、无数据模型变更、无配置变更
- **风险极低**：所有改动都是静态分析可验证的机械性修复
