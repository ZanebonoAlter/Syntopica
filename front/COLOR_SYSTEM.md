# 前端颜色系统设计文档 - Editorial Magazine Theme

## 设计理念

**Editorial/Magazine（杂志/报纸）风格**

RSS 阅读器本质是内容消费工具，应该让用户感觉像在阅读精心设计的杂志，而不是使用 SaaS 软件。

- **温暖的纸张色调** - 代替冷冰冰的玻璃态
- **高对比度排版** - 强调内容可读性
- **印刷品经典配色** - 墨水蓝 + 印刷红
- **优雅的阴影** - 模拟纸质印刷品的质感

---

## 颜色系统架构

### 1. Ink Blue - 墨水蓝（主色）

替代原蓝紫色，更像印刷品：

```
--color-ink-50:   #e8eef4  (最浅，用于背景)
--color-ink-100:  #d0dde8
--color-ink-200:  #a8bdd0
--color-ink-300:  #7a9cb5
--color-ink-400:  #56839c
--color-ink-500:  #3b6b87  ⭐ 主色，用于按钮、链接
--color-ink-600:  #2d5670
--color-ink-700:  #25465c
--color-ink-800:  #1f3b4d
--color-ink-900:  #1a3140  (最深，用于文字)
```

### 2. Print Red - 印刷红（强调色）

替代原紫色，用于重要操作、提示：

```
--color-print-red-50:   #fef2f2
--color-print-red-100:  #fde6e6
--color-print-red-200:  #f9c5c5
--color-print-red-300:  #f29a9a
--color-print-red-400:  #e87070
--color-print-red-500:  #d94a4a  ⭐ 强调色
--color-print-red-600:  #c12f2f
--color-print-red-700:  #a41f1f
--color-print-red-800:  #8a1818
--color-print-red-900:  #721515
```

### 3. Paper Warmth - 纸张暖色调（背景）

```
--color-paper-ivory: #faf7f2  象牙白（主背景）
--color-paper-cream: #f5f0e6  奶油色
--color-paper-warm:  #f0e8dc  暖纸色
--color-paper-sand:  #e8dfd1  沙色
```

### 4. Ink Tones - 文字墨色

```
--color-ink-black:   #1a1a1a  主文字
--color-ink-dark:    #2d2d2d  次要文字
--color-ink-medium:  #5a5a5a  辅助文字
--color-ink-light:   #8a8a8a  占位/禁用
--color-ink-muted:   #b5b5b5  边框/分隔
```

### 5. Accent Colors - 强调色系

```
--color-accent-teal:    #2d8a7a  青绿（成功状态）
--color-accent-amber:   #d4883c  琥珀（收藏/警告）
--color-accent-indigo:  #4a5d8a  靛蓝（信息）
--color-accent-forest:  #3d7a4a  森林绿
```

### 6. Semantic Colors - 语义色

```
--color-success: #3d8a4a  成功
--color-warning: #c4883c  警告
--color-error:   #c42f3c  错误
--color-info:    #3d7a8a  信息
```

---

## 组件样式指南

### 按钮

```css
/* 主要操作 - 墨水蓝 */
.btn-primary {
  background: var(--color-ink-500);
  color: white;
}

/* 强调操作 - 印刷红 */
.btn-accent {
  background: var(--color-print-red-500);
  color: white;
}

/* 次要操作 */
.btn-secondary {
  background: transparent;
  border: 2px solid var(--color-ink-300);
  color: var(--color-ink-500);
}

/* 幽灵按钮 */
.btn-ghost {
  background: transparent;
  border: 1px solid var(--color-border-subtle);
  color: var(--color-ink-medium);
}
```

### 卡片

```css
/* 纸张卡片 */
.paper-card {
  background: rgba(255, 255, 255, 0.7);
  border: 1px solid var(--color-border-subtle);
  box-shadow: var(--shadow-subtle);
}

/* 文章预览 */
.article-preview {
  background: linear-gradient(to bottom, ...);
  border-left: 3px solid transparent;
}

.article-preview.unread {
  border-left-color: var(--color-print-red-500);
}

.article-preview.favorite {
  border-left-color: var(--color-accent-amber);
}
```

### 输入框

```css
.input {
  background: rgba(255, 255, 255, 0.85);
  border: 2px solid var(--color-border-medium);
  color: var(--color-ink-black);
}

.input:focus {
  border-color: var(--color-ink-400);
  box-shadow: 0 0 0 3px rgba(59, 107, 135, 0.1);
}
```

---

## 分类/订阅源颜色选项

更新后的颜色选项（`constants.ts`）：

```typescript
export const COLOR_OPTIONS = [
  '#3b6b87', // Ink Blue - 墨水蓝
  '#c12f2f', // Print Red - 印刷红
  '#2d8a7a', // Teal - 青绿
  '#d4883c', // Amber - 琥珀
  '#4a5d8a', // Indigo - 靛蓝
  '#3d7a4a', // Forest - 森林绿
  '#8a5a4a', // Sepia - 褐色
  '#5a5a5a', // Charcoal - 炭灰
] as const
```

这些颜色更符合杂志印刷风格，避免高饱和度的霓虹色。

---

## 阴影系统

```css
--shadow-subtle:  0 1px 3px rgba(26, 26, 26, 0.06);   轻微
--shadow-medium:  0 2px 8px rgba(26, 26, 26, 0.08);   中等
--shadow-strong:   0 4px 16px rgba(26, 26, 26, 0.12);  强烈
--shadow-print:    0 1px 0 rgba(26, 26, 26, 0.1),      印刷质感
                   0 2px 4px rgba(26, 26, 26, 0.06);
```

