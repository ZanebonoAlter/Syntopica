## Purpose

feed 级 AI 总结触发链路的字段语义契约：`article_summary_enabled`（总结能力主开关，含手动总结可用性）与 `completion_on_refresh`（刷新后自动总结闸门）的组合行为、默认值与存量迁移，覆盖 firecrawl 与非 firecrawl 双路径。

## ADDED Requirements

### Requirement: 总结双层开关组合语义

feed 的总结行为 SHALL 由两层开关组合决定：`article_summary_enabled` 为能力主开关（关闭时无任何总结能力，含手动入口），`completion_on_refresh` 为自动触发闸门（仅控制刷新后是否自动总结）。组合语义：主开关关闭 → 无总结；主开关开启 + 自动闸门关闭 → 刷新不自动总结，但总结能力可用（手动生成入口保留）；主开关开启 + 自动闸门开启 → 刷新后自动排队总结。全文抓取（`firecrawl_enabled`）SHALL 与总结开关完全独立，关闭总结 SHALL NOT 影响全文抓取。

#### Scenario: 手动模式
- **GIVEN** 某 feed `article_summary_enabled = true` 且 `completion_on_refresh = false`
- **WHEN** 该 feed 刷新拉入新文章
- **THEN** 新文章不进入自动总结队列
- **AND** 该 feed 的文章在文章页保留手动生成总结入口，手动触发后正常产出 AI 整理稿

#### Scenario: 关闭总结不影响抓取
- **GIVEN** 某 feed `firecrawl_enabled = true` 且 `article_summary_enabled = false`
- **WHEN** 该 feed 刷新拉入新文章
- **THEN** 全文抓取正常入队并完成，文章正文可读
- **AND** 不产生任何总结请求

#### Scenario: 完全关闭
- **GIVEN** 某 feed `article_summary_enabled = false`
- **WHEN** 用户查看该 feed 任一文章
- **THEN** 不显示手动总结入口

### Requirement: firecrawl 路径自动总结闸门

对 `firecrawl_enabled = true` 的 feed，全文抓取完成后系统 SHALL 仅在该 feed 同时满足 `article_summary_enabled = true` 且 `completion_on_refresh = true` 时，将文章标记为待总结并进入自动总结流程；否则文章总结状态 SHALL 保持"无需总结"。

#### Scenario: 双开时抓取完成进入总结
- **GIVEN** 某 feed `firecrawl_enabled = true`、`article_summary_enabled = true`、`completion_on_refresh = true`
- **WHEN** 该文章全文抓取完成
- **THEN** 文章被标记为待总结并随后由 AI 生成整理稿

#### Scenario: 自动闸门关闭时抓取完成不总结
- **GIVEN** 某 feed `firecrawl_enabled = true`、`article_summary_enabled = true`、`completion_on_refresh = false`
- **WHEN** 该文章全文抓取完成
- **THEN** 文章总结状态保持"无需总结"，不进入总结队列

### Requirement: 非 firecrawl 路径自动总结闸门

对 `firecrawl_enabled = false` 的 feed，刷新拉入新文章时系统 SHALL 仅在该 feed 同时满足 `article_summary_enabled = true` 且 `completion_on_refresh = true` 时，将文章标记为待总结；否则文章总结状态 SHALL 保持"无需总结"。

#### Scenario: 非 firecrawl 双开时入库即待总结
- **GIVEN** 某 feed `firecrawl_enabled = false`、`article_summary_enabled = true`、`completion_on_refresh = true`
- **WHEN** 刷新拉入该 feed 的一篇新文章
- **THEN** 文章被标记为待总结并进入自动总结流程（素材为 RSS 自带内容）

#### Scenario: 非 firecrawl 自动闸门关闭时入库不总结
- **GIVEN** 某 feed `firecrawl_enabled = false`、`article_summary_enabled = true`、`completion_on_refresh = false`
- **WHEN** 刷新拉入该 feed 的一篇新文章
- **THEN** 文章总结状态保持"无需总结"

### Requirement: 自动总结调度扫描闸门

自动总结调度扫描待处理文章时 SHALL 仅捞取同时满足 `article_summary_enabled = true` 且 `completion_on_refresh = true` 的 feed 的待总结文章；主开关或自动闸门任一关闭的 feed 的文章 SHALL NOT 被调度扫描消费。

#### Scenario: 关闭自动闸门后积压文章不再消费
- **GIVEN** 某 feed 存在历史遗留的待总结文章，且该 feed `completion_on_refresh` 已被置为 `false`
- **WHEN** 自动总结调度器触发扫描
- **THEN** 该 feed 的待总结文章不被捞起处理

### Requirement: 自动总结闸门默认关闭

新建 feed 时 `completion_on_refresh` SHALL 默认为 `false`；后端模型默认值与前端创建路径缺省回退 MUST 一致为 `false`，各 UI 入口 MUST NOT 出现相互矛盾的回退默认。

#### Scenario: 新建 feed 不传开关字段
- **WHEN** 创建 feed 请求未携带 `completion_on_refresh` 字段
- **THEN** 新建 feed 的 `completion_on_refresh` 为 `false`

#### Scenario: 前端数据缺省回退
- **WHEN** 前端接收的 feed 数据中 `completion_on_refresh` 缺失或为 null
- **THEN** 前端按 `false` 处理

### Requirement: 存量 feed 自动总结一次性关闭

本变更部署时 SHALL 执行一次性数据迁移，将所有存量 feed 的 `completion_on_refresh` 置为 `false`；迁移 MUST 幂等（重复执行无副作用）。

#### Scenario: 部署后存量 feed 自动总结全部关闭
- **WHEN** 本变更部署完成
- **THEN** 所有存量 feed 的 `completion_on_refresh = false`，刷新不再自动触发总结
- **AND** 已生成的 AI 整理稿保留不动
