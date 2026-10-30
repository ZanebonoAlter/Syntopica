# unified-dialog Specification

## Purpose

统一对话框外壳组件 `AppDialog`：Teleport 到 body，overlay + 关闭行为 + 动画，Layer 2 语义 token 自动适配主题。布局契约权威源在 `standard/frontend/layout.md`（dialog 四档），本 spec 固化组件级行为。

## Requirements

### Requirement: 统一对话框外壳 AppDialog

新弹窗 SHALL 复用统一外壳 `AppDialog`，尺寸经 `size` 属性四档选择，SHALL 受可视区宽度约束（92vw 上限），SHALL NOT 传自由 width。遮罩层级 SHALL 取 `Z_INDEX.dialog` 档位（不接受组件内手写数字）；需要高于基础档（阅读层等）时经 `z-index` prop 显式指定，并 SHALL 高于宿主浮层。

#### Scenario: 新弹窗使用四档尺寸

- **WHEN** 需要新增一个弹窗
- **THEN** 实现 MUST 复用 `<AppDialog size="...">` 四档之一，不传自由 width（与 `standard/frontend/layout.md` dialog 契约一致）

#### Scenario: 主题自动适配

- **WHEN** 在 editorial/dark 任一主题下打开 AppDialog
- **THEN** overlay 与内容 SHALL 经 Layer 2 语义 token 自动适配当前主题

#### Scenario: 默认遮罩层级取基础档

- **WHEN** 使用 `AppDialog` 未显式传 `z-index`
- **THEN** 遮罩层级 SHALL 等于 `Z_INDEX.dialog`（1000），与层级档位表一致

### Requirement: 浮层层级契约

系统 SHALL 为浮层提供**唯一档位表**（`front/app/components/ui/layout-contract.ts` 的 `Z_INDEX`，权威源 `docs/reference/standard/frontend/layout.md`「浮层层级契约」节），组件 SHALL NOT 自行手写层级数字（存量例外见下）。

档位与规则：

- 基础弹窗遮罩 = `Z_INDEX.dialog`（1000）。
- 弹窗内再开的浮层（嵌套浮层）层级 SHALL **严格高于其宿主弹窗遮罩**——否则被宿主遮罩整体盖住，用户点了入口看不到浮层。
- 嵌套浮层默认取 `Z_INDEX.dialogNested`（1100）；宿主弹窗处于更高档时，宿主 SHALL 用 `nestedOverlayZ(hostZ)` 抬升并显式传入——返回值 SHALL 严格大于 hostZ，SHALL **不落在任何已占档位上**（档位表是唯一占用清单；裸抬升撞上「弹窗内全屏层」9200 时 SHALL 继续抬升跳过该档），且 SHALL 不越过全局提示档 `Z_INDEX.toast`。
- 全屏层 SHALL 高于其宿主弹窗遮罩，且低于全局提示层（`Z_INDEX.toast`）。
- **存量例外**：已入库组件的历史手写层级数字保持现状（登记表见 `standard/frontend/layout.md`「存量迁移」节），不随本契约强制迁移；主动重构该组件时 SHALL 一并收敛到档位表。

#### Scenario: 弹窗内嵌套浮层高于宿主遮罩

- **WHEN** 用户在弹窗内（版块治理工作面等）打开一个嵌套浮层（组合标签「创建并挂载」弹窗）
- **THEN** 嵌套浮层的 z-index SHALL 严格大于宿主弹窗遮罩的 z-index，渲染在宿主之上，不被宿主遮罩遮挡

#### Scenario: 嵌套浮层档位可按宿主抬升

- **WHEN** 宿主弹窗使用高于基础档的层级（如阅读层弹窗 9100）
- **THEN** `nestedOverlayZ(hostZ)` SHALL 返回严格大于 hostZ 的层级，供宿主显式传给嵌套浮层
- **AND** 返回值 SHALL 不等于任何其它已占档位（如 `readingFullscreen` 9200），且 SHALL 不超过 `Z_INDEX.toast`

#### Scenario: 层级档位单一事实源

- **WHEN** 新增或修改任一浮层组件
- **THEN** 其层级 SHALL 取自 `Z_INDEX` 档位表（组件内不得手写层级数字），且 `AppDialog` 默认层级 SHALL 等于 `Z_INDEX.dialog`

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
  /** 尺寸档：sm=420 / md=560 / lg=760 / xl=1040（受 92vw 上限约束）；新代码用它替代自由 width */
  size?: 'sm' | 'md' | 'lg' | 'xl'
  /** 宽度（@deprecated 存量兼容，新代码用 size） */
  width?: string            // 默认 '480px'
  /** 点击 overlay 是否关闭 */
  closeOnOverlay?: boolean  // 默认 true
  /** 按 Escape 是否关闭 */
  closeOnEscape?: boolean   // 默认 true
  /** 是否显示关闭按钮 */
  showClose?: boolean       // 默认 true
  /** 遮罩层级：默认取 Z_INDEX.dialog（1000）；高于基础档（阅读层等）时显式传入 */
  zIndex?: number
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
- 嵌套 dialog 已支持：层级按「浮层层级契约」管理——弹窗内再开的浮层取 `Z_INDEX.dialogNested`（1100），宿主在更高档时用 `nestedOverlayZ(hostZ)` 抬升（跳过已占档位、不越过 `toast`）；档位表唯一事实源在 `front/app/components/ui/layout-contract.ts`
- GlobalSettings 的"去掉底部完成按钮"在此 spec 内完成
- ArticlePreviewModal 只有一份 (features/tags/)，不存在合并问题
- AppDialog 不用于完整设置中心、超长管理列表或需要 URL 导航状态的工作区
