# 实现计划: daily-report-newspaper-modal

> 基于 `openspec/changes/daily-report-magazine-layout/` 的迭代更新

## 概述

将 `BoardDailyReportTimeline.vue` 中已实现的 inline 杂志页替换为全屏旧报纸弹窗。TagsPage 部分已完成（布局宽度、默认 tab、sticky 面板、快捷 chip），概要列表也已完成，本次只改弹窗部分。

## 任务分解

### Task 1: 重构组件状态 + 清除旧杂志页代码

**文件**: `BoardDailyReportTimeline.vue`

**改动**:
- 删除 `viewMode` ref，改为 `showModal` boolean ref
- 删除 `selectedIndex` ref，改为 `currentDayIndex` ref
- 新增 `currentPage` ref（当前内容页码，从 1 开始）
- 删除 `closeMagazine` 函数，新增 `closeNewspaper` 函数
- 删除旧 magazine 模板（`v-else-if="viewMode === 'magazine'"` 及其内容）
- 删除所有 `.drt-magazine-*`、`.drt-mag-*` 样式
- 删除 `.drt-switch-*` 过渡样式
- 删除 `totalPages`、`currentPage` computed（旧版，后面重新定义）

**验证**: `pnpm lint`

### Task 2: 实现全屏旧报纸弹窗模板

**文件**: `BoardDailyReportTimeline.vue`

**改动**:
- 使用 `<Teleport to="body">` 包裹弹窗
- 弹窗结构：
  ```
  遮罩 div (fixed 全屏, @click.self 关闭)
    纸张面板 div (居中, 泛黄底色)
      顶栏: [↑] 日期 [↓] [×]
      纸张内容区: 当前页内容
      页码: n / N
    左边缘按钮 ←
    右边缘按钮 →
  ```
- 新增 `pages` computed：将 DailyReport 按章节分页
  - Page 1: highlights + dynamics（如果有）
  - Page 2+: 每页一个 section
- 新增 `openNewspaper(idx)` 函数：设置 currentDayIndex、showModal=true、currentPage=1，加载详情
- 新增 `closeNewspaper()` 函数：showModal=false
- 新增 `nextPage()/prevPage()` 函数：翻内容页
- 新增 `prevDay()/nextDay()` 函数：换日报天，重置 currentPage=1

**验证**: `pnpm lint`

### Task 3: 旧报纸排版样式 + 字体

**文件**: `BoardDailyReportTimeline.vue`, `nuxt.config.ts`

**改动**:
- `nuxt.config.ts`: 在 `app.head.link` 中添加 Google Fonts Noto Serif SC
- 纸张面板样式：
  - `background: #f4eed7`
  - 噪点纹理: `background-image` 叠加 SVG noise 或 radial-gradient
  - 边缘泛黄: `box-shadow: inset 0 0 80px rgba(180,160,120,0.3)`
  - `box-shadow` 外层: `0 20px 60px rgba(0,0,0,0.5)` 模拟纸张浮起
  - `max-width: 800px; max-height: 90vh; border-radius: 4px`
- 遮罩样式：`position: fixed; inset: 0; background: rgba(0,0,0,0.7); z-index: 100`
- 排版层级：
  - 日期: 2.2rem Noto Serif SC, color: rgba(0,0,0,0.2)
  - 重点标题: 1.3rem Noto Serif SC, color: rgba(0,0,0,0.9)
  - 正文: 0.82rem 无衬线, color: rgba(0,0,0,0.55)
  - 聚类标题: 0.92rem Noto Serif SC, color: rgba(0,0,0,0.8)
- 分隔线改为深色系：`rgba(0,0,0,0.12)` 实线，`rgba(0,0,0,0.08)` 虚线
- 边缘翻页按钮：绝对定位，半透明背景，hover 变亮
- thread 状态 badge 颜色适配纸张底色（降饱和度）

**验证**: `pnpm lint`

### Task 4: 弹窗动画 + 键盘导航

**文件**: `BoardDailyReportTimeline.vue`

**改动**:
- 弹窗出入：Vue `<Transition>` 包裹弹窗
  - enter: 遮罩 fade-in 200ms + 面板 scale(0.95→1) + fade-in 300ms
  - leave: 反向 200ms
- 内容翻页：Vue `<Transition>` + translateX
  - 右翻：当前页向左滑出，新页从右滑入
  - 左翻：当前页向右滑出，新页从左滑入
  - 300ms ease-out
- 键盘：`onMounted` 时 `addEventListener('keydown', handleKeydown)`
  - ArrowLeft → prevPage
  - ArrowRight → nextPage
  - ArrowUp → prevDay
  - ArrowDown → nextDay
  - Escape → closeNewspaper
- `onUnmounted` 时移除监听
- 使用 `watch(showModal)` 控制监听器的注册/注销（只在弹窗打开时监听）

**验证**: `pnpm lint`

### Task 5: 最终验证

- `pnpm lint`
- `pnpm exec nuxi typecheck`（Windows cmd）
- `pnpm build`（Windows cmd）
