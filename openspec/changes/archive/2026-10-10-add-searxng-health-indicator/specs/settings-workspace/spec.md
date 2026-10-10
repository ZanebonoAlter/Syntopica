## ADDED Requirements

### Requirement: 数据源连通性探测

设置工作台「数据源与网络」分区的 SearXNG 配置 SHALL 提供连通性主动探测（`GET /api/settings/searxng/health`）：复用问答联网链路同一配置解析（`searxng_config` DB > env > config.yaml，现读现生效）与同一搜索客户端，发起一次真实探针搜索后返回 `{status, latency_ms, detail}`。status 枚举：`disabled`（未配置/未启用）｜`unreachable`（endpoint 连接失败或超时）｜`degraded`（HTTP 可达但搜索结果为空）｜`ok`（可达且结果非空）。探测 SHALL 为按需触发（面板操作），不引入后台轮询。

面板 SHALL 提供「测试连接」按钮展示探测结果四态标记（含耗时）；保存配置成功后 SHALL 自动触发一次探测。探测失败 SHALL NOT 阻塞配置保存。

#### Scenario: 点击测试连接获得四态结果

- **WHEN** 用户在 SearXNG 配置面板点击「测试连接」
- **THEN** 按钮进入加载态，探测完成后展示 status 对应的状态标记与耗时，不重复提交

#### Scenario: 引擎全挂时探测显示 degraded

- **WHEN** SearXNG 服务进程存活但上游引擎全部超时（results 为空）
- **THEN** 探测返回 `degraded`（而非 ok/unreachable），面板提示「服务可达但搜索无结果」

#### Scenario: 未配置时探测显示 disabled

- **WHEN** searxng_config 未配置或 enabled=false
- **THEN** 探测返回 `disabled`，面板显示中性提示，SHALL NOT 返回错误码

#### Scenario: 保存后自动探测

- **WHEN** 用户保存 SearXNG 配置成功
- **THEN** 面板自动执行一次连通性探测并展示结果；探测异常不影响保存成功的反馈
