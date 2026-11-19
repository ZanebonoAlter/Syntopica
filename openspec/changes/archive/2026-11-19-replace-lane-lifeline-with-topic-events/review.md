# Review 台账 · replace-lane-lifeline-with-topic-events

> 审查者：`change-review` 子线程（只读），2026-11-17。审查范围：proposal / design D1–D8 / 两份 spec delta / ui-design / tasks / test-cases / explore-findings + 前后端实现与测试。
> 处置人：主线程 + 实现子线程。状态词取值：待处理 / 已修复 / 已采纳 / 已驳回 / 误报丢弃。

### R-1 2026-11-17【High】点击 chip 的阅读预览弹窗没有关联度上下文
- **状态**：已修复
- 问题：spec 要求弹窗内显示相关度，实现只 `emit('openArticle', articleId)`。
- 处置：`ThreadRelatedEvents` 改为 emit `{ articleId, score, level, periodDate }`；`DailyReportTopicSection` → `DailyReportReader` → `DailyReportDeskShell` 透传并存 `previewContext`；`ArticlePreviewModal` 新增可选 `relatedContext`，在标题下渲染 `相关线索 · 强关联 0.89 · 2026-11-02`。**数字入参保持向后兼容**（普通文章行/页边注引用 chip 行为逐字不变，不传上下文则不渲染该行）。
- 验证：`ArticlePreviewModal.test.ts` 新增 3 例（strong 文案 / 0.8499→0.84 不跨档 / 不传不渲染）；`DailyReportDeskShell.test.ts` 新增 2 例（数字入参 context 为 null、对象入参透传且关闭后清空）。

### R-2 2026-11-17【High】「完整生命线」替代入口不存在，`TopicDetectiveWall` 成零消费者死代码
- **状态**：已采纳（承认现状，改口径而非补入口）
- 问题：proposal/spec 宣称「完整生命线从版块态势页泳道进入」，但删掉日报侧挂载后全仓无任何组件挂载侦探墙，`TopicDetectiveWall.client.vue` 零引用。
- 处置：**不补入口、不删组件**。把 spec 的「完整话题生命线」Scenario 改为如实表述：本 change 后完整生命线**暂无界面入口**，该缺口登记为已知限制，MUST NOT 渲染不可达入口，也 MUST NOT 在文档里宣称可进入；`docs/reference/flow/daily-report.md` 同步。`TopicDetectiveWall` 与 `detective-wall/` 的去留（补入口 or 彻底删除 ~900 行 3D 代码）另行决策，不在本 change 顺手处理——避免把「删功能」混进 UI 形态替换。
- 待办承接：需要新 change 决定侦探墙命运（建议名 `retire-or-revive-detective-wall`）。

### R-3 2026-11-17【Medium】非默认档位下无候选/失败也渲染整行，只剩裸档位控件
- **状态**：已修复
- 问题：为修「卡在 30 天档无法切回」引入的 `showRow = … || props.days !== DEFAULT_DAYS`，导致 30 天档下每条无候选线索都留一个只有档位控件的空行，与 spec「无候选整行不渲染」冲突。
- 处置：**档位控件上提到分区级工具条**（`DailyReportTopicSection` 的 zone header 右侧，每个内容分区一条），`ThreadRelatedEvents` 删除档位 UI 与 `props.days`，`showRow` 恢复 `loading || items.length > 0`。切档失败后再切档 = 显式重试（error 条目 force 重发，成功条目命中缓存）。
- 验证：`DailyReportTopicSection.test.ts` 新增 4 例（分区只有一条档位控件且在线索行之外、切档取数、切回命中缓存、失败档再切强制重发）；`ThreadRelatedEvents.test.ts` 删档位相关 3 例、新增「不渲染档位控件」。
- 制品同步：spec（分区级工具条 + 无候选任何档位都不渲染）、ui-design（Interaction Contract 5 / State Matrix / Tab 顺序）、flow 红线 57 均已改。

### R-4 2026-11-17【Medium】Esc 同时关浮层与阅读台，键盘用户被踢出
- **状态**：已修复
- 问题：浮层未登记 `overlayStack`，`document` 级 Esc 先于 `window` 执行，一击键既隐浮层又 `closeReader()`。
- 处置：浮层 `show()` 时 `pushOverlay()`、`hide()`/卸载时 `popOverlay()`（组件内布尔守卫保证成对；`overlayStack` 是无参计数器，未改共享模块签名）。测试用 `bubbles: true` 复刻 document→window 真实冒泡顺序，断言浮层消失且宿主未 close、第二次 Esc 才轮到阅读台。

