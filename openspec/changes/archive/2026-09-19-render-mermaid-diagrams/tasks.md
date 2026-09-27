<!-- doc-impact: flow -->

# Tasks — render-mermaid-diagrams

## 1. 渲染内核：useMermaidRender composable（design D1–D4）

- [x] 1.1 新建 `front/app/composables/useMermaidRender.ts`：动态 `import('mermaid')` 模块级单例 + `initialize({ startOnLoad: false, securityLevel: 'strict', useMaxWidth: true })` 一次性执行；导出 `useMermaidRender(hostRef, source)`。验证：`cd front && pnpm test:unit useMermaidRender --maxWorkers=2` 全绿
- [x] 1.2 扫描与替换逻辑：watch(source) + `nextTick` 后扫描 `pre > code.language-mermaid`，逐块 `mermaid.render` 替换为 SVG（幂等：已渲染跳过），按宿主顺序生成图题「图 N ·」写入 `.fig-cap`。验证：单测覆盖——单块成图、多块按序编号、v-html 重渲染后 SVG 恢复
- [x] 1.3 失败降级：逐块 try/catch，失败块保留源码 `<pre>` + `data-mermaid-failed` 标记 + 一行 warning 色提示（含原因缩略），单块失败不中断其他块；chunk 加载失败整体降级。验证：单测覆盖——非法语法块降级、混合块（合法+非法）互不影响、import 拒绝时全降级
- [x] 1.5 裸围栏识别（apply 实锤 #131733：firecrawl 丢语言标记 → 无类名 `code`）：无语言标注且首词为 mermaid 图类型的代码块按 mermaid 处理；带其它语言标注的块不猜。验证：`cd front && pnpm test:unit useMermaidRender --maxWorkers=2` 全绿（新增裸围栏正/反用例）
- [x] 1.4 主题适配与重绘：`MutationObserver` 观察 `documentElement` 的 `data-theme`（editorial→`default`，dark→`dark`），切换时对已渲染块重绘，版本 token 防竞态；组件卸载时断开观察者。验证：单测覆盖——主题切换重绘调用、快速连点仅最后一次生效

## 2. 宿主挂接（design D1）

- [x] 2.1 阅读页：`ArticleContentPreviewPanel.vue` 正文容器（`.markdown-article`）与 AI 整理稿容器（`.markdown-summary`）各接 `useMermaidRender`（source 分别为 `displayContent` / `renderedStoredSummary`）。验证：`cd front && pnpm test:unit ArticleContentPreviewPanel --maxWorkers=2` 全绿（新增：含 mermaid 块的文章渲染出 `.mermaid-block svg`）
- [x] 2.2 三面板：`QAPanel.vue` / `CausalAnalysisReport.vue` / `BoardEnrichmentPanel.vue` 的 markdown 宿主各接 composable（apply 修正：CandidateEditDialog 实测无 markdown 块级渲染，proposal/spec 已同步去除）。验证：`cd front && pnpm test:unit QAPanel BoardEnrichmentPanel --maxWorkers=2` 全绿（新增宿主各 1 条成图断言）

## 3. 容器样式：方案 B（design D5–D6）

- [x] 3.1 `ArticleContent.css` 基础段纯增量新增 `.mermaid-block`（上下 hairline `--color-border-subtle`、`padding`、`overflow-x: auto`、`text-align: center`）与 `.mermaid-block .fig-cap`（小号 `--color-text-muted`）；不改动任何既有 `.markdown-body` 规则。验证：`grep -n 'markdown-body' front/app/components/article/ArticleContent.css` 既有规则 diff 为零（仅尾部追加），双主题人工核对
- [x] 3.2 双视口视觉检查：阅读页含 mermaid 的文章在 1440×900 / 1920×1080 下图不破 840px 列版式，超宽图横向滚动可用（agent-browser 截图存档）。验证：截图证据附任务完成说明，与批准原型方案 B 一致

## 4. 测试（§11 固定尾节）

