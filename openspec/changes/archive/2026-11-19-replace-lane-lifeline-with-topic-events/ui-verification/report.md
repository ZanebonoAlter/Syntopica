# UI 验收 · replace-lane-lifeline-with-topic-events

> 对照已批准基线 `ui-design.md`「Acceptance」A-1～A-9。
> 环境：静态托管 `http://localhost:5100`（`deploy-frontend.sh` 产物），真实数据 `board=1974`、`report_date=2026-11-18`（report id 1537）。
> 工具：`agent-browser`（Chromium headless），主题 editorial / dark，正文列宽由阅读台三栏决定（1440 视口下正文列 ≈900px，正是旧折线崩坏的区间）。

### A-1 1440×900 展开话题卡片【pass】
- 依据：`A1-1440x900.png`。「关心的话题」分区标题行右侧是分区级 `7 天 / 30 天` 工具条；分区下 4 条当日线索，其中 3 条渲染 chip 行（每条 3 chip + 「还有 N 条」），第 4 条无候选、行内无任何新增节点（**档位控件不在线索行内**：`.tre .tre__day` 计数为 0）。
- 机械断言：每个 `.tre` 的 `scrollWidth === clientWidth`（621/621），无线索行横向溢出；`[data-testid^=zone-related-days]` 共 2 条（active / 其他动态各一条）。

### A-2 1920×1080 展开话题卡片【pass】
- 依据：`agent-browser viewport 1920 1080` 后同一断言：3 行 chip、4 条线索；每个 `.tre` 的 `scrollWidth === clientWidth`（621/621）。
- 说明：正文列宽由三栏决定，1920 下仍为 621px 的 chip 行容器，未随视口无限拉宽。

### A-3 正文列 ~900px（原崩坏区间）【pass】
- 依据：`A1-1440x900.png`。该视口下正文列 ≈900px（旧 `.drm-history` 在此区间被挤成一字一行）。
- 断言：chip 单行 ellipsis、多 chip 自然换行；截图中线索正文行宽 ≥ 20 汉字，无逐字竖排。

### A-4 悬停 chip 出浮层【pass】
- 依据：`A4-tooltip.png`。浮层含四段：候选线索标题 → 摘要（多行截断）→ 「来源：<文章标题>」→ 「强关联 0.91」+ 进度条 + 日期 `2026-11-16`。
- 机械断言：`.tre-tip` `getBoundingClientRect` = 384×167，`top>=0 && left>=0 && bottom<=innerHeight && right<=innerWidth` → **完全在视口内**（边界翻转生效，未被线索行 `overflow` 裁剪）。

### A-5 点击 chip 打开站内阅读预览【pass】
- 依据：`A5-preview-dialog.png`（弹窗 + 上下文行）、`A5-chip-row.png`（线索行 chip 形态）。点击首个 chip 后打开既有「文章预览」弹窗，内容为对应来源文章。
- 机械断言：`[role=dialog]` 存在；`[data-testid=preview-related-context]` 文本为 `相关线索 · 强关联 0.90 · 2026-11-16`；`location.pathname` 保持 `/tags/1974/2026-11-18`。
- 兼容性：普通文章行（非 chip 入口）不传上下文，该行不渲染（`ArticlePreviewModal` 的 `relatedContext` 默认 `null`）。

### A-6 无相关线索的线索行不渲染 chip 行【pass】
- 依据：`.drm-thread` 计数 4，`.tre` 计数 3 → 恰有 1 条线索无候选且**未渲染任何节点**（无占位、无空态文案、无 `min-height`）。

### A-7 7 ↔ 30 档位切换与缓存【pass】
- 依据：`A7-days30.png`。在**分区级工具条**切到 30 天后：`thread-relations?days=30` 返回，折叠计数由「还有 7 条」变为「还有 30 条 / 32 条」，chip 行仍为 3 行（**无候选的线索行不会因非默认档位留下空行**）。
- 分区独立：切档后 `[data-testid=zone-related-days-30]` 的 `aria-pressed` 为 `["true","false"]`——active 分区切了、其他动态分区仍保持 7 天（各分区各一份 `relatedDays`）。
- 缓存：切回 7 天命中缓存不发请求；失败档再切档走 force 重发（作为重试入口）。

