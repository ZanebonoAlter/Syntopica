# AI Router 统一配置与多路由降级设计

**日期:** 2026-03-12
**状态:** 已确认方案 A

---

## 1. 目标

把当前分散在 SQLite `summary_config`、前端 `localStorage`、以及多个业务模块中的 AI 配置统一收口到一个可扩展的 AI Router 层。

本次设计解决四个核心问题：

1. 支持配置多个 AI provider，并按优先级自动失败切换。
2. 支持按能力路由，不同能力可绑定不同 provider 链路。
3. 降低业务模块与底层模型配置的耦合。
4. 为后续扩展新的模型、供应商、日志与可观测能力留出结构空间。

---

## 2. 当前问题

当前实现存在以下结构性问题：

### 2.1 配置耦合过高

- `summary_config` 同时承载总结主模型、自动总结、正文补全、topic tag、Firecrawl 嵌套配置。
- `firecrawl` 作为独立外部服务，被塞进 AI 总配置 JSON，职责边界不清晰。
- 多个模块直接读取同一份 JSON，并自行解析 `base_url/api_key/model`。

### 2.2 配置源不唯一

- 后端使用 SQLite 中的 `summary_config`。
- 前端仍在 `localStorage` 中保存并读取 `aiSettings`。
- 手动总结、队列总结、连接测试等接口还要求前端重复传入模型配置。

### 2.3 调用方式重复且不可扩展

- `auto_summary`、`summary_queue`、`topicgraph/tagger`、`content_completion` 各自拼接 OpenAI-like 请求。
- 没有统一 provider adapter，未来接入新的兼容接口会继续复制逻辑。

### 2.4 没有失败切换能力

- 当前所有能力本质上只有一个 provider 配置。
- 请求失败后只能整体失败，无法自动切换备用模型。

---

## 3. 设计原则

### 3.1 能力与模型解耦

业务层只声明“我要做什么”，例如 `summary`、`article_completion`、`topic_tagging`，不再关心具体走哪个模型。

### 3.2 配置与调用解耦

配置管理、provider 适配、失败切换、调用日志由 AI Router 统一负责，业务模块只构造 prompt 和消费结果。

### 3.3 默认简单，渐进增强

第一阶段只支持稳定的顺序失败切换 `ordered_failover`，不引入负载均衡、动态权重、复杂编排。

### 3.4 向后兼容

通过迁移脚本把旧 `summary_config` 导入新结构，保证升级后原有能力仍可用。

---

## 4. 目标架构

### 4.1 核心分层

```text
业务模块
  ├─ summaries
  ├─ jobs/auto_summary
  ├─ contentprocessing
  ├─ topicgraph
  └─ digest / open notebook

          ↓ 只传 capability + request

AI Router
  ├─ RouteResolver
  ├─ ProviderRegistry
  ├─ FailoverExecutor
  └─ CallLogger

          ↓

Provider Adapter
  ├─ openai_compatible
  └─ future providers

          ↓

SQLite Config Tables
```

### 4.2 关键能力标识

第一版建议支持以下 capability：

- `summary`：手动总结、批量总结、自动总结
- `article_completion`：正文补全与整理
- `topic_tagging`：主题与实体提取
- `digest_polish`：日报/周报润色与二次汇总
- `open_notebook`：如果该能力内需要模型调用，则经路由选择模型；其外部服务配置仍独立保存

---

## 5. 数据模型设计

### 5.1 `ai_providers`

存储单个 provider 节点。

建议字段：

```sql
CREATE TABLE ai_providers (
  id INTEGER PRIMARY KEY,
  name VARCHAR(100) NOT NULL,
  provider_type VARCHAR(50) NOT NULL DEFAULT 'openai_compatible',
  base_url VARCHAR(500) NOT NULL,
  api_key TEXT NOT NULL,
  model VARCHAR(100) NOT NULL,
  enabled BOOLEAN NOT NULL DEFAULT 1,
  timeout_seconds INTEGER NOT NULL DEFAULT 120,
  max_tokens INTEGER,
  temperature REAL,
  metadata TEXT,
  created_at DATETIME,
  updated_at DATETIME
);
```

说明：

- `provider_type` 第一版只需支持 `openai_compatible`。
- `metadata` 用 JSON 字符串承载额外扩展字段，避免过早设计过细。

### 5.2 `ai_routes`

存储能力路由定义。

