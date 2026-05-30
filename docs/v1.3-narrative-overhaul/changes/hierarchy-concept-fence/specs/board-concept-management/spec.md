## MODIFIED Requirements

### Requirement: Board concept persistence
系统 SHALL 存储 board concepts 在 board_concepts 表中，字段：id, name, description, embedding (pgvector), category (event/keyword/person), scope_type, scope_category_id, is_system, status (pending/active/inactive/merged), display_order, created_at, updated_at。is_active 字段 SHALL 被删除。

#### Scenario: Board concept creation with category
- **WHEN** 一个 board concept 以 name="AI开发" 和 category="event" 被创建
- **THEN** 行插入 board_concepts，status='active'（手动创建）或 'pending'（bootstrap 生成），is_system=false，category='event'

#### Scenario: Status 替代 is_active 查询
- **WHEN** 查询活跃 board concept
- **THEN** 使用 WHERE status='active' 而非 WHERE is_active=true

#### Scenario: Pending concept 不参与匹配
- **WHEN** MatchTagToConcept 执行
- **THEN** 只有 status='active' 且 embedding 非空的 concept 参与匹配

### Requirement: Board concept user CRUD
系统 SHALL 提供 API 端点管理 board concept。

#### Scenario: List all concepts including pending
- **WHEN** GET /api/hierarchy/concepts 被调用
- **THEN** 返回所有 concept（含 pending），按 category 和 display_order 排序

#### Scenario: Create concept manually
- **WHEN** POST /api/hierarchy/concepts 传入 {name, description, category}
- **THEN** 创建 status='active' 的 concept，生成 embedding

#### Scenario: Confirm pending concept
- **WHEN** POST /api/hierarchy/concepts/:id/confirm 被调用
- **THEN** concept status 从 'pending' 变为 'active'

#### Scenario: Deactivate concept
- **WHEN** DELETE /api/hierarchy/concepts/:id 被调用
- **THEN** concept status 变为 'inactive'
