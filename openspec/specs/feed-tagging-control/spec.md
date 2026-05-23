# Feed Tagging Control

## Purpose

TBD

## Requirements

### Requirement: Feed 模型 tagging_enabled 字段
Feed 模型 SHALL 包含 `tagging_enabled` bool 字段，默认 true。该字段控制该 feed 下的文章是否进入 tag 处理管线。

#### Scenario: 新建 feed 默认开启打标签
- **WHEN** 创建新 feed（未指定 tagging_enabled）
- **THEN** tagging_enabled 为 true

#### Scenario: 手动关闭打标签
- **WHEN** 用户通过设置将某 feed 的 tagging_enabled 设为 false
- **THEN** 该 feed 后续新文章不进入 tag 队列

### Requirement: enqueueArticleProcessing 尊重 tagging_enabled
`enqueueArticleProcessing` SHALL 检查 feed 的 `tagging_enabled` 字段。当 tagging_enabled 为 false 且 Firecrawl 未启用时，不将文章入队到 tag 队列。

#### Scenario: firecrawl 关闭 + tagging 开启
- **WHEN** feed.FirecrawlEnabled = false 且 feed.TaggingEnabled = true
- **THEN** 文章直接入 tag 队列

#### Scenario: firecrawl 关闭 + tagging 关闭
- **WHEN** feed.FirecrawlEnabled = false 且 feed.TaggingEnabled = false
- **THEN** 不入任何队列，文章仅入库

#### Scenario: firecrawl 开启 + tagging 开启
- **WHEN** feed.FirecrawlEnabled = true 且 feed.TaggingEnabled = true
- **THEN** 文章入 Firecrawl 队列，完成后自动触发 tag

#### Scenario: firecrawl 开启 + tagging 关闭
- **WHEN** feed.FirecrawlEnabled = true 且 feed.TaggingEnabled = false
- **THEN** 文章入 Firecrawl 队列，完成后不触发 tag（只做内容补全）

### Requirement: Firecrawl 完成回调检查 tagging_enabled
Firecrawl 任务完成后的回调 SHALL 检查所属 feed 的 `tagging_enabled`。当 tagging_enabled 为 false 时，跳过 tag 队列入队。

#### Scenario: Firecrawl 完成后跳过 tag
- **WHEN** Firecrawl 完成抓取且 feed.TaggingEnabled = false
- **THEN** 不触发 tag 入队，仅继续后续内容补全流程

#### Scenario: Firecrawl 完成后正常 tag
- **WHEN** Firecrawl 完成抓取且 feed.TaggingEnabled = true
- **THEN** 正常入 tag 队列

### Requirement: Feed update API 支持 tagging_enabled
`PATCH /api/feeds/:id` SHALL 接受 `tagging_enabled` 字段并更新 feed 记录。

#### Scenario: 更新 tagging_enabled
- **WHEN** 发送 PATCH 请求 body 包含 `{"tagging_enabled": false}`
- **THEN** 对应 feed 的 tagging_enabled 更新为 false