```sql
CREATE TABLE ai_routes (
  id INTEGER PRIMARY KEY,
  name VARCHAR(100) NOT NULL,
  capability VARCHAR(50) NOT NULL,
  enabled BOOLEAN NOT NULL DEFAULT 1,
  strategy VARCHAR(50) NOT NULL DEFAULT 'ordered_failover',
  description VARCHAR(255),
  created_at DATETIME,
  updated_at DATETIME
);
```

约束建议：

- `capability` + `name` 唯一。
- 每个 capability 第一版只允许存在一个 `enabled` 的默认 route，避免解析复杂度过高。

### 5.3 `ai_route_providers`

定义一条路由上的 provider 链。

```sql
CREATE TABLE ai_route_providers (
  id INTEGER PRIMARY KEY,
  route_id INTEGER NOT NULL,
  provider_id INTEGER NOT NULL,
  priority INTEGER NOT NULL DEFAULT 100,
  enabled BOOLEAN NOT NULL DEFAULT 1,
  created_at DATETIME,
  updated_at DATETIME
);
```

说明：

- `priority` 越小优先级越高。
- 同一个 provider 可被多个 capability 复用。

### 5.4 `ai_call_logs`（推荐）

用于记录调用与降级链路。

```sql
CREATE TABLE ai_call_logs (
  id INTEGER PRIMARY KEY,
  capability VARCHAR(50) NOT NULL,
  route_name VARCHAR(100) NOT NULL,
  provider_name VARCHAR(100) NOT NULL,
  success BOOLEAN NOT NULL,
  is_fallback BOOLEAN NOT NULL DEFAULT 0,
  latency_ms INTEGER,
  error_code VARCHAR(100),
  error_message TEXT,
  request_meta TEXT,
  created_at DATETIME
);
```

该表不是功能必需，但对排查“为什么切到备用模型”非常重要。

---

## 6. 运行时设计

### 6.1 统一请求接口

建议定义统一 Router 输入：

```go
type Capability string

const (
    CapabilitySummary           Capability = "summary"
    CapabilityArticleCompletion Capability = "article_completion"
    CapabilityTopicTagging      Capability = "topic_tagging"
    CapabilityDigestPolish      Capability = "digest_polish"
    CapabilityOpenNotebook      Capability = "open_notebook"
)

type ChatRequest struct {
    Capability  Capability
    Messages    []Message
    Temperature *float64
    MaxTokens   *int
    Metadata    map[string]any
}
```

业务层调用时只传 capability 和消息体。

### 6.2 Router 执行流程

```text
接收 capability
  -> 查找启用中的 route
  -> 读取 route 绑定的 provider 列表
  -> 按 priority 顺序调用
  -> 成功则直接返回
  -> 失败则记录日志并切下一个 provider
  -> 全部失败则返回聚合错误
```

### 6.3 错误策略

第一版只实现以下行为：

- 网络错误：切备用
- 5xx / rate limit：切备用
- provider 明确返回业务错误：切备用
- 配置错误：不切备用，直接报配置问题
- prompt 构造错误：不切备用，直接返回业务层错误

### 6.4 结果结构

建议 Router 返回：

```go
type ChatResult struct {
    Content       string
    ProviderID    uint
    ProviderName  string
    RouteName     string
    UsedFallback  bool
    AttemptCount  int
}
```

这样上层既能得到内容，也能在日志或任务状态里显示是否发生降级。

---

## 7. 模块改造边界

### 7.1 `summaries`

- 手动总结、队列总结、自动总结统一使用 `summary` capability。
- handler 不再接收 `base_url/api_key/model`。
- 业务层只负责构造 prompt 与保存结果。

### 7.2 `contentprocessing`

- `content_completion_handler` 不再从 `summary_config` 读取模型。
- `ContentCompletionService` 通过 `article_completion` capability 获取可用模型。

### 7.3 `topicgraph`

- `tagger.go` 不再解析 `LoadSummaryConfig()`。
- 统一通过 `topic_tagging` capability 调用 AI；失败时仍可保留 heuristic 兜底。

### 7.4 `digest / open notebook`

- 外部服务地址、notebook 名称、是否自动发送等仍作为独立业务配置保存。
- 仅当其内部需要 AI 润色时，走 `digest_polish` 或 `open_notebook` capability。

### 7.5 `firecrawl`

