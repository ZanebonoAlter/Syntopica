## ADDED Requirements

### Requirement: 辅助标签垃圾回收（GC）
系统 SHALL 支持扫描并清理无活跃 topic_tag 引用的辅助标签。清理时 SHALL 跳过 protected=true、创建时间在 grace_days 内、或 status 非 active 的标签。清理策略支持三种模式：dry_run（仅预览）、disable（软删除）、delete（硬删除）。

#### Scenario: GC dry_run 预览
- **WHEN** 用户调用 GC API mode=dry_run, grace_days=1
- **THEN** 系统 SHALL 返回符合条件的标签数量和前 20 个示例列表，不修改任何数据

#### Scenario: GC disable 软删除
- **WHEN** 用户调用 GC API mode=disable, grace_days=1
- **THEN** 系统 SHALL 将符合条件的辅助标签 status 更新为 "disabled"，并返回实际影响的标签数量

#### Scenario: GC delete 硬删除
- **WHEN** 用户调用 GC API mode=delete, grace_days=1
- **THEN** 系统 SHALL 硬删除符合条件的辅助标签记录（含 embedding 向量），并返回实际删除的标签数量

#### Scenario: 跳过有活跃引用的标签
- **WHEN** auxiliary label "AI" 有关联的 topic_tag_semantic_labels 指向 status='active' 的 topic_tag
- **THEN** 该标签 SHALL NOT 被 GC 清理

#### Scenario: 跳过 protected 标签
- **WHEN** auxiliary label "AI" 的 protected=true，即使无任何 topic_tag 引用
- **THEN** 该标签 SHALL NOT 被 GC 清理

#### Scenario: 跳过 grace_days 内新建标签
- **WHEN** auxiliary label "量子计算" 创建于 12 小时前，无任何 topic_tag 引用，grace_days=1
- **THEN** 该标签 SHALL NOT 被 GC 清理

#### Scenario: 清理后减少匹配负载
- **WHEN** 100 个无引用的 auxiliary label 被 GC disable
- **THEN** 后续 `loadActiveAuxiliaryLabels` 和 board 匹配 SHALL 排除这些 disabled 标签

### Requirement: AuxLabelCleanup 定时任务
系统 SHALL 提供每小时执行一次的 AuxLabelCleanup 定时任务，自动对无活跃引用的辅助标签执行 disable 清理。任务 SHALL 通过现有调度器框架管理，支持查看状态、手动触发、重置统计、修改间隔。

#### Scenario: 定时自动执行
- **WHEN** AuxLabelCleanup 调度器运行中，到达下一次执行时间
- **THEN** 系统 SHALL 执行 mode=disable, grace_days=1 的 GC 清理，记录执行结果到 SchedulerTask 数据库记录

#### Scenario: 手动触发执行
- **WHEN** 用户在 GlobalSettings 定时任务面板点击 "手动执行" AuxLabelCleanup
- **THEN** 系统 SHALL 立即执行一次 GC 清理，返回执行结果

#### Scenario: 调度器状态查询
- **WHEN** 用户打开 GlobalSettings 定时任务面板
- **THEN** AuxLabelCleanup 任务 SHALL 显示名称、状态、执行次数、上次执行时间、执行耗时等统计信息

#### Scenario: 调度器跳过重复执行
- **WHEN** AuxLabelCleanup 正在执行中，到达下一次定时触发时间
- **THEN** 系统 SHALL 跳过本次执行，等待下次周期

### Requirement: 手动 GC API
系统 SHALL 提供 `POST /api/auxiliary-labels/gc` 端点，允许手动触发 GC 操作，支持指定 mode 和 grace_days 参数。

#### Scenario: 手动 dry_run
- **WHEN** POST /api/auxiliary-labels/gc body={"mode":"dry_run","grace_days":1}
- **THEN** 返回 eligible_count 和 preview 列表，不修改数据

#### Scenario: 手动 disable
- **WHEN** POST /api/auxiliary-labels/gc body={"mode":"disable","grace_days":1}
- **THEN** 符合条件的标签 status 设为 "disabled"，返回 affected_count

#### Scenario: 手动 delete
- **WHEN** POST /api/auxiliary-labels/gc body={"mode":"delete","grace_days":1}
- **THEN** 符合条件的标签被硬删除，返回 affected_count

#### Scenario: 参数校验
- **WHEN** mode 不为 dry_run / disable / delete 之一
- **THEN** 返回 400 错误

#### Scenario: grace_days 默认值
- **WHEN** 请求未提供 grace_days
- **THEN** 系统 SHALL 使用默认值 1
