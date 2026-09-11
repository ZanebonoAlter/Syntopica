# unified-form-controls Specification

## Purpose

统一表单原子组件（AppButton/AppToggle/AppInput/AppSectionHeader），响应主题 token，替代历史上多套不一致的按钮/开关/输入框样式。组件 API 细节见下方各节。

## Requirements

### Requirement: 表单原子组件统一且响应主题

表单基础控件 SHALL 统一使用原子组件族（`AppButton`/`AppToggle`/`AppInput`/`AppSectionHeader`），视觉 SHALL 经主题 token 自动适配双主题（editorial/dark），SHALL NOT 另起平行样式体系。

#### Scenario: 表单控件使用原子组件

- **WHEN** 页面需要按钮/开关/输入框/分组标题
- **THEN** 实现 SHALL 使用对应 `App*` 原子组件而非自写样式变体

#### Scenario: 双主题下表单控件可读

- **WHEN** 切换 editorial ↔ dark 主题
- **THEN** 全部表单原子组件 SHALL 经语义 token 自动适配，无可读性回归

## Capability

统一表单原子组件，响应主题 token。AppButton、AppToggle、AppInput、AppSectionHeader 四个组件，替代现有三套不一致的按钮/开关/输入框样式。

## AppButton

### Props

```typescript
interface AppButtonProps {
  variant?: 'primary' | 'secondary' | 'ghost' | 'danger'
  size?: 'sm' | 'md' | 'lg'
  disabled?: boolean
  loading?: boolean
  type?: 'button' | 'submit' | 'reset'
}
```

### CSS

```css
.app-button {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: 6px;
  border: none;
  cursor: pointer;
  font-weight: 500;
  border-radius: 8px;
  transition: background 0.15s, color 0.15s, opacity 0.15s;

  /* Layer 3 token defaults (primary) */
  --btn-bg: var(--color-accent);
  --btn-bg-hover: var(--color-accent-hover);
  --btn-text: var(--color-text-inverted);

  background: var(--btn-bg);
  color: var(--btn-text);
}

/* Variants */
.app-button--secondary {
  --btn-bg: var(--color-bg-hover);
  --btn-bg-hover: var(--color-bg-active);
  --btn-text: var(--color-text-primary);
}
.app-button--ghost {
  --btn-bg: transparent;
  --btn-bg-hover: var(--color-bg-hover);
  --btn-text: var(--color-text-secondary);
}
.app-button--danger {
  --btn-bg: var(--color-error);
  --btn-bg-hover: var(--color-error);
  --btn-text: #fff;
}

/* Sizes */
.app-button--sm  { padding: 4px 10px; font-size: 13px; }
.app-button--md  { padding: 6px 14px; font-size: 14px; }
.app-button--lg  { padding: 8px 18px; font-size: 15px; }

/* States */
.app-button:hover   { background: var(--btn-bg-hover); }
.app-button:active  { opacity: 0.85; }
.app-button:disabled { opacity: 0.4; cursor: not-allowed; }
```

### Migration Map

```
旧                              → 新
──────────────────────────────────────
.btn-primary                    → <AppButton variant="primary">
.btn-primary-sm                 → <AppButton variant="primary" size="sm">
.btn-secondary-sm               → <AppButton variant="secondary" size="sm">
.dialog-btn--primary            → <AppButton variant="primary">
button.btn.btn-primary          → <AppButton variant="primary">
.dialog-btn                     → <AppButton variant="secondary">
button class="mc-btn"           → <AppButton variant="secondary">
.gray-btn / .cancel-btn         → <AppButton variant="ghost">
标签页内的各种 inline button     → <AppButton>
```

**不在本 change 范围内的按钮**：
- `ArticleContentToolbar` 的 `.action-btn` icon button（工具栏专用图标按钮，非表单场景）
- `ArticleContentView` 的 `.back-to-top-btn`（浮动按钮，非表单场景）
- `app.vue` 的刷新按钮（应用级错误恢复，独立场景，迁移到 AppButton 即可）

## AppToggle

### Props

```typescript
interface AppToggleProps {
  modelValue: boolean
  disabled?: boolean
  label?: string
}
```

### CSS

