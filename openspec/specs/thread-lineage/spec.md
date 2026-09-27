# thread-lineage Specification

## Purpose

**DEPRECATED** — Thread 级别血统追踪已移除。Lineage 追踪上移到 Section 级关系表（参见 section-relations）。
- `prev_thread_id` 赋值已移除
- 线程血统链查询 API（GET /api/daily-reports/threads/:id/lineage）已移除
- 板块线程时间线 API（GET /api/semantic-boards/:id/thread-timeline）已移除
- ThreadLineagePanel 组件已移除
- BoardThreadBrowser 已改造为叙事级 DAG 时间线（参见 section-lifecycle）

## Requirements

### Requirement: Thread lineage 已退役

系统 SHALL NOT 提供 Thread 级别血统追踪能力；血统语义 SHALL 由 section-relations（Section 级关系）承担。

#### Scenario: lineage API 不存在

- **WHEN** 客户端请求 `GET /api/daily-reports/threads/:id/lineage`
- **THEN** 该路由 SHALL 不存在（404）

#### Scenario: thread-timeline API 不存在

- **WHEN** 客户端请求 `GET /api/semantic-boards/:id/thread-timeline`
- **THEN** 该路由 SHALL 不存在（404）
