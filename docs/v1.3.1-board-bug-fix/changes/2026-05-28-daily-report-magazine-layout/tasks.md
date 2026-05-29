## 1. TagsPage 基础调整（已完成）

- [x] 1.1-1.3 布局宽度 1800px、默认 tab→文章、sticky 面板

## 2. 文章时间筛选快捷 chip（已完成）

- [x] 2.1-2.5 quickRange + chip UI + 样式

## 3. 日报概要列表收起态（已完成）

- [x] 3.1-3.5 概要卡片列表 + staggered fade-in

## 4. 全屏旧报纸弹窗（已完成）

- [x] 4.1 弹窗结构：全屏遮罩 + 居中纸张面板，Teleport to body
- [x] 4.2 纸张面板样式：#f4eed7 泛黄底色 + SVG 噪点纹理 + inset vignette + box-shadow
- [x] 4.3 Noto Serif SC 字体引入（Google Fonts）
- [x] 4.4 顶栏：[↑上一天] 日期 [↓下一天] + × 关闭，首末天置灰
- [x] 4.5 按章节分页：pages computed，第1页=highlights+dynamics，后续每页=一个 section
- [x] 4.6 纸张内容区：旧报纸排版（衬线标题+无衬线正文+深色系分隔线）
- [x] 4.7 左右边缘翻页按钮，首页/末页置灰
- [x] 4.8 页码指示 n/N

## 5. 导航交互（已完成）

- [x] 5.1 键盘快捷键：←→翻页，↑↓换天，Esc 关闭
- [x] 5.2 上下换天：页码重置为1，加载对应天详情
- [x] 5.3 左右翻页动画：Vue Transition + translateX，300ms ease-out

## 6. 弹窗动画（已完成）

- [x] 6.1 弹窗打开：遮罩 fade-in 200ms + 面板 scale(0.95→1)+fade-in 300ms
- [x] 6.2 弹窗关闭：反向 200ms
- [x] 6.3 删除旧的 .drt-switch 过渡 CSS

## 7. 清理（已完成）

- [x] 7.1 删除旧杂志页模板和样式（.drt-magazine-*, .drt-mag-*）
- [x] 7.2 重构状态（viewMode→showModal, openMagazine→openNewspaper）

## 8. 验证（已完成）

- [x] 8.1 pnpm lint ✅
- [x] 8.2 pnpm exec nuxi typecheck ✅
- [x] 8.3 pnpm build ✅
- [ ] 8.4 手动验证