```css
.app-toggle {
  --toggle-width: 36px;
  --toggle-height: 20px;
  --toggle-radius: 10px;
  --toggle-track: var(--color-border-medium);
  --toggle-track-active: var(--color-accent);
  --toggle-thumb: #fff;
  --toggle-thumb-size: 16px;

  display: inline-flex;
  align-items: center;
  gap: 8px;
  cursor: pointer;
}

.app-toggle__track {
  width: var(--toggle-width);
  height: var(--toggle-height);
  border-radius: var(--toggle-radius);
  background: var(--toggle-track);
  position: relative;
  transition: background 0.2s;
}
.app-toggle.is-active .app-toggle__track {
  background: var(--toggle-track-active);
}

.app-toggle__thumb {
  width: var(--toggle-thumb-size);
  height: var(--toggle-thumb-size);
  border-radius: 50%;
  background: var(--toggle-thumb);
  position: absolute;
  top: 2px;
  left: 2px;
  transition: transform 0.2s;
  box-shadow: 0 1px 3px rgba(0,0,0,0.2);
}
.app-toggle.is-active .app-toggle__thumb {
  transform: translateX(16px);
}

.app-toggle__label {
  font-size: 14px;
  color: var(--color-text-secondary);
}
```

### Migration Map

```
旧                              → 新
──────────────────────────────────────
<button> + :pressed 手写 toggle  → <AppToggle v-model="value">
<input type="checkbox" + peer    → <AppToggle v-model="value">
<label> + checkbox hack           → <AppToggle v-model="value">
<input type="checkbox" class="rounded"> (AI 面板) → <AppToggle v-model="value">
<input type="checkbox" class="dialog-checkbox"> (tags 面板) → <AppToggle v-model="value">
```

涉及：EditFeedDialog (×3), FirecrawlConfigPanel, AIRouterSettingsPanel, AIRouterBackupProviders (×3), AddSemanticBoardDialog, BoardTimelinePanel, AuxiliaryLabelPicker

## AppInput

### Props

```typescript
interface AppInputProps {
  modelValue: string | number
  type?: string              // 默认 'text'，支持 'number'
  placeholder?: string
  disabled?: boolean
  error?: string             // 错误提示文字
  step?: string | number    // 透传给 <input> (number 类型用)
  min?: string | number     // 透传给 <input> (number 类型用)
  max?: string | number     // 透传给 <input> (number 类型用)
}
```

### CSS

```css
.app-input-wrapper {
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.app-input {
  padding: 8px 12px;
  font-size: 14px;
  border: 1px solid var(--color-input-border);
  border-radius: 8px;
  background: var(--color-input-bg);
  color: var(--color-text-primary);
  outline: none;
  transition: border-color 0.15s, box-shadow 0.15s;
}
.app-input::placeholder {
  color: var(--color-text-muted);
}
.app-input:focus {
  border-color: var(--color-input-focus);
  box-shadow: 0 0 0 2px var(--color-accent-subtle);
}
.app-input.is-error {
  border-color: var(--color-error);
}
.app-input:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}

.app-input-error {
  font-size: 12px;
  color: var(--color-error);
}
```

### Migration Map

```
旧                              → 新
──────────────────────────────────────
<input class="input">            → <AppInput v-model="value">
<input> + inline Tailwind        → <AppInput v-model="value">
<input class="mc-input">         → <AppInput v-model="value">
<input> + scoped CSS per-dialog  → <AppInput v-model="value">
```

## AppSectionHeader

### Props

```typescript
interface AppSectionHeaderProps {
  title: string
  description?: string
  icon?: Component
}
```

### CSS

```css
.app-section-header {
  margin-bottom: 16px;
}

.app-section-header__icon {
  width: 32px;
  height: 32px;
  border-radius: 8px;
  background: var(--color-accent-subtle);
  color: var(--color-accent);
  display: flex;
  align-items: center;
  justify-content: center;
  margin-bottom: 8px;
}

.app-section-header__title {
  font-size: 14px;
  font-weight: 600;
  color: var(--color-text-primary);
  margin: 0;
}

.app-section-header__desc {
  font-size: 13px;
  color: var(--color-text-muted);
  margin-top: 2px;
}
```

### Migration Map

```
旧                              → 新
──────────────────────────────────────
各面板自建的 <h3> + <p> header   → <AppSectionHeader>
手写的 icon-box + title          → <AppSectionHeader icon="...">
```

## Shared Constraints

- 所有组件使用 Layer 2 语义 token，不使用任何硬编码色值
- 不使用 Tailwind class 做主题相关的样式（布局用 Tailwind 可以）
- 不使用 `dark:` 前缀
- 组件文件位置: `components/ui/` 目录
- 组件注册方式: 按需 import（不做全局注册）
- 所有组件支持 `v-model` 绑定（AppInput、AppToggle）
- AppButton 的 `loading` 状态显示一个小的 CSS spinner