- `firecrawl` 配置彻底从 `summary_config` 中拆出。
- 作为独立服务配置存在，不进入 AI Router 的 provider/route 模型。

---

## 8. API 设计

### 8.1 新增后端接口

建议新增：

- `GET /api/ai/providers`
- `POST /api/ai/providers`
- `PUT /api/ai/providers/:id`
- `DELETE /api/ai/providers/:id`
- `GET /api/ai/routes`
- `PUT /api/ai/routes/:capability`
- `POST /api/ai/routes/:capability/test`
- `GET /api/ai/routes/:capability/health`

### 8.2 兼容旧接口

短期保留：

- `GET /api/ai/settings`
- `POST /api/ai/settings`
- `POST /api/ai/test`
- `POST /api/auto-summary/config`

但这些旧接口内部应转为读写新的 provider/route 结构，并逐步标记为 deprecated。

### 8.3 业务接口调整

以下接口请求体应去掉显式模型字段：

- `/api/ai/summarize`
- `/api/summaries/generate`
- `/api/summaries/queue`
- `/api/auto-summary/config`（最终废弃）

改造后由后端根据 capability 自行解析 route。

---

## 9. 前端设计

### 9.1 设置界面重构

当前“一个 base_url + api_key + model”需要改成两块：

1. Provider 管理
   - 新增 provider
   - 编辑 provider
   - 启停 provider
   - 测试 provider 连接

2. Route 管理
   - 按 capability 展示当前路由
   - 配置主 provider / 备用 provider 顺序
   - 显示当前默认策略 `ordered_failover`

### 9.2 前端状态源统一

- 前端不再把 AI 主配置长期存到 `localStorage`。
- 页面初始化从后端 API 获取 provider/route 配置。
- 仅允许少量临时 UI 状态保留在本地，不再缓存密钥主数据。

### 9.3 用户体验建议

对每个 capability 显示：

- 主用模型
- 备用模型数量
- 上次测试结果
- 最近是否发生过 fallback

---

## 10. 迁移方案

### 10.1 数据迁移

从旧 `summary_config` 中迁移出：

- 一个默认 provider，例如 `default-primary`
- 一组默认 route：
  - `summary`
  - `article_completion`
  - `topic_tagging`
  - `digest_polish`

以上 route 初始都只绑定该 provider。

### 10.2 Firecrawl 迁移

如果旧 `summary_config.firecrawl` 存在：

- 拆到新的 Firecrawl 独立配置表或独立 key 中
- 原位置保留只读兼容一段时间，迁移后不再写回旧字段

### 10.3 兼容窗口

升级后的一个过渡版本内：

- 旧读取逻辑只用于迁移兜底，不再作为主路径
- 新保存逻辑只写新表，不再写回 `summary_config`

---

## 11. 风险与控制

### 11.1 风险

- 改造涉及多个调用链，容易出现遗漏
- 旧前端仍传模型字段时，前后端可能出现双配置不一致
- provider 失败切换后，部分任务日志需要补充上下文才能排查

### 11.2 控制措施

- 优先统一后端路由层，再逐步清理旧前端字段
- 为每个 capability 增加最小集成测试
- 引入 `ai_call_logs` 或至少结构化日志
- 保留旧接口兼容层，避免一次性切断

---

## 12. 分阶段落地建议

### Phase 1: 底座搭建

- 新建 provider/route 数据表
- 实现 AI Router 与 openai-compatible adapter
- 完成旧配置迁移

### Phase 2: 后端切流

- summaries 切到 `summary`
- auto_summary 切到 `summary`
- content_completion 切到 `article_completion`
- topicgraph 切到 `topic_tagging`

### Phase 3: 前端改造

- 设置页改为 provider + route 管理
- 业务请求去掉显式模型配置
- 清理 `localStorage` 主配置逻辑

### Phase 4: 收尾

- Firecrawl 完全拆出
- 旧接口标记废弃
- 增加调用日志和健康检查

---

## 13. 成功标准

- 可配置多个 provider，并为每个 capability 绑定主备顺序
- 任一 provider 失败时可自动切换备用 provider
- `summary`、`article_completion`、`topic_tagging`、`digest` 链路都不再直接读取 `summary_config`
- 前端业务接口不再显式上传 `base_url/api_key/model`
- Firecrawl 从 AI 配置中彻底拆出
- 新增 provider 或更换模型时，无需修改业务代码