- [x] 4.1 影响范围测试：`bash scripts/harness/change-scope.sh` 判定命令，仅跑受影响文件（`useMermaidRender` / `ArticleContentPreviewPanel` / `QAPanel` / `BoardEnrichmentPanel` 相关单测，`--maxWorkers=2`）。验证：`cd front && pnpm test:unit useMermaidRender ArticleContentPreviewPanel QAPanel BoardEnrichmentPanel --maxWorkers=2` 全绿
- [x] 4.2 lint / typecheck / build：`cd front && pnpm lint && pnpm exec nuxi typecheck && pnpm build` 全部通过；构建产物含独立 mermaid chunk（`ls .output/public/_nuxt/ | grep -i mermaid` 或 chunk 体积核对）。验证：build 成功且 mermaid 不在首屏入口 bundle 内

## 5. 文档（doc-impact: flow）

- [x] 5.1 `docs/reference/flow/reading.md` 变更溯源表补一行（本 change：mermaid 图客户端渲染 + 方案 B 容器，链接本 change 归档路径）。验证：表格新行含 change 链接
- [x] 5.2 `docs/reference/flow/reading.md` 业务约束节核对：约束 7 阅读列版式条款下确认「去卡片化不含 .mermaid-block 轻容器（hairline+图题）」表述不冲突，如需一句澄清则补（不新增约束条目）。验证：人工核对约束节无新旧冲突

## 6. 验证（§11 固定尾节，每条 = 命令 + 期望结果）

### Scenario → 测试映射（与 delta spec 逐条对账）

| Scenario | 测试文件 |
| --- | --- |
| 合法 mermaid 块变为图 | front/app/composables/useMermaidRender.test.ts |
| 无 mermaid 块零行为 | front/app/composables/useMermaidRender.test.ts |
| 裸围栏 mermaid 块识别 | front/app/composables/useMermaidRender.test.ts |
| 图语法无效 | front/app/composables/useMermaidRender.test.ts |
| 渲染资源加载失败 | front/app/composables/useMermaidRender.offline.test.ts |
| 主题切换重绘 | front/app/composables/useMermaidRender.test.ts |
| 面板宿主同规则渲染 | front/app/features/tags/components/QAPanel.test.ts |
| 常规图在阅读列内呈现 | front/app/features/articles/components/ArticleContentPreviewPanel.test.ts |
| 超宽图横向滚动兜底 | 人工：实机 DOM 断言 .mermaid-block overflow-x=auto + svg max-width:100%（2026-09-19 agent-browser eval 通过；截图见 acceptance/） |

- [x] 6.1 `cd front && pnpm test:unit useMermaidRender ArticleContentPreviewPanel QAPanel BoardEnrichmentPanel --maxWorkers=2` → 全部通过
- [x] 6.2 `cd front && pnpm lint && pnpm exec nuxi typecheck && pnpm build` → 全部通过
- [x] 6.3 `bash scripts/dev/deploy-frontend.sh` 铺静态产物走 :5100 验收（不走 dev server），agent-browser（opencli 主交互链路）打开含合法 mermaid 的存量文章：打开文章 → 列表点选 → 正文渲染。**验收证据（2026-09-19，文章 #131733）**：实机 DOM 断言 `{mermaidBlock:1, svg:1, cap:"图 1", remainingPre:0, hint:0}`；切 dark 主题重绘通过；非法语法/资源加载失败降级由 useMermaidRender(.offline).test.ts 机写覆盖。期望：断言全过 ✓
- [x] 6.4 `openspec validate render-mermaid-diagrams --type change` → 通过（deltas 完整、无零 delta 报错）
- [x] 6.5 **UI 双视口验收证据（major，ui-approval: approved）**：1440×900 与 1920×1080 两档、editorial/dark 双主题截图存 `acceptance/`（mermaid-{1440,1920}-{editorial,dark}.png）；实机断言图不破 840px 列版式；与批准原型差异见 ui-design.md §8 差异说明
