# 研究数据源域（`tables/research-data-sources.md`）

> 真相源 = 代码（GORM struct：`backend-go/internal/datasources/model.go`），本文件是投影。全局约定见 [_conventions.md](_conventions.md)；导航见 [_index.md](../_index.md)。

本域 1 张表，由 `internal/datasources` 包 `init()` 经 `RegisterModels` 注册；启动时幂等 upsert seed（静态列快照 + status 重算；运行时列 last_probe_at/last_error 由外部写入，seed 不触碰）。

### data_sources（研究数据源目录）

| 字段名 | 类型 | 约束/默认/索引 | 用途 |
| -------- | ------ | ------ | ------ |
| `code` | VARCHAR(32) | PK | 源标识（eia_wpsr / jodi_oil_primary / wb_wdi / un_comtrade） |
| `name` | VARCHAR(128) | — | 名称 |
| `provider` | VARCHAR(128) | — | 提供方 |
| `homepage_url` | VARCHAR(256) | — | 官方入口 |
| `coverage` | VARCHAR(256) | — | 覆盖范围描述 |
| `topics` | VARCHAR(256) | — | 主题标签 JSON 数组（目录 API 解析为数组返回） |
| `frequency` | VARCHAR(16) | — | 频度（weekly/monthly/annual…） |
| `typical_lag` | VARCHAR(128) | — | 典型滞后描述 |
| `unit_policy` | VARCHAR(256) | — | 单位纪律（不换算口径） |
| `requires_key` | BOOLEAN | — | 是否需订阅 key |
| `config_key_name` | VARCHAR(64) | — | key 的配置项名（requires_key 时非空，如 COMTRADE_API_KEY） |
| `status` | VARCHAR(16) | index | enabled / disabled（seed 时按 key 配置重算） |
| `status_reason` | VARCHAR(256) | — | disabled 原因（指向配置项） |
| `last_probe_at` | TIMESTAMPTZ | — | 最近 probe 时间（本域不写入，留给未来运维） |
| `last_error` | VARCHAR(512) | — | 最近错误记录（本域不写入） |

> 观测值**不落库**（spec「取数结果不落库」）：取数仅内存 TTL 缓存（900s），快照由消费方（研究对话会话的工具结果记录）承担。
