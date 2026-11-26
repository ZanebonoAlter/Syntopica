<!-- domain: topic-graph, ui -->

## Purpose

主题图（TopicGraphCanvas）的用户交互增强：显示设置面板（视图缩放、节点/标签尺寸控制）、节点拖拽和响应式刷新。

## Requirements

### Requirement: 主题图显示设置面板

`TopicGraphCanvas.client.vue` SHALL 提供默认折叠的显示设置面板，包含以下控制：

- 视图缩放：调整相机与图中心的相对距离，不改变语义节点权重
- 节点尺寸：对渲染半径应用本地 multiplier
- 标签尺寸：对 `SpriteText.textHeight` 应用本地 multiplier
- 重置视图：恢复默认相机位置、节点尺寸和标签尺寸

设置 SHALL 只影响当前组件实例，不写入 `localStorage` 或后端配置。OrbitControls 原有滚轮和触控缩放 SHALL 保持可用。

#### Scenario: 调整视图缩放

- **WHEN** 用户调整视图缩放滑块
- **THEN** 相机 SHALL 相对图中心拉近或拉远，节点语义 size 数据保持不变

#### Scenario: 调整节点尺寸

- **WHEN** 用户提高节点尺寸 multiplier
- **THEN** 所有节点渲染半径 SHALL 按比例增大，节点之间的权重相对关系保持不变

#### Scenario: 调整标签尺寸

- **WHEN** 用户提高标签尺寸 multiplier
- **THEN** 标签的 `textHeight` SHALL 按比例增大

#### Scenario: 重置显示设置

- **WHEN** 用户点击"重置视图"
- **THEN** 相机位置、节点尺寸和标签尺寸 SHALL 恢复默认值

#### Scenario: 设置不跨组件实例持久化

- **WHEN** 用户调整设置后销毁并重新创建主题图组件
- **THEN** 新组件实例 SHALL 使用默认设置

### Requirement: 主题图节点拖拽

主题图 SHALL 启用 `3d-force-graph` 节点拖拽，不得通过 `.enableNodeDrag(false)` 禁用该能力。

#### Scenario: 拖拽节点调整布局

- **WHEN** 用户按住并拖动一个主题节点
- **THEN** 该节点 SHALL 跟随指针移动并更新图布局

### Requirement: 显示设置响应式刷新

节点尺寸、标签尺寸或视图缩放发生变化时，canvas SHALL 刷新受影响的 Three.js node object、连线样式或相机状态，不要求重新请求图数据。

#### Scenario: 调整尺寸不重新加载 API

- **WHEN** 用户调整节点尺寸或标签尺寸
- **THEN** 图 SHALL 使用现有 nodes/edges 重新渲染，且 SHALL NOT 发起新的 topic graph API 请求
