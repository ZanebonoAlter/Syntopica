# unified-dialog Specification

## Purpose

统一对话框外壳组件 `AppDialog`：Teleport 到 body，overlay + 关闭行为 + 动画，Layer 2 语义 token 自动适配主题。布局契约权威源在 `standard/frontend/layout.md`（dialog 四档），本 spec 固化组件级行为。

## Requirements

### Requirement: 统一对话框外壳 AppDialog

新弹窗 SHALL 复用统一外壳 `AppDialog`，尺寸经 `size` 属性四档选择，SHALL 受可视区宽度约束（92vw 上限），SHALL NOT 传自由 width。

#### Scenario: 新弹窗使用四档尺寸

- **WHEN** 需要新增一个弹窗
- **THEN** 实现 MUST 复用 `<AppDialog size="...">` 四档之一，不传自由 width（与 `standard/frontend/layout.md` dialog 契约一致）

#### Scenario: 主题自动适配

- **WHEN** 在 editorial/dark 任一主题下打开 AppDialog
- **THEN** overlay 与内容 SHALL 经 Layer 2 语义 token 自动适配当前主题

## Capability

统一对话框外壳组件。Teleport 到 body，overlay + 关闭行为 + 动画，通过 Layer 2 语义 token 自动适配主题。替代现有四套 dialog pattern。

## API

### Props

```typescript
interface AppDialogProps {
  /** 控制显隐 */
  modelValue: boolean
  /** 对话框标题 */
  title?: string
  /** 宽度 */
  width?: string            // 默认 '480px'
  /** 点击 overlay 是否关闭 */
  closeOnOverlay?: boolean  // 默认 true
  /** 按 Escape 是否关闭 */
  closeOnEscape?: boolean   // 默认 true
  /** 是否显示关闭按钮 */
  showClose?: boolean       // 默认 true
}
```

### Emits

```typescript
interface AppDialogEmits {
  'update:modelValue': (value: boolean) => void
}
```

### Slots

```typescript
interface AppDialogSlots {
  /** 标题区域（覆盖 title prop） */
  header?: () => VNode
  /** 对话框内容 */
  default: () => VNode
  /** 底部操作区 */
  footer?: () => VNode
}
```

### Usage

```vue
<AppDialog v-model="showDialog" title="编辑话题">
  <template #default>
    <!-- 内容 -->
  </template>
  <template #footer>
    <AppButton variant="primary" @click="save">保存</AppButton>
    <AppButton variant="ghost" @click="showDialog = false">取消</AppButton>
  </template>
</AppDialog>
```

## Structure

### DOM

```
<Teleport to="body">
  <Transition name="dialog">
    <div v-if="modelValue" class="app-dialog-overlay">
      <div class="app-dialog" :style="{ maxWidth: width }">
        <div class="app-dialog__header" v-if="title || $slots.header">
          <slot name="header">
            <h2 class="app-dialog__title">{{ title }}</h2>
          </slot>
          <button v-if="showClose" class="app-dialog__close" @click="close">
            ✕
          </button>
        </div>
        <div class="app-dialog__body">
          <slot />
        </div>
        <div class="app-dialog__footer" v-if="$slots.footer">
          <slot name="footer" />
        </div>
      </div>
    </div>
  </Transition>
</Teleport>
```

### CSS

