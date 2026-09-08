# 调度与配置域（`tables/scheduling-config.md`）

> 真相源 = 代码（GORM struct + `postgres_migrations.go`），本文件是投影。全局约定（FK 真相 / 向量维度 / 枚举 / 唯一与 CHECK 约束）见 [_conventions.md](_conventions.md)；完整表清单与导航见 [_index.md](../_index.md)。


### 2.1 scheduler_tasks（调度任务表）

| 字段名 | 类型 | 约束/默认/索引 | 用途 |
| -------- | ------ | ------ | ------ |
| `id` | SERIAL | PK | 主键 |
| `name` | VARCHAR(50) | UNIQUE NOT NULL; index | 任务名称 |
| `description` | VARCHAR(200) | — | 任务描述 |
| `check_interval` | INTEGER | DEFAULT 60 NOT NULL | 检查间隔（秒） |
| `last_execution_time` | TIMESTAMP | — | 上次执行时间 |
| `next_execution_time` | TIMESTAMP | — | 下次执行时间 |
| `status` | VARCHAR(20) | DEFAULT 'idle'; index | 状态 |
| `last_error` | TEXT | — | 最近错误 |
| `last_error_time` | TIMESTAMP | — | 最近错误时间 |
| `total_executions` | INTEGER | DEFAULT 0 | 总执行次数 |
| `successful_executions` | INTEGER | DEFAULT 0 | 成功次数 |
| `failed_executions` | INTEGER | DEFAULT 0 | 失败次数 |
| `consecutive_failures` | INTEGER | DEFAULT 0 | 连续失败次数 |
| `last_execution_duration` | FLOAT | —（`*float64`，可空） | 上次执行耗时（秒） |
| `last_execution_result` | TEXT | — | 上次执行结果 |
| `created_at` | TIMESTAMP | — | 创建时间 |
| `updated_at` | TIMESTAMP | — | 更新时间 |

| 任务名（示例） | 描述 | 执行间隔 |
| -------- | ------ | ---------- |
| `auto_refresh` | 自动刷新 RSS 订阅源 | 60 秒 |
| `ai_summary` | AI 智能总结文章内容（基于 Firecrawl） | 3600 秒 |

### 2.2 ai_settings（AI 配置键值对）

通用键值对表，承载大量运行时配置 seed（如 `semantic_board_match_*`、`persistent_topic_*`、`event_cluster_*`、`daily_report_time`、`auxiliary_label_dedupe_sim` 等）。具体键由各业务域代码读写，非表结构约束。

| 字段名 | 类型 | 约束/默认/索引 | 用途 |
| -------- | ------ | ------ | ------ |
| `id` | SERIAL | PK | 主键 |
| `key` | VARCHAR(100) | UNIQUE NOT NULL; index | 配置键 |
| `value` | TEXT | — | JSON / 字符串值 |
| `description` | VARCHAR(200) | — | 说明 |
| `created_at` | TIMESTAMP | — | 创建时间 |
| `updated_at` | TIMESTAMP | — | 更新时间 |

---


## 域 ER 图

（任务 3 嵌入）
