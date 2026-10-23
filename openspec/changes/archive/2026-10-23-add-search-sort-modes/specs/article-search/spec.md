## MODIFIED Requirements

### Requirement: 混合搜索聚合端点

`GET /api/search` SHALL 接收查询参数 `q`、`archived`（include|active-only，默认 include）、`expand`（off|on，默认 off）、`sort_by`（date|relevance，默认 date）、`page`/`per_page`，执行关键词路径与语义路径召回，按 `sort_by` 指定的顺序返回分页结果；每个结果项 MUST 携带命中解释 `hit_reasons`（type: keyword|semantic|expand，含命中文本与分数）；响应 MUST 携带 `semantic` 块（命中标签列表或 `skipped: true`），并在 `data` 顶层回显 `sortBy`（实际生效值）。首屏延迟目标 P50 < 1s（语义路径并行执行，不阻塞关键词结果）。

#### Scenario: 关键词路径命中并返回解释
- **WHEN** 客户端请求 `GET /api/search?q=国漫崛起`
- **THEN** 返回结果中标题含「国漫」的文章，其 `hit_reasons` 含 type=keyword 项，且不依赖语义路径可用性

#### Scenario: 语义路径命中返回标签与分数
- **WHEN** 查询命中语义路径（query embedding 与标签向量 cosine 达标）
- **THEN** 结果项 `hit_reasons` 含 type=semantic 项（标签名与 cosine 分数），响应 `semantic` 块列出命中标签

#### Scenario: 非法分页参数
- **WHEN** 客户端请求 `per_page=0` 或 `page=-1`
- **THEN** 返回 400 错误，不执行搜索

#### Scenario: 未传排序参数默认时间优先
- **WHEN** 客户端请求 `GET /api/search?q=英伟达` 且不带 `sort_by`
- **THEN** 响应 `sortBy=date`，结果按发布日倒序分组展示

#### Scenario: 相关度排序模式
- **WHEN** 客户端请求 `GET /api/search?q=英伟达&sort_by=relevance`
- **THEN** 响应 `sortBy=relevance`，结果按现有 RRF 融合相关度排序

### Requirement: 搜索页入口与状态

侧边栏「发现订阅源」之后 SHALL 新增「文章搜索」入口，指向新页面 `/search`；页面 SHALL 使用 contained 布局模式（≤1120px 居中），editorial/dark 双主题零特判；SHALL 呈现完整状态矩阵：初始空态（未搜索引导）、加载骨架屏、无结果（含「开启 LLM 扩展搜索再试」引导，一键开启并重查）、错误（重试按钮，保留输入）、语义降级提示条（不阻塞）。LLM 扩展开关默认关闭。过滤行 SHALL 提供排序切换「最新优先（默认）｜相关度优先」，切换后 SHALL 按当前查询立即重查，加载更多 SHALL 沿用当前排序；首查加载中排序控件 SHALL disabled。

#### Scenario: 无结果引导扩展
- **WHEN** 三路召回均零结果且 expand=off
- **THEN** 展示空态与扩展引导按钮；点击后开启扩展并自动重查

#### Scenario: 语义降级提示
- **WHEN** 响应含 `semantic.skipped=true`
- **THEN** 页面顶部显示降级提示条，关键词结果正常展示，无错误弹窗

#### Scenario: 双主题渲染
- **WHEN** 用户在 editorial 与 dark 主题间切换
- **THEN** 搜索页与结果卡片样式随语义 token 切换，无未适配区块

#### Scenario: 默认最新优先
- **WHEN** 用户首次进入搜索页并提交查询
- **THEN** 排序控件「最新优先」高亮，请求携带 `sort_by=date`

#### Scenario: 切换排序立即重查
- **WHEN** 搜索已有结果时用户点击「相关度优先」
- **THEN** 页面以同一查询、`sort_by=relevance` 重新请求并替换结果，加载更多沿用该排序

## ADDED Requirements

### Requirement: 搜索结果排序模式

`GET /api/search` 的结果排序 SHALL 由 `sort_by` 控制，支持 `date`（默认）与 `relevance` 两种模式；两种模式 MUST 返回相同的结果集与 `total`（只改变顺序，不做过滤）。

`date` 模式 MUST：以文章 `pub_date`（为空时回退 `created_at`）在 Asia/Shanghai（UTC+8）的日历日倒序分组；同一天内按现有融合相关度（RRF 分数 → 关键词分 → 语义分）倒序；同日同分时以精确发布时间倒序、文章 ID 升序兜底，保证跨页稳定。`relevance` 模式 MUST 保持现有 RRF 融合相关度排序（RRF 分数 → 关键词分 → 语义分 → 文章 ID 升序）。`sort_by` 非法值 MUST 静默回退 `date`（HTTP 200，不报错）。

#### Scenario: 日界按 Asia/Shanghai 划分
- **WHEN** 两篇命中文章分别发布于 2026-10-22T23:59:00+08:00 与 2026-10-23T00:01:00+08:00，`sort_by=date`
- **THEN** 10-23 那篇排在 10-22 那篇之前（不同日分组）

#### Scenario: 同一天内按相关度排序
- **WHEN** 同一天有两篇命中文章，A 的 RRF 融合分高于 B
- **THEN** A 排在 B 之前，而不是按精确发布时间排序

#### Scenario: 空发布时间回退创建时间
- **WHEN** 命中文章的 `pub_date` 为空
- **THEN** 使用 `created_at` 参与日期分组与排序，不因空值排到异常位置

#### Scenario: 非法排序值回退默认
- **WHEN** 客户端请求 `sort_by=bogus`
- **THEN** 返回 HTTP 200，`sortBy=date`，按时间优先排序

#### Scenario: 分页顺序稳定
- **WHEN** 同一查询以相同 `sort_by` 连续请求第 1 页与第 2 页
- **THEN** 两页结果无重复、无遗漏（排序键全序确定）
