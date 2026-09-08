# 链路追踪域（`tables/tracing.md`）

> 真相源 = 代码（GORM struct + `postgres_migrations.go`），本文件是投影。全局约定（FK 真相 / 向量维度 / 枚举 / 唯一与 CHECK 约束）见 [_conventions.md](_conventions.md)；完整表清单与导航见 [_index.md](../_index.md)。


### 12.1 otel_spans（OpenTelemetry 链路追踪）

存储 GORM Span Exporter 导出的 OpenTelemetry span 数据，通过独立 `EnsureTracingTable` AutoMigrate 创建。

| 字段名 | 类型 | 约束/默认/索引 | 用途 |
| -------- | ------ | ------ | ------ |
| `id` | BIGSERIAL | PK autoIncrement | 主键 |
| `trace_id` | CHAR(32) | NOT NULL | 追踪 ID（索引 `idx_otel_spans_trace_id` 已删，2026-08-20，零使用；偶发按 trace_id 排障退化为顺序扫） |
| `span_id` | CHAR(16) | NOT NULL | Span ID |
| `parent_span_id` | CHAR(16) | DEFAULT '' | 父 Span ID |
| `trace_state` | TEXT | DEFAULT '' | W3C trace state |
| `name` | VARCHAR(255) | NOT NULL | Span 名称（索引已删，2026-08-20） |
| `kind` | INTEGER | DEFAULT 1 | Span 类型（Internal=1, Server=2, Client=3, Producer=4, Consumer=5）（索引已删，2026-08-20） |
| `status_code` | INTEGER | DEFAULT 0 | 状态码（0=Unset, 1=Error, 2=OK）（索引已删，2026-08-20） |
| `status_message` | TEXT | DEFAULT '' | 状态信息 |
| `start_time_unix_nano` | BIGINT | NOT NULL; index `idx_otel_spans_start_time` | 开始时间（Unix 纳秒） |
| `end_time_unix_nano` | BIGINT | NOT NULL | 结束时间（Unix 纳秒） |
| `duration_ms` | BIGINT | DEFAULT 0 | 持续时间（毫秒） |
| `service_name` | VARCHAR(100) | DEFAULT 'syntopica' | 服务名称 |
| `service_version` | VARCHAR(50) | DEFAULT '' | 服务版本 |
| `resource_attributes` | TEXT | DEFAULT '{}' | 资源属性（JSON） |
| `scope_name` | VARCHAR(100) | DEFAULT '' | Scope 名称 |
| `scope_version` | VARCHAR(50) | DEFAULT '' | Scope 版本 |
| `attributes` | TEXT | DEFAULT '{}' | Span 属性（JSON） |
| `events` | TEXT | DEFAULT '[]' | Span 事件（JSON） |
| `links` | TEXT | DEFAULT '[]' | Span 链接（JSON） |
| `created_at` | TIMESTAMP | — | 创建时间 |

---


## 域 ER 图

`otel_spans` 为独立写入的追踪数据表，与业务表无关联边（无 FK、无 GORM 关联），故无域 ER 图。
