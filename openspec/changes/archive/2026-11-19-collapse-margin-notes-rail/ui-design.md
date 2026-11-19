<!-- ui-impact: minor -->
<!-- ui-approval: not-required -->
<!-- ui-prototype: none -->

# UI 设计（minor — 复用契约 + 状态契约）

沿用现有三栏阅读台、`drm-notes-rail` 面板与批注卡，不新增页面/弹窗/导航，不引入新样式体系。只改「第三列在不在」这一件事。

## 入口与入口变更

日报阅读台路由 `/tags/:boardId/:date`（`DailyReportReader.vue mode="desk"`）内，页边注栏的入口由「常驻第三列」改为两个显式入口：

| 入口 | 出现时机 | 触发结果 |
| --- | --- | --- |
| 右缘竖向浮标「页边注 · N」 | 桌面（≥1100px）且栏处于**收起态** | 展开为第三列（面板进场 180ms 淡入 + 右移归位），浮标消失 |
| 栏头收起按钮（`→`，朝栏外） | 桌面且栏处于**展开态** | 收回为两列，浮标出现 |
| 右下 FAB「页边注 N」 | 窄屏（<1100px）且有批注 | 打开右侧抽屉（**本次不变**） |

面板位置/宽度/内部交互与改造前完全一致，只是默认不在 DOM 中；不新增导航层、不加工具栏按钮、不改左目录与正文分区。

## 布局

| 状态 | 网格 | 右侧入口 |
| --- | --- | --- |
| 收起（桌面默认） | `14rem minmax(0,1fr)`（目录 + 正文，无第三列、无留白列） | 视口右边缘中部固定竖向标签：图标 + 竖排「页边注」+ 计数徽标 |
| 展开 | 现状 `14rem minmax(0,1fr) clamp(15rem,17vw,17rem)` | 栏头右侧收起按钮（`→`，朝栏外方向），浮标隐去 |
| 窄屏 <1100px | 单列（现状不变） | 右下 FAB + 右抽屉（现状不变，抽屉头不出现收起按钮） |

- 浮标不遮挡正文：贴右边缘（`right: 0`）、垂直居中，圆角朝左，宽度随「页边注」竖排自然收窄（约 2–2.6rem）；展开后立即消失，不给正文留占位。
- 浮标与既有 `.drm-notes-fab` 共享视觉语言（深底/强调色计数），但为边缘形态：`writing-mode: vertical-rl`、竖向字距 0.18em、`z-index: 30`。
- 浮标与窄屏 FAB 都是**视口固定**元素，因此渲染在 `.drm-peel-host`（带 `perspective`，会把 `position: fixed` 的包含块改成本身）之外，作为 `.drm-reader` 的直接子元素。

## 状态契约

| 维度 | 契约 |
| --- | --- |
| 默认态 | 无 `localStorage` 记录 = 收起。 |
| 记忆 | 仅**用户显式**切换（点浮标 / 点栏头收起）写 `syntopica.daily-report.margin-notes-collapsed`（`1`/`0`）。 |
| 程序性展开 | 新建批注落锚成功、点正文高亮、深链/管理页「跳原日报」定位 → 展开并定位卡，**不写偏好**（下次进入仍按用户上次显式选择）。 |
| 加载/失败/落锚失败 | 与现状一致；收起态不渲染面板，故这些态只在展开后可见（展开即发起原数据加载，不额外请求）。 |
| 空态 | 收起态：浮标计数 `0` + 可访问名/`title`「划选正文文字可添加批注」；展开态：原极轻灰提示不变。 |
| 删除全部批注 | 面板保持展开，回落到空态提示（不自动收起，避免操作中途面板消失）。 |

## Interaction Decisions

| ID | 决策 | 模式 | forbidden | 状态 |
| --- | --- | --- | --- | --- |
| D1 | 页边注栏的展开/收起入口 | `panel` | `dialog` | `pinned` |

- 「展开=与正文并列的分区」来自 2026-11-18 用户对话裁决（三选一里选了右缘浮标；工具栏按钮被否）。`dialog` 被禁：批注需要与正文并列阅读，弹窗会丢掉上下文。
- 窄屏的 `drawer` 是 `layout.md`「抽屉 + 单栏切换」的布局降级，不是本决策选择的交互模式，故不列入 forbidden。

## Data Provenance

| 数据项 | source | 说明 |
| --- | --- | --- |
| 浮标徽标 / 栏头的批注数 N | `api GET /daily-reports/:id/annotations` | 既有端点，`useMarginNotes` 拉取后取 `.length` |
| 收起/展开状态 | `派生 localStorage syntopica.daily-report.margin-notes-collapsed` | 前端派生，无后端字段 |
| 批注卡列表与问答轮 | `api GET /daily-reports/:id/annotations` | 既有渲染，本次不改 |

## 交互与可达性

- 浮标是 `button`：`aria-expanded="false"`、`aria-label` 与 `title` 同为「展开页边注（N 条批注）」/ 空态时「展开页边注 · 划选正文文字可添加批注」（计数与引导进可访问名，不只靠 hover）。
- 栏头收起按钮：`aria-label="收起页边注"`、`aria-expanded="true"`、`title="收起页边注"`。
- 键盘：两处入口均原生 Tab 可达 + Enter/Space 触发。
- 动效：面板进出 180ms 透明度 + 12px 右移；`prefers-reduced-motion` 下关闭该过渡与按钮过渡。
- 焦点：展开后不抢焦（用户可用 Tab 进入面板）；「新建批注/深链定位」路径按现状把焦点落到问题输入框或卡片。

## 验收

- 组件回归：`front/app/features/tags/components/DailyReportReader.test.ts`（默认收起两列 + 浮标；显式展开/收起双向切换并落盘偏好；`localStorage='0'` 时进入即三栏；程序性展开不写偏好；窄屏不出现桌面浮标/第三列）与 `front/app/features/tags/components/daily-report/MarginNotesRail.test.ts`（`canCollapse` 渲染收起按钮并上抛 `collapse`、`aria-expanded`；抽屉实例不渲染）。
- 浏览器验收（1440×900 与 900×800 双视口，证据见本 change `ui-verification/report.md`）：① 首屏浮标贴视口右缘中部（不随正文滚动）；② 点浮标 → 三栏且栏头有 `→`；③ 刷新后保持展开；④ 点 `→` → 两列且浮标回归；⑤ 900px 宽无浮标、无第三列；⑥ 暗色主题下浮标底色/文字/徽标可读。

## 不做的事

- 不做拖拽调宽、不做半展开（peek）形态、不改窄屏抽屉、不改批注卡内部布局、不加键盘快捷键。
