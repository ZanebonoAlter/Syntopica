## ADDED Requirements

### Requirement: TagsPage 提供标签合并入口

TagsPage 左侧栏操作按钮区 SHALL 包含"标签合并"按钮，点击后显示 `TagMergePreview` 组件。

#### Scenario: 点击标签合并按钮
- **WHEN** 用户在 TagsPage 点击左侧栏"标签合并"按钮
- **THEN** 弹出 TagMergePreview 面板
- **AND** 自动从 `tag_merge_suggestions` 表加载 pending 状态的候选对
- **AND** 用户可逐个确认合并、跳过、交换合并方向、编辑合并后名称

#### Scenario: 合并完成
- **WHEN** 用户完成合并操作并关闭面板
- **THEN** TagsPage 刷新标签数据以反映合并结果
- **AND** 已合并的 suggestion 被标记为 `merged` 状态

#### Scenario: 忽略建议
- **WHEN** 用户点击某个候选对的"忽略"按钮
- **THEN** 该 suggestion 被标记为 `dismissed` 状态
- **AND** 不再出现在候选列表中

### Requirement: 异步全量扫描 + SSE 进度展示

用户可手动触发全量扫描，发现老标签之间的相似对。扫描异步执行，进度通过 SSE 实时推送到前端。

#### Scenario: 触发全量扫描
- **WHEN** 用户点击"全量扫描"按钮
- **THEN** 后端启动异步扫描任务（同时只允许一个任务运行）
- **AND** 前端通过 SSE 接收进度更新

#### Scenario: SSE 进度推送
- **WHEN** 全量扫描正在运行
- **THEN** 前端实时展示：进度条（scanned/total）、当前处理的类别、已发现的新建议数
- **AND** 扫描完成后自动刷新候选列表

#### Scenario: 扫描已完成时再次触发
- **WHEN** 用户在扫描进行中再次点击"全量扫描"
- **THEN** 返回错误提示"扫描正在进行中"
