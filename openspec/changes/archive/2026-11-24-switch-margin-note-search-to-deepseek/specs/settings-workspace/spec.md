## MODIFIED Requirements

### Requirement: 数据源连通性探测

设置工作台「数据源与网络」分区的网页搜索配置 SHALL 提供连通性主动探测（`GET /api/settings/web-search/health`）：复用问答联网链路同一凭据解析（AI 供应商注册表中 DeepSeek 供应商密钥，现读现生效）与同一搜索客户端，发起一次真实探针搜索后返回 `{status, latency_ms, detail, error_kind}`。status 枚举：`disabled`（未配置）｜`unreachable`（含超时、网络不可达、鉴权失败、额度不足、限速，具体类别见 `error_kind`）｜`degraded`（HTTP 可达但搜索结果为空）｜`ok`（可达且结果非空）。探测 SHALL 为按需触发（面板操作），不引入后台轮询。

面板 SHALL 提供「测试连接」按钮展示探测结果四态标记（含耗时），并按 `error_kind` 展示针对性文案（鉴权失败 / 余额或额度不足 / 请求超时 / 搜索服务暂不可用）。SHALL NOT 提供可编辑的 endpoint 或密钥字段——密钥由 AI 模型的供应商配置统一管理，面板 SHALL 显示「已关联供应商 / 未关联」状态并在未关联时给出前往 AI 模型的指引。探测失败 SHALL NOT 阻塞任何操作。

原 SearXNG 配置与探测 SHALL 随本变更整体退役：`searxng_config` 配置键、可编辑的 endpoint / enabled 字段、`SEARXNG_URL` 环境变量与 `config.yaml` 的 `searxng.endpoint` 均不再被任何面板或接口读写；`GET|POST /api/settings/searxng` 与 `GET /api/settings/searxng/health` 路由下线（调用方得到 404），由上述两个 GET 路由承接。旧深链 `tab=searxng` 重定向到 `tab=web-search`。

#### Scenario: 点击测试连接获得四态结果

- **WHEN** 用户在网页搜索配置面板点击「测试连接」
- **THEN** 按钮进入加载态，探测完成后展示 status 对应的状态标记与耗时，不重复提交

#### Scenario: 引擎全挂时探测显示 degraded

- **WHEN** 搜索服务可达但探针搜索返回空结果（服务商侧无匹配）
- **THEN** 探测返回 `degraded`（而非 ok/unreachable），面板提示「服务可达但搜索无结果」

#### Scenario: 未配置时探测显示 disabled

- **WHEN** AI 供应商注册表中无可用 DeepSeek 供应商密钥
- **THEN** 探测返回 `disabled`，面板显示中性提示与前往 AI 模型的指引，SHALL NOT 返回错误码

#### Scenario: 密钥失效时探测指向鉴权

- **WHEN** 供应商密钥无效，用户点击「测试连接」
- **THEN** 探测返回 `unreachable` 且 `error_kind=auth`，面板展示「鉴权失败」

#### Scenario: 保存后自动探测

- **WHEN** 用户在网页搜索面板触发探测（无独立保存动作，探测入口即操作）
- **THEN** 面板执行一次连通性探测并展示结果；探测异常不弹出阻断式错误

### Requirement: 设置导航分组与复合 section

设置导航 SHALL 由四个分组标题与组内导航项构成：内容管理（订阅源/兴趣画像/页边注）、AI 配置（AI 模型/能力路由）、数据源与网络（1 个复合 section）、运行状态（1 个复合 section）。分组标题 SHALL 不可点击。

#### Scenario: 分组导航渲染

- **WHEN** 用户打开设置工作区
- **THEN** 侧栏按「内容管理 / AI 配置 / 数据源与网络 / 运行状态」四个分组标题渲染全部 7 个导航项

#### Scenario: 复合 section 子 tab 切换与 URL 承载

- **WHEN** 用户在「数据源与网络」内从 Firecrawl 切到出站代理子 tab
- **THEN** URL 查询参数更新为 `section=datasources-network&tab=proxy`，刷新后仍停留在该子 tab

#### Scenario: 旧 section 键深链重定向

- **GIVEN** URL 指向旧键 `?section=proxy`（或 firecrawl/bocha/rsshub/datasources/ai-health/queues/schedulers 任一）
- **WHEN** 页面加载
- **THEN** 工作区 SHALL 重定向到对应新键（`datasources-network` 或 `runtime-status`）并携带 `tab=<旧键>`，不出现 404 或空白

#### Scenario: 已下线面板的旧键重定向到替代 tab

- **GIVEN** URL 指向 `?section=searxng`，或 `?section=datasources-network&tab=searxng`（SearXNG 面板已下线，由「网页搜索」tab 承接）
- **WHEN** 页面加载
- **THEN** 工作区 SHALL 重定向到 `section=datasources-network&tab=web-search`，不出现 404、空白或落回默认子 tab；非「数据源与网络」分区携带 `tab=searxng` 时 SHALL NOT 触发该 tab 级重定向

#### Scenario: 非法 tab 值回退

- **GIVEN** URL 含 `section=datasources-network&tab=不存在值`
- **WHEN** 页面加载
- **THEN** 回退到该复合 section 的默认子 tab（Firecrawl）并清除非法 tab 参数

#### Scenario: 复合 section 进入默认子 tab

- **WHEN** 用户分别进入「数据源与网络」与「运行状态」
- **THEN** 默认分别落在 Firecrawl 与 AI 健康子 tab，不跨 section 记忆上次子 tab
