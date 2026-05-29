# 数据库初始化总结

## 当前数据库表（7张）

| 序号 | 表名 | 说明 |
|------|------|------|
| 1 | categories | 分类表 |
| 2 | feeds | 订阅源表 |
| 3 | articles | 文章表 |
| 4 | scheduler_tasks | 定时任务表 |
| 5 | ai_settings | AI 设置表 |
| 6 | reading_behaviors | 阅读行为表 |
| 7 | user_preferences | 用户偏好表 |

## 已实现功能

### 1. 自动初始化（EnsureTables）
- **位置**: `pkg/database/db.go`
- **触发时机**: 每次服务器启动时自动执行
- **功能**: 检查表是否存在，不存在则自动创建
- **特点**:
  - 使用纯 SQL `CREATE TABLE IF NOT EXISTS`
  - 不会修改现有表结构
  - 自动创建所有必要索引
  - 支持外键约束（CASCADE 删除）

### 2. 实用工具命令

#### 查看所有表
```bash
go run cmd/list-tables/main.go
```

#### 测试初始化功能
```bash
go run cmd/test-init/main.go
```

#### 检查数据完整性
```bash
go run cmd/check-data/main.go
```

### 3. 数据恢复
- 从 `cmd/server/rss_reader.db` 成功恢复了所有数据
- 当前数据库状态：
  - 5 个分类
  - 33 个订阅源
  - 2279 篇文章

## 使用方式

### 日常使用（推荐）
直接启动服务器，会自动初始化：
```bash
cd backend-go
go run cmd/server/main.go
```

### 手动检查
查看当前表状态：
```bash
cd backend-go
go run cmd/list-tables/main.go
```

## 技术细节

### 表创建策略
- 使用 `CREATE TABLE IF NOT EXISTS` 确保幂等性
- 使用 `CREATE INDEX IF NOT EXISTS` 确保索引安全
- 外键使用 `ON DELETE CASCADE` 保证数据一致性

### 日志输出
```
2025/02/05 15:15:02 Database initialized successfully
2025/02/05 15:15:02 Creating table: reading_behaviors
2025/02/05 15:15:02 ✓ Table created: reading_behaviors
```

## 注意事项

1. **数据安全**: 初始化模块只创建表，不修改或删除现有数据
2. **自动执行**: 服务器启动时自动检查，无需手动干预
3. **幂等性**: 可以安全地多次执行
4. **向后兼容**: 支持从旧版本数据库平滑升级

## 文件清单

| 文件 | 说明 |
|------|------|
| `pkg/database/db.go` | 数据库初始化核心逻辑 |
| `cmd/list-tables/main.go` | 列出所有表 |
| `cmd/test-init/main.go` | 测试初始化功能 |
| `cmd/check-data/main.go` | 检查数据完整性 |
| `DATABASE.md` | 数据库文档 |
