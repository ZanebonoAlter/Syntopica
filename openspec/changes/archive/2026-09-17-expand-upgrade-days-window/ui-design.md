<!-- ui-impact: minor -->
<!-- ui-approval: not-required -->
<!-- ui-prototype: none -->

# UI Design: expand-upgrade-days-window（minor）

## 入口与入口变更

入口不变：TagsPage「升级建议」弹窗（`UpgradeSuggestionPanel`，`BoardListSidebar` 按钮触发）。唯一入口变更：生成参数区「候选时间窗」下拉（data-testid=gen-days）从「创建×单标签」单格可见放开为四格方向组合恒显，控件本体、选项集（今天/3/7/30/全部）与默认值（今天）均不变。

## 受影响状态

- **loading**：生成进行中（persistedGenerating=true）时下拉 disabled——现状已有，不变
- **success**：生成完成后下拉保持当前选中值——不变
- **empty/error**：不涉及（下拉是参数控件，无自身空/错误态；生成结果空态文案已有「该版块可能已充分覆盖」承接，本次不动）
- **方向切换**：切换 genDirection/genSource 时下拉不再消失（本次变更的核心行为），选中值保留不清空

## 复用组件与布局模式

- 下拉控件本体：复用现有原生 select 样式（`usp-gen-days`），不新增组件
- 布局：弹窗内 `usp-gen-row--days` 行布局不变，无 layout mode 变化（dialog 尺寸沿用现有档）

## 验收映射

组件测试（tasks 2.2）：扩充方向下 `gen-days` 可见性 + emit 载荷含 days 断言，`pnpm test:unit` 绿即验收。
