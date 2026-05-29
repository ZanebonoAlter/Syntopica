# 数据库初始化模块

## 功能说明

数据库初始化模块会自动检查并创建缺失的表，确保应用正常运行。

**自动执行时机：**
- 每次启动服务器时（`InitDB()` 函数会自动调用 `EnsureTables()`）

**支持的表（共7张）：**
1. `categories` - 分类表
2. `feeds` - 订阅源表
3. `articles` - 文章表
4. `scheduler_tasks` - 定时任务表
5. `ai_settings` - AI 设置表
6. `reading_behaviors` - 阅读行为表
7. `user_preferences` - 用户偏好表

## 工作原理

1. **检查表是否存在** - 查询 `sqlite_master` 判断表是否存在
2. **创建缺失的表** - 使用 `CREATE TABLE IF NOT EXISTS` 安全创建
3. **创建索引** - 自动创建所有必要的索引（`CREATE INDEX IF NOT EXISTS`）
4. **保护现有数据** - 不会修改或删除已存在的表和数据

## 使用方式

### 自动初始化（推荐）

服务器启动时自动执行：
```bash
cd backend-go
go run cmd/server/main.go
```

### 手动检查

查看当前数据库中的表：
```bash
cd backend-go
go run cmd/list-tables/main.go
```

测试初始化功能：
```bash
cd backend-go
go run cmd/test-init/main.go
```

## 特性

✅ **安全** - 只创建表，不修改现有表结构
✅ **幂等** - 多次执行不会有副作用
✅ **自动索引** - 自动创建所有必要的索引
✅ **日志输出** - 详细记录创建过程

## 示例输出

```
2025/02/05 15:15:02 Database initialized successfully
2025/02/05 15:15:02 Creating table: reading_behaviors
2025/02/05 15:15:02 ✓ Table created: reading_behaviors
```

## 注意事项

- 使用纯 SQL 语句创建表，避免 GORM AutoMigrate 的潜在问题
- 外键约束使用 `ON DELETE CASCADE` 确保数据一致性
- 索引使用 `IF NOT EXISTS` 确保重复执行不会报错