---

## 边框系统

```css
--color-border-subtle:  rgba(26, 26, 26, 0.08);  轻微
--color-border-medium:  rgba(26, 26, 26, 0.15);  中等
--color-border-strong:  rgba(26, 26, 26, 0.25);  强烈
```

---

## 状态徽章

```css
.status-badge {
  padding: 0.25rem 0.625rem;
  border-radius: 0.25rem;
  font-size: 0.75rem;
  font-weight: 600;
  text-transform: uppercase;
  letter-spacing: 0.05em;
}

.status-badge-success {
  background: rgba(61, 138, 74, 0.12);
  color: var(--color-success);
  border: 1px solid rgba(61, 138, 74, 0.25);
}
```

---

## 使用建议

### DO ✅

1. **使用 CSS 变量** - 所有颜色都通过变量定义，便于统一修改
2. **优先使用墨水蓝** - 作为主要交互色
3. **印刷红用于强调** - 重要操作、提示、错误
4. **纸张色作背景** - 创造温暖、舒适的阅读环境
5. **深色文字** - 保证可读性（`#1a1a1a` 或 `#2d2d2d`）

### DON'T ❌

1. **避免使用原紫色** - `#6366F1`、`#8B5CF6` 等已被替换
2. **避免纯色背景** - 使用纸张色系或半透明
3. **避免高饱和度颜色** - 保持印刷品的沉稳感
4. **避免玻璃态过度** - 减少模糊效果，增强清晰度
5. **避免过多动画** - 保持克制、优雅的过渡效果

---

## 与原系统对比

| 项目 | 原系统 | 新系统 |
|------|--------|--------|
| 主色 | 蓝紫色 `#6e7bcc` | 墨水蓝 `#3b6b87` |
| 强调色 | 紫色 `#8b5cf6` | 印刷红 `#d94a4a` |
| 背景 | 冷灰 `#f8fafc` | 纸张暖色 `#faf7f2` |
| 阴影 | 蓝色调 `rgba(100,100,120,.1)` | 墨色调 `rgba(26,26,26,.08)` |
| 风格 | 玻璃态 | 纸质感 |

---

## 字体建议（待实现）

为完整实现杂志风格，建议配置：

```css
--font-display: 'Playfair Display', 'Georgia', serif;  标题
--font-body: 'Source Serif 4', 'Georgia', serif;        正文
--font-ui: 'Inter', system-ui, sans-serif;              UI 元素
```

---

## 更新日志

- **2026-02-05**: 从玻璃态蓝紫色系统重构为杂志印刷风格
- 更新所有组件 CSS 文件
- 更新 `main.css` 颜色变量
- 更新 `constants.ts` 颜色选项
- 移除所有 AI 组件和 Dialog 中的玻璃态样式
- 移除所有紫色 (`#8b5cf6`, `#7c3aed`, `purple-*`)
- iframe 模式添加纸质纹理背景效果
- 所有 Dialog 组件统一使用杂志风格圆角和边框

---

## 组件更新清单

### 已完全更新的组件

✅ **AI 组件**
- `AISummary.vue` - AI 总结卡片
- `AISummaryDetail.vue` - AI 摘要详情（含内容样式）
- `AISummariesList.vue` - AI 摘要列表

✅ **Dialog 组件**
- `AddFeedDialog.vue` - 添加订阅源
- `EditFeedDialog.vue` - 编辑订阅源
- `AddCategoryDialog.vue` - 添加分类
- `EditCategoryDialog.vue` - 编辑分类
- `GlobalSettingsDialog.vue` - 全局设置
- `ImportOpmlDialog.vue` - OPML 导入
- `Dialog.css` - 对话框基础样式

✅ **Layout 组件**
- `FeedLayout.vue` - 主布局
- `AppHeader.vue` - 顶部栏
- `AppSidebar.vue` - 侧边栏
- `ArticleListPanel.vue` - 文章列表面板

✅ **Article 组件**
- `ArticleCard.vue` - 文章卡片
- `ArticleContent.vue` - 文章内容（含 iframe 背景）

✅ **其他组件**
- `CategoryCard.vue` - 分类卡片
- `FeedIcon.vue` - 订阅图标
- `RefreshStatusIcon.vue` - 刷新状态图标

### 样式替换对照

| 旧值 | 新值 | 组件 |
|------|------|------|
| `glass-card` | `paper-card` | ArticleCard, AISummaryDetail |
| `glass-strong` | `bg-white/95 backdrop-blur-sm` | 所有 Dialog |
| `rounded-3xl` | `rounded-lg` | Dialog 容器 |
| `rounded-2xl` | `rounded-lg` | 各种卡片 |
| `from-purple-* to-blue-*` | `from-ink-* to-paper-*` | AI 组件渐变 |
| `#8b5cf6` (紫色) | `#3b6b87` (墨水蓝) | 主色调 |
| `#7c3aed` (深紫) | `#25465c` (深墨) | 文字/强调 |
| `text-primary-*` | `text-ink-*` | 所有组件 |
| `bg-primary-*` | `bg-ink-*` | 所有组件 |
| `border-primary-*` | `border-ink-*` | 所有组件 |
| `text-gray-*` | `text-ink-*` | 文字颜色 |
| `bg-gray-*` | `bg-paper-*` | 背景颜色 |
