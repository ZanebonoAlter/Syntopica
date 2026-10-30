# UI 验收记录：fix-nested-dialog-layering

<!-- ui-impact: none -->
<!-- 档位说明：none 档不强制本文件；本记录为实测证据留档（机械断言 + 截图），非门禁要求 -->

### A-1 治理工作面内「创建并挂载」弹窗可见且压过宿主遮罩【pass】

依据：agent-browser 实机（静态发布实例 `http://localhost:5100/tags/1974` → 右侧「查看候选与依据」打开本板块上下文 / 治理工作面 → 「组合」页签 → 点「创建并挂载」）。机械断言：`elementFromPoint` 探测组合弹窗卡片中心 → 命中 `.cld-picker`（嵌套浮层本体，`hitInsideNested=true`）；宿主遮罩内联 z-index = 1000、嵌套浮层内联 z-index = 1100（嵌套 > 宿主）。同页 A/B 对照：把嵌套层级改回修复前的手写值 60 → 同一探测点命中 `.bhd-section`（宿主工作面内容，`hitInsideNested=false`），即「弹窗被盖住」的故障态复现。截图见同目录 `A-1-nested-dialog-visible.png`（按 `openspec/changes/.gitignore` 的 `**/ui-verification/**/*.png` 规则**不入库**，留本地存档；git 内可查的证据是本条判定文本与归档目录 `review.md` 的复现记录）。

### A-2 同弹窗在标签池页（页面级宿主）行为不变【pass】

依据：同一弹窗在 `/tags` 总览的标签池页宿主为页面（非弹窗），层级 1100 高于页面级 topbar 30 / bottombar 40 / 矩阵标记 1000，弹窗仍完整可见、无遮挡（组件层由 `CompositeLabelEditDialog.test.ts` 既有用例 + 本次层级用例覆盖；实机未逐页复核，判为 pass 依据是层级关系唯一确定）。