```css
.app-dialog-overlay {
  position: fixed;
  inset: 0;
  z-index: 1000;
  display: flex;
  align-items: center;
  justify-content: center;
  background: var(--color-bg-overlay);
  backdrop-filter: blur(4px);
}

.app-dialog {
  background: var(--color-dialog-bg);
  backdrop-filter: blur(16px);
  border: 1px solid var(--color-border-subtle);
  border-radius: 12px;
  box-shadow: var(--shadow-strong);
  max-height: 85vh;
  display: flex;
  flex-direction: column;
  overflow: hidden;
}

.app-dialog__header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 16px 20px;
  border-bottom: 1px solid var(--color-dialog-divider);
  background: var(--color-dialog-header);
}

.app-dialog__title {
  font-size: 16px;
  font-weight: 600;
  color: var(--color-text-primary);
  margin: 0;
}

.app-dialog__close {
  width: 28px;
  height: 28px;
  border: none;
  background: transparent;
  color: var(--color-text-muted);
  cursor: pointer;
  border-radius: 6px;
  display: flex;
  align-items: center;
  justify-content: center;
}
.app-dialog__close:hover {
  background: var(--color-bg-hover);
  color: var(--color-text-secondary);
}

.app-dialog__body {
  padding: 20px;
  overflow-y: auto;
  flex: 1;
  color: var(--color-text-primary);
}

.app-dialog__footer {
  display: flex;
  align-items: center;
  justify-content: flex-end;
  gap: 8px;
  padding: 12px 20px;
  border-top: 1px solid var(--color-dialog-divider);
}
```

### Transition

```css
.dialog-enter-active,
.dialog-leave-active {
  transition: opacity 0.2s ease;
}
.dialog-enter-active .app-dialog,
.dialog-leave-active .app-dialog {
  transition: transform 0.2s ease, opacity 0.2s ease;
}

.dialog-enter-from {
  opacity: 0;
}
.dialog-enter-from .app-dialog {
  transform: scale(0.97);
  opacity: 0;
}

.dialog-leave-to {
  opacity: 0;
}
.dialog-leave-to .app-dialog {
  transform: scale(0.97);
  opacity: 0;
}
```

## Migration Map

### Pattern A → AppDialog (主阅读页暖白对话框)

```
现有: <div class="overlay"> + <div class="dialog">
迁移: <AppDialog v-model="show" title="...">
保留: 内部表单内容不动
删除: overlay/dialog CSS, .dialog-btn-*
```

涉及: AddFeedDialog, EditFeedDialog, AddCategoryDialog, EditCategoryDialog, ImportOpmlDialog

### Pattern B → AppDialog (标签/图谱深色对话框)

```
现有: <div class="mc-overlay"> + <div class="mc-dialog">
迁移: <AppDialog v-model="show" title="...">
保留: 内部表单内容不动
删除: mc-* CSS 全部
```

涉及: AddSemanticBoardDialog, BoardEditDialog, MatchingConfigDialog, TopicGraphMergeDialog, NarrativeGenerateDialog

### Pattern C → AppDialog (GlobalSettings 白色对话框)

```
现有: 纯 Tailwind 手写布局
迁移: <AppDialog v-model="show" title="设置">
保留: 各面板组件不动
删除: 外壳 Tailwind class
```

涉及: GlobalSettingsDialog

> 后续决策（2026-06-12）：浏览器回归证明 Global Settings 已超出 Dialog 的合理职责。该 Pattern C 迁移仅作为统一组件过程中的过渡实现，最终由 `settings-workspace` 独立路由替代。`AppDialog` 不再承载多 section 配置中心。

### Pattern D → AppDialog (全屏预览)

```
现有: ArticlePreviewModal (tags/ 下一份，带 iframe 的全屏覆盖)
迁移: <AppDialog v-model="show" width="90vw">
保留: iframe 渲染逻辑
```

涉及: ArticlePreviewModal (仅 features/tags/ 一份；topic-graph 使用 useArticlePreview composable 复用同一逻辑，无独立 Modal 组件)

## Constraints

- AppDialog 不包含任何主题逻辑——所有视觉由 Layer 2 token 决定
- overlay 的 `backdrop-filter: blur(4px)` 在 dark 主题下保留（效果更好）
- 对话框 `backdrop-filter: blur(16px)` 在两个主题下都保留
- `closeOnEscape` 通过 `keydown.escape` 监听实现，dialog 可见时绑定
- 不支持嵌套 dialog（如果业务需要，后续再加 z-index 管理）
- GlobalSettings 的"去掉底部完成按钮"在此 spec 内完成
- ArticlePreviewModal 只有一份 (features/tags/)，不存在合并问题
- AppDialog 不用于完整设置中心、超长管理列表或需要 URL 导航状态的工作区
