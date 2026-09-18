<!-- ui-impact: major -->
<!-- ui-approval: approved -->
<!-- ui-prototype: ui-prototype/index.html -->

## User Journey

**入口**：手机浏览器访问 Syntopica 站点（375px 级视口），或桌面浏览器窗口缩窄至 <768px。

**主任务**：
1. 刷文章列表 → 点选文章 → 阅读正文 → 返回列表继续刷（核心阅读循环，全程单栏）
2. 打开抽屉导航 → 切换订阅源/关注标签筛选 → 回到列表看结果
3. 查看日报 / 进入 discovery / settings / tags 页面完成查看与基本操作

**次任务**：顶栏通知铃查看通知；刷新按钮拉取新文章；主题切换。

## Information Architecture

窄屏下信息层级从「三栏并列」折叠为「单栏 + 两层导航遮罩」：

```
┌─ 顶栏（常驻单行）────────────────┐
│ ☰ 汉堡  Logo   🔔  ⟳ 主题      │
├─────────────────────────────────┤
│                                 │
│  列表态（默认）                   │
│  ├ 文章卡片流                    │
│  └ 筛选 chips（横向滚动）         │
│                                 │
│  ─或─ 阅读态（点文章后）           │
│  ├ ← 返回条                      │
│  └ 正文（reader 排版）            │
└─────────────────────────────────┘

抽屉态（点 ☰）：侧栏 overlay 滑出
┌────────────┬────────────────────┐
│ 订阅源列表   │   半透明遮罩        │
│ 关注标签     │   (点击关闭)        │
│ 导航入口     │                    │
└────────────┴────────────────────┘
```

- 列表态 ↔ 阅读态互斥单栏切换，无双栏并存
- 抽屉复用现有侧栏信息结构（订阅源树 / 关注标签 / 页面导航），不重组内容
- 宽屏（≥768px）信息架构与现状完全一致

## Interaction Contract

**主操作**：
- 点列表项 → 进入阅读态（滚动位置记忆，返回列表时恢复）
- 阅读态「← 返回」/ 浏览器返回手势 → 回列表态
- 点「☰」→ 抽屉滑出（260ms ease-out）；点遮罩 / 再点 ☰ / 选定导航项 → 抽屉收起
- 抽屉内点订阅源/标签 → 应用筛选 + 关抽屉 + 回列表态（重置到列表态顶部）

**次操作**：刷新按钮（保持现有行为）；通知铃（AppDialog 92vw 兜底，不动）。

**反馈方式**：抽屉开合用 transform 过渡；列表→阅读切换无转场（即时切换，避免干扰滚动）；筛选应用后列表态顶部呈现当前筛选 chip。

**边界**：阅读态下点 ☰ 仍可开抽屉（换筛选即放弃当前文章回列表态）；旋转屏/窗口 resize 跨过 768px 断点时，恢复宽屏三栏布局并保留当前选中文章。

## State Matrix

| 状态 | 列表态 | 阅读态 |
| --- | --- | --- |
| loading | 拟真进度条（复用现有 useFakeProgress）；骨架卡片 ×N | 正文区骨架 |
| empty | FeedEmptyGuide 窄屏适配（单栏卡片化，引导按钮全宽） | 不适用（无文章即无阅读态） |
| error | 初始化错误屏窄屏适配（文案 + 重试按钮全宽） | 正文加载失败 → 阅读态内错误占位 + 重试 |
| success | 文章卡片流 + 筛选 chips | 正文 reader 排版 |

抽屉：自身无 loading/error 态（数据已由宽屏逻辑预载）；空订阅时显示引导文案。

## Layout Contract

**断点**：`<768px` 进入窄屏降级；`≥768px` 恢复现状布局（**宽屏零变化**）。

**layout mode**：主工作台沿用现状（flex 三栏，非 AppPageShell 管辖）；阅读态正文复用 reader 排版约束（760px 上限在窄屏自动失效，即 100% − gutter）。discovery/settings/tags 维持各自 contained/workspace 模式不变，仅做 375px 破版修复。

**各栏宽度与溢出策略**：
| 区块 | 窄屏宽度 | 溢出策略 |
| --- | --- | --- |
| 顶栏 | 100% 单行，高 56px | 溢出项收「⋯」菜单 |
| 文章列表 / 阅读正文 | 100%（gutter 16px，宽屏 24px） | 纵向滚动 |
| 抽屉 | min(80vw, 320px)，滑入 overlay | 内容纵向滚动 |
| 筛选 chips | 100% | 横向滚动（overflow-x-auto） |

**高度**：`100vh` → `100dvh`（`@supports` fallback：不支持 dvh 的浏览器保留 vh）。

**dialog**：全部复用 AppDialog 现有四档（sm/md/lg/xl，92vw 上限），窄屏自动受 92vw 约束，不新增档位。**无自由宽度例外。**

**目标视口**：
- 桌面 1440×900、1920×1080：零变化回归（对照截图走查）
- 窄屏 375×667（iPhone SE）、390×844（iPhone 14）：本 change 主验收视口

## Component Reuse

| 需求 | 复用 | 新增 |
| --- | --- | --- |
| 抽屉容器 | 主题 token（--color-bg-overlay / bg-elevated）、Teleport 模式 | `AppSidebarDrawer.vue`（窄屏专用，滑入面板；不复用 AppDialog——居中 dialog 语义不符） |
| 列表/阅读切换 | FeedLayoutShell 既有状态管理 | `viewMode: 'list' \| 'reading'` 窄屏态 + `useMediaQuery` 断点 composable |
| 顶栏收纳 | AppHeaderView 既有结构 | 窄屏变体 class + 「⋯」溢出菜单 |
| 正文阅读 | ArticleContentView（reader 排版） | 无 |
| 空/错/载 | FeedEmptyGuide / 现错误屏 / useFakeProgress | 无（仅窄屏样式适配） |
| 弹窗 | AppDialog 全系 | 无 |

## Prototype

原型位于 `ui-prototype/index.html`——独立静态 HTML（editorial 主题 token，零依赖零 API），浏览器直接打开。内含 375×667 手机框演示四个交互态：列表态 / 抽屉态 / 阅读态 / 宽屏对照，点击可真实切换。fixture 数据为虚构订阅源与文章标题。与真实接口完全解耦。

## Acceptance

- **opencli 主链路断言**（375×667 视口）：列表态无横向溢出 → 点文章进阅读态 → 返回回列表态（滚动位置恢复）→ 开抽屉 → 选筛选关抽屉回列表态
- **视觉检查证据**：375×667 与 390×844 两档窄屏截图（主工作台三态 + discovery / settings / tags / 日报页）；1440×900 与 1920×1080 两档桌面截图与现状逐页对比（零变化）
- **与批准原型的差异说明**：实现完成后在 tasks.md 验证节记录与 `ui-prototype/index.html` 的差异点；信息架构/主流程/布局模式级差异需重新审批
