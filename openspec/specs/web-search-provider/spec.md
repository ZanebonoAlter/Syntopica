# web-search-provider Specification

## Purpose

为需要「给定搜索词、取回原始网页结果」的业务（页边注问答联网参考等）提供统一、可探测、失败语义明确的网页搜索入口，屏蔽具体搜索服务商的协议差异，使搜索后端的更换不扩散到业务调用方。

## Requirements

### Requirement: 搜索请求与结果形状

系统 SHALL 提供统一的网页搜索入口：输入一个非空搜索词（trim 后 1~128 个 Unicode 字符），输出原始网页结果列表（顺序保留、URL 非空）。每条结果 SHALL 包含标题、可点击 URL、与搜索词相关的摘要文本；SHOULD 包含相关度分、来源站点名、发布日期、权威度等排序与可信度信号（服务商提供时透传，缺失时为零值）。结果 SHALL 为原始网页内容，SHALL NOT 包含服务商生成的 AI 总结段落。系统 SHALL NOT 对结果做域名白名单过滤，SHALL NOT 跨调用缓存结果。

#### Scenario: 正常搜索返回原始结果

- **WHEN** 以非空搜索词调用搜索入口
- **THEN** 返回该搜索词的原始网页结果列表，每条含标题、URL、摘要；按服务商返回顺序保留

#### Scenario: 空搜索词拒绝

- **WHEN** 搜索词 trim 后为空或超过 128 个字符
- **THEN** 不发起网络请求，返回「搜索词非法」错误

#### Scenario: 搜索正常但无匹配结果

- **WHEN** 服务商正常响应且结果数组为空
- **THEN** 返回空列表且不视为错误

### Requirement: 凭据来源与配置解析

搜索入口 SHALL 复用 AI 供应商注册表中已登记的 DeepSeek 供应商密钥：取 provider 为 OpenAI 兼容且 base_url 主机为 `api.deepseek.com` 的已启用供应商，以其 API Key 作为 Bearer 凭据；SHALL NOT 新增独立的密钥存储。解析 SHALL 每次调用现读（供应商配置变更即时生效）。无匹配供应商、供应商未启用或密钥为空时 SHALL 判定为「未配置」。

#### Scenario: 复用已登记供应商密钥

- **WHEN** AI 供应商注册表中存在已启用、已填密钥、base_url 为 `https://api.deepseek.com/v1` 的供应商
- **THEN** 搜索入口以该供应商密钥发起请求，无需任何额外配置项

#### Scenario: 未登记供应商视为未配置

- **WHEN** AI 供应商注册表中不存在 base_url 主机为 `api.deepseek.com` 的已启用供应商，或其密钥为空
- **THEN** 搜索入口不发起网络请求，返回「未配置」；调用方按未配置降级

### Requirement: 错误分类

搜索失败 SHALL 按下列类别返回可区分的错误，SHALL NOT 把不同类别合并为单一字符串：`not_configured`（未配置）｜`auth`（服务商返回 401/403 且非额度类）｜`quota`（服务商返回 402 或 403 且响应体表明余额/额度不足）｜`rate_limited`（429）｜`timeout`（超过调用截止时间）｜`unavailable`（网络不可达、5xx、响应体无法解析）。搜索词非法（空或超过 128 个字符）的本地拒绝 SHALL 返回 `invalid_query` 类错误且不发起网络请求。调用方可据此分类决定降级与用户提示。

#### Scenario: 鉴权失败归类为 auth

- **WHEN** 服务商返回 401
- **THEN** 返回 `auth` 类错误，错误信息含「鉴权失败」

#### Scenario: 额度不足归类为 quota

- **WHEN** 服务商返回 403 且响应体含余额/额度不足语义
- **THEN** 返回 `quota` 类错误，而非 `auth`

#### Scenario: 超时归类为 timeout

- **WHEN** 请求在截止时间前未返回
- **THEN** 返回 `timeout` 类错误

#### Scenario: 无结果不计费不报错

- **WHEN** 服务商返回 200 且 `results` 为空数组
- **THEN** 不返回错误，调用方拿到空列表

### Requirement: 连通性探测

系统 SHALL 提供按需触发的连通性探测：复用搜索入口同一凭据解析与同一客户端，以固定探针搜索词发起一次真实搜索，返回 `{status, latency_ms, detail, error_kind}`。status 四态：`disabled`（未配置）｜`unreachable`（`timeout`/`unavailable`）｜`degraded`（HTTP 可达但结果为空）｜`ok`（可达且结果非空）；`auth`/`quota`/`rate_limited` 归 `unreachable` 并在 `error_kind` 中给出具体类别。探测 SHALL NOT 走 AI 路由器、SHALL NOT 写入 AI 调用审计。SHALL NOT 引入后台轮询。

#### Scenario: 密钥失效时探测指向具体原因

- **WHEN** 供应商密钥无效，用户点击「测试连接」
- **THEN** 探测返回 `unreachable` 且 `error_kind=auth`，面板展示「鉴权失败」而非泛化的连接失败

#### Scenario: 余额耗尽时探测指向额度

- **WHEN** 账户余额或额度耗尽，用户点击「测试连接」
- **THEN** 探测返回 `unreachable` 且 `error_kind=quota`，面板展示「余额或额度不足」

#### Scenario: 探测异常不阻塞面板

- **WHEN** 探测返回 `unreachable` 且面板无独立保存动作（探测入口即操作）
- **THEN** 面板展示失败态与 `error_kind` 文案，不弹出阻断式错误、不自动重试
