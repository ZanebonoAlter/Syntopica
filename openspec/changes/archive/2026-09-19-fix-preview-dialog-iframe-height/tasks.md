## 1. 复现测试（用例先行，§2 不可豁免）

- [x] 1.1 新增 `front/app/features/tags/components/ArticlePreviewModal.test.ts`：断言修复前失败的契约——选中文章时渲染 `.preview-body` 且携带显式高度声明（内联 height 样式）、`ArticleContentView` 渲染于其内、弹窗经 AppDialog 宿主打开。验证：`cd front && pnpm test:unit ArticlePreviewModal --maxWorkers=2` 修复前红、修复后绿

## 2. 修复实现

- [x] 2.1 `ArticlePreviewModal.vue`：`.preview-body` 从无效的 `flex: 1` 改为内联确定高度绑定 `height: calc(85vh - 110px)`（85vh 上限 − header ≈61px − body padding 40px，注释推导式），scoped 样式块同步清理失效声明。验证：1.1 测试转绿；人工打开日报文章预览切「内嵌网页」iframe 撑满

## 3. 测试（§11 固定尾节）

- [x] 3.1 影响范围测试：`cd front && pnpm test:unit ArticlePreviewModal --maxWorkers=2` 全绿
- [x] 3.2 lint / typecheck / build：`cd front && pnpm lint && pnpm exec nuxi typecheck && pnpm build` 全部通过（lint 0 error，存量 warning 零条落本 change 文件；typecheck exit 0；build ✨ complete）

## 4. 文档（doc-impact: flow）

- [x] 4.1 `docs/reference/flow/reading.md` 变更溯源表补一行（预览弹窗高度修复，链接本 change 归档路径）；核对业务约束节（含 CSS 作用域红线）与本修复无冲突——本改动只动弹窗宿主 scoped 作用域，不触碰 `ArticleContent.css`。验证：表格新行含 change 链接、约束节无冲突

## 5. 验证（§11 固定尾节，每条 = 命令 + 期望结果）

### Scenario → 测试映射（与 delta spec 逐条对账）

| Scenario | 测试文件 |
| --- | --- |
| 切换到内嵌网页模式 | 人工：日报文章预览切内嵌网页，iframe 撑满内容区（截图/实机验证记录于本节） |
| 文本预览模式内部滚动 | 人工：长文预览滚动发生在内容区内、工具栏常驻、无双滚动条 |
| 高度契约结构锚点 | front/app/features/tags/components/ArticlePreviewModal.test.ts |

- [x] 5.1 `cd front && pnpm test:unit ArticlePreviewModal --maxWorkers=2` → 3 passed（含修复前红→绿复现）
- [x] 5.2 `cd front && pnpm lint && pnpm exec nuxi typecheck && pnpm build` → 全部通过
- [x] 5.3 `bash scripts/dev/deploy-frontend.sh` 铺静态产物，实机验证两条人工 Scenario（2026-09-19，agent-browser，板块「中东地缘政治与美伊关系」日报 9月19日期 → 话题 01 泳道首篇文章预览）：①内嵌网页——iframe 363px 撑满 .iframe-mode（previewBody 428px = 85vh(538)−110 推导式精确命中，弹窗总高 531 ≤ 85vh），截图 `acceptance/iframe-mode-filled.png`；②文本预览——.app-dialog__body 不滚（无双滚动条）、.preview-mode 内部滚动、工具栏常驻。期望：两条 Scenario 全过 ✓
- [x] 5.4 `openspec validate fix-preview-dialog-iframe-height --type change` → 通过

<!-- doc-impact: flow -->