### R-5 2026-11-17【Medium】test-cases 的 loading 结论已被推翻
- **状态**：已修复
- 处置：变体 15 改为「2 个静态骨架 chip（`aria-busy`），行高固定不跳动」；删除「与 ui-design 状态矩阵的差异」旧留痕（已按 ui-design 收敛）。

### R-6 2026-11-17【Medium】tasks 的 Scenario→测试映射有不实条目
- **状态**：已修复
- 处置：「进入完整话题生命线」映射从 `DailyReportDeskShell.test.ts`（该文件 11-11 后未改、无相关断言）改到 `DailyReportEvents.layout.test.ts`（有「日报侧无折线/节点图」的结构断言）；R-2 处置后该 Scenario 改为「不出现不可达入口」。

### R-7 2026-11-17【Low】tasks 9.4 命令按原文执行必然失败
- **状态**：已修复
- 处置：命令改为相对 `front/` 的 `app/...` 路径（vitest include 是 `app/**`），并在 tasks 里写明「不能再带 `front/` 前缀」。

### R-8 2026-11-17【Low】浮层分数四舍五入跨档（0.8499 显示成 0.85 却标「相关」）
- **状态**：已修复
- 处置：新增 `app/utils/threadRelation.ts#formatRelationScore`，**向零截断到两位**（`Math.trunc(score*100 + 1e-9)/100`），0.8499→0.84；浮层与弹窗上下文行共用，档位词也统一到 `relationLevelLabel`。

### R-9 2026-11-17【Low】spec 三态优先级未写明
- **状态**：已修复
- 处置：spec 补「优先级 `empty` → `partial` → `ready`：`total == 0` 恒为 `empty`（即便有跳过）」，并新增 Scenario「全部线索缺向量时 empty 优先」。

### R-10 2026-11-17【Low】档位是「每分区一份」，文档写「整份日报口径」
- **状态**：已修复
- 处置：文档改为「**分区级工具条**：`relatedDays` 在 `DailyReportTopicSection` 持有，三个内容分区各一份、互不影响」；spec 同步。

### R-11 2026-11-17【Low】死代码与过期注释
- **状态**：已修复
- 处置：`useThreadRelatedEvents` 删除 `retry`（由 `ensure(..., force)` 覆盖），`resetLocal` 接到 `reportId` 变化（换报告/版块清缓存并重拉，注释成真）；`useDailyReportReader#ensureHistoricalDetail`（唯一调用方 mini 生命线已删）删除导出+注释，其回归价值改挂 `ensureDetail` 的 `detailLoading` 用例。

### R-12 2026-11-17【Low】`TIP_Z_INDEX = readingDialog - 1` 绕过档位表
- **状态**：已修复
- 处置：`Z_INDEX` 新增 `readingTooltip: 9099`（注释：高于阅读台遮罩 `.drm-overlay` 9000、低于阅读弹窗 9100），组件引用常量；`layout-contract.test.ts` 补数值锚与「行内浮层低于阅读弹窗」关系锚；`standard/frontend/layout.md` 档位表补该行。

### R-13 2026-11-17【Low】浮层随滚动/缩放脱位
- **状态**：已修复
- 处置：浮层打开期间挂 `window` 的 `scroll`（capture）+ `resize`，触发即 `hide()`（不追位、不抖）；隐藏/卸载解绑。

### R-14 2026-11-17【Low】「还有 N 条」缺 `aria-expanded`
- **状态**：已修复
- 处置：展开器补 `:aria-expanded`；测试断言。

## 审毕无发现（审查者确认项）

- 契约一致性：`score` = 余弦（含不使用 `1-(1-s)` 往返的 ulp 说明）、阈值 0.75 / 档位 0.85、三级排序、返回全量不截断、`days` 只认 7/30 并回显、窗口不含当期——逐条与 spec/design 对齐，测试覆盖 B1–B15。
- 状态语义：`threads` 恒为 `{}`、`skipped_no_embedding` 只计当期缺向量。
- confirmed 只读：全文件无写操作；星标只认 `category='event'`。
- 前端形态红线：chip 只写标题、`position: fixed` + Teleport、无 `source_article` 不 emit、无占位/空态文案。
- 移除面：`MiniLifeline` / `buildLifelineWindow` / `buildBezierPath` / `TopicLifelineData` / `ensureLifeline` / `selectedDayByTopic` / `#details` / `openDetective` / `loadHistorical` 全部零残留；`getTopicLifeline` 与后端 lifeline 端点按要求保留。

## 工作树边界说明

审查快照（23:59）期间本 change 仍在变动（`ThreadRelatedEvents.vue` 23:55 / 测试 23:56，即 R-3 引入的行为），结论以该快照为准；上述处置均已在新一轮实现后重新验证。工作树同时混有其它 change 的未提交改动（`main.css` / `scroll-thin` 迁移 / `theming.md` 等），不属于本 change。