### A-8 双主题【pass】
- 依据：`A8-dark.png`（dark）与 `A1-1440x900.png`（editorial）。chip 文字、★ 星标、浮层标题/摘要/来源/分数在两种主题下均可读；chip 文字使用 `--color-text-secondary`（editorial ≈6.3:1 / dark ≈6.5:1，均 ≥4.5:1）。

### A-9 与已批准原型逐交互对账【pass】
- 依据：`ui-design.md`「Interaction Contract」6 条主操作逐条对账，无多无少：
  1. 展开话题卡片 → chip 行随线索出现（截图 A-1）✅
  2. 悬停 / Tab 聚焦 → 浮层四段（截图 A-4）✅
  3. 点击 chip → 阅读预览弹窗（截图 A-5）✅
  4. Esc / 点遮罩 → 关闭并回到原位（人工：Esc 关闭弹窗后滚动位置未变）✅
  5. 7/30 档位切换（截图 A-7）✅
  6. 「还有 N 条」就地展开（DOM 中 `.tre__chip--more` 点击后补出剩余 chip）✅
- 「禁止项」5 条无越界：chip 只有标题无分数；无相关不渲染（A-6）；无颜色编码；不整行可点；浮层不承载必读信息。

## 实现期发现并修复的问题

| 问题 | 现象 | 修复 |
| --- | --- | --- |
| 切到 30 天档后整行（含档位控件）消失 | 首次 30 天请求因环境负载超时 → entry 置 `error` → 整行按「无数据不渲染」消失，档位控件随之消失，用户被卡在 30 天档无法切回 7 天 | `showRow` 增加 `props.days !== DEFAULT_DAYS`：非默认档位即使无数据也保留该行与档位控件；默认档位无候选时仍整行不渲染。补测试 `30 天档请求失败仍保留档位控件` |
| 切档瞬间档位控件闪没 | 切到未缓存档位时 cache 新键无 data → `items` 空 → 整行消失约 200ms | 增加 loading 分支：2 个静态骨架 chip（无循环脉冲）+ `aria-busy`，档位控件常驻 |
| 全量构建 OOM | `pnpm build` 在树莓派上 Node 堆溢出 | 用 `NODE_OPTIONS=--max-old-space-size=5632` 构建（部署脚本沿用此环境变量） |
| 30 天档下每条无候选线索都留一个只有档位控件的空行（review R-3） | 修「卡在 30 天」时的 `showRow` 分支引入 | 档位控件上提到**分区级工具条**，`showRow` 恢复严格条件，任何档位下无候选都不渲染 |
| Esc 同时关浮层与阅读台（review R-4） | 浮层未登记 `overlayStack`，document 级 Esc 先于 window 执行 | 浮层 `push/popOverlay` 成对记账，Esc 只关浮层；实测 `tipBefore=true → tipAfter=false` 且仍停在阅读台 |
| 浮层分数四舍五入跨档（review R-8） | `0.8499` → 显示 `0.85` 却标「相关」 | 新增 `formatRelationScore` 向零截断到两位（0.8499→0.84），浮层与弹窗共用 |

## 未覆盖（含原因）

- **375×667 / 390×844 窄屏档**：本 change 不触及阅读台窄屏降级路径（三栏 → 单栏切换是既有能力，chip 行按容器宽自然换行、不新增断点逻辑），故未单独跑窄屏视口；A-3 的 900px 正文列已覆盖本 change 的原始崩坏区间。
- **全系统路由矩阵**：本 change 不改共享壳与导航（仅阅读台内一个内联区块），按 `layout.md`「UI 复审的全系统与产品任务覆盖」的触发条件（改动 `/tags` 及其子路由或全局导航）——**本次改动的正是 `/tags/:boardId/:date` 页面**，故补记人工走查：从矩阵进入阅读台 → 展开话题 → 点 chip → 读原文 → Esc 返回，路径通；矩阵侧泳道与侦探墙入口未受影响（侦探墙文件保留，日报侧挂载已随折线退役）。
