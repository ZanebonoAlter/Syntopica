# 用例设计 — slim-article-list-payload

> 测试单元 = 一个 Requirement 的用户故事。本文件把 spec Scenario 串成完整故事，断言判据由主线程定；复杂档附白盒分支表与边界值。
> 故事锚点：**用户在远程（低带宽）链路上打开阅读页 → 列表加载 → 点选文章读正文 → 定时刷新回来列表仍稳。**

## 1. 主链路表（节拍）

| # | 步 / 动作 | 来源 Scenario | 期望 | 层 | 落点 |
|---|---|---|---|---|---|
| 1 | 打开应用，列表首屏发 `GET /api/articles?page=1&per_page=20` | `article-list-projection` / 列表响应体积上限 | 响应体（未压缩）< 100 KB；项内无 `content`/`description`/`firecrawl_content`；含 `excerpt` | Go handler | `backend-go/internal/reader/handler/article_handler_test.go`（新增断言） |
| 2 | 切换筛选（feed / category / watched_tags+relevance / concept / aux / archived） | 同 capability / 各筛选分支字段集一致 | 每个分支字段集合与无筛选一致 | Go handler | 同上（表驱动） |
| 3 | 列表行渲染 | `reading-list-panel` / 文章列表采用行式布局（回归） | 行仍渲染标题/时间/作者/已读/收藏/状态图标 | Vitest 组件 | `front/app/features/articles/components/ArticleCardView.test.ts`（回归跑） |
| 4 | 点选文章（详情未就绪的首帧） | `reading-article-pane` / 详情未就绪时的首帧导语 | 导语段以 `excerpt` 呈现；无空白占位、无布局跳动；**与正文重复的导语不给 excerpt → 首帧与终态一致（无导语）** | Vitest 组件 + Go handler | `front/app/features/articles/components/ArticleContentPreviewPanel.test.ts`（新增用例）+ `backend-go/internal/reader/handler/article_handler_test.go` |
| 5 | 详情返回 | `reading-article-pane` / 详情返回后以详情为准 + design D7 | 导语与正文取自详情；`displayContent` 非空 | opencli 端到端（静态 `:5100`） | `evidence/` 人工留痕（截图 + 断言输出） |
| 6 | 单 feed 定时刷新完成 | `refresh-parallelization` / 定时刷新后列表更新为按需重取 | 以当前筛选 + 当前页重取；`per_page ≤ 100`；选中行与滚动位置不变 | Vitest（composable） | `front/app/features/feeds/composables/useAutoRefresh.test.ts`（新增） |
| 7 | 23 个 feed 同周期到期 | `refresh-parallelization` / 多 feed 同周期 | 同一分钟至多 1 个刷新触发（fake timers 推进 60min） | Vitest（fake timers） | 同上 |
| 8 | 刷新请求失败 | `refresh-parallelization` / 刷新失败不终止调度 | 记录失败并继续调度后续 feed | Vitest | 同上 |
| 9 | 客户端请求 `per_page=10000` | `article-list-projection` / 客户端请求 10000 条 | 返回 100 条 + 服务端 WARN（含 `per_page=10000`） | Go handler + `curl` | `article_handler_test.go` + 验证节命令 |

**负向节拍（SHALL NOT）**：主链路中任何时刻都不得出现「`per_page > 100` 的文章列表请求」与「同分钟 20+ 并发刷新」——由节拍 6/7 的断言与实现后的日志/抓包核对覆盖（见 §4 效果核对）。

## 2. 变体走查

| 组 | 变体 | 答案 |
|---|---|---|
| 输入 | 空串 / 纯空白（含全角、tab） / 纯分隔符 / 单 token / 大小写 / 特殊字符 / 超长 | `excerpt` 边界值见 §5 白盒附加（逐条给期望） |
| 前置 | 空集（无文章） / 单元素 / 重复（同 link 不同 feed） / 越界引用（`?feed_id=999999`） / 部分满足（部分文章无 `description`） | 空集→`items: []` 且 `success: true`；单元素→投影一致；重复→按既有去重语义不变；越界→空列表不报错；部分满足→有源走源、无源 `excerpt: ""` |
| 时间窗口 | 边界两端（当天算不算）/ 空窗口 / 跨窗口 / 归一化 | **划除**：本 change 不改 `start_date/end_date` 与分页语义，只换投影列；既有日期筛选测试保持回归绿 |
| 幂等 | 重复执行（重复点选同一文章、重复触发刷新）/ 部分失败重试（详情 500）/ 并发（仅当声称线程安全） | 重复点选→允许重复 hydrate 但最终状态一致（详情覆盖为幂等可重入）；详情 500→列表行仍可读、导语回退 `excerpt`、不弹错；并发→**划除**（未声称线程安全，前端为单用户单标签页场景） |
| 可用性（UI 必检前三） | 加载态 / 空态 / 错误态 / 超长文本 / 重复提交 | 加载态：首帧用列表行渲染（不新增加载屏）；空态：既有空列表文案不变；错误态：详情失败时正文区显示既有兜底（不变）+ 导语回退；超长文本：`excerpt` 截断 200 字符；重复提交：点选去抖沿用既有实现 |

## 3. 继承与调整（⓪ 契约变更反查）

反查命令：`bash scripts/harness/test-assets.sh reading-article-pane` / `... refresh-parallelization`（结果见下）。

| 旧 Scenario（capability） | 处置 | 旧测试资产 | 动作 |
|---|---|---|---|
| `reading-article-pane` / 简介导语段…·有实质内容 | 语义保留、**数据来源变更** | `ArticleContentPreviewPanel.test.ts`（直接传 `description` fixture） | 保留 + 新增「详情未就绪用 excerpt」「详情返回覆盖 excerpt」两用例 |
| `reading-article-pane` / 简介导语段…·无实质内容 | 不变 | 同上（guard 断言） | 保留回归 |
| `reading-list-panel` / 文章列表采用行式布局·行内元数据去重 | 不受影响（行字段未变） | `ArticleCardView.test.ts` | 回归跑（不改） |
| `refresh-parallelization` / 前端页面加载两波并行 | 不变（`onMounted` 两波路径不动） | 无既有自动化（历史 change 记「人工」） | 保持人工验证；本 change 不改该文件路径 |
| `refresh-parallelization` / 后端 refresh-all 并发执行 | 不受影响（后端不改） | — | 无需动作 |
| `article_handler_test.go` 中断言列表返回含 `content`/`description` 的用例（若有） | **按新契约调整** | `backend-go/internal/reader/handler/article_handler_test.go` | 动工前先跑该文件，逐条按新契约更新断言（旧断言视为无效契约） |

## 4. 效果核对（真库量化）

- **触发原因**：响应体积依赖真库文章内容（单条 `content` 从数百字节到 89 KB 不等），fixture 无法代表，必须真库量。
- **方法**：只读 `GET /api/articles?per_page=20`（`curl --noproxy '*'`），量响应体字节数与字段集合；对照组 = 动工前实测值。
- **量化结果（动工前基线）**：响应体 **834.2 KB**；`content` 349.5 KB + `description` 345.7 KB + `firecrawl_content` 48.2 KB = 743 KB（占字段 99%）；列表消费字段合计 5 KB。
- **结论 / 达标判据**：动工后真库实测 **< 100 KB** 且响应项不含 `content`/`description`/`firecrawl_content`；记录动工前后两次数值到 `evidence/`。

## 5. 白盒附加（复杂档）

### 5.1 `GetArticles` 投影分支表

| 分支 | 请求 | 期望 |
|---|---|---|
| 无筛选 | `?page=1&per_page=20` | 窄字段集 + `excerpt` |
| relevance 排序 | `?watched_tags=true&sort_by=relevance` | 窄字段集 + `excerpt` + `relevance_score`（排序键保留） |
| date 排序（DISTINCT 路径） | `?watched_tag_ids=1&sort_by=date` | 窄字段集 + `excerpt` |
| concept 筛选 | `?concept_id=N` | 窄字段集 + `excerpt` |
| aux 筛选 | `?auxiliary_label_id=N` | 窄字段集 + `excerpt` |
| 归档视图 | `?archived=true` | 窄字段集 + `excerpt` |
| `per_page=0` / 负数 | `?per_page=0` | clamp 到默认（20），无超限 WARN |
| `per_page=100`（边界内） | `?per_page=100` | 100 条，无超限 WARN |
| `per_page=101`（越界） | `?per_page=101` | 100 条 + WARN |
| `per_page=10000` | `?per_page=10000` | 100 条 + WARN 含 `10000` |

### 5.2 `excerpt` 生成边界值

| 输入 | 期望 |
|---|---|
| `""` | `""` |
| `"  \t\n "` | `""` |
| `"　　"`（全角空格） | `""` |
| `"<p></p>"` | `""` |
| `"<img src='x'>"` | `""` |
| `"，，，。"`（纯符号） | `""` |
| `"abc"` | `"abc"` |
| `"<p>Hello <b>world</b></p>"` | `"Hello world"` |
| `"a&amp;b&lt;c"` | `"a&b<c"` |
| 199 / 200 / 201 字符纯文本 | 分别为 199 / 200 / 200 字符（≤200） |
| 50 KB HTML（真库样本） | ≤200 字符、不含 `<`/`>` 标签 |
| `<script>alert(1)</script>text` | 不含脚本标签，保留 `text` |
| `description` 空但 `content` 非空 | 无 Firecrawl 正文 → `""`（导语来自正文，与展示正文必然重复）；有 Firecrawl 正文 → 照常返回 |
| `description` 与 `content` 为同一份文本（RSS 全文源） | `""`（去重 guard 本就会隐藏，下发会造成首帧闪现） |
| 短导语（< 40 字符）且导语来自正文兜底（`description` 空、无 Firecrawl 正文） | `""`（guard 的相等规则无长度门槛，短帖也会被隐藏 → 实测 V2EX 15 条） |
| `description` 是 `content` 的子串且 ≥ 40 字符 | `""` |
| `description` 与正文不同（≥ 40 字符） | 照常返回导语文本（guard 会渲染） |
| 短导语（< 40 字符，且与正文不同） | 照常返回（guard 的重复判定带 ≥ 40 字符门槛，短导语会渲染） |

### 5.3 划除留痕

- 时间窗口组：不改日期语义 → 划除（见 §2）。
- 并发/线程安全：未声称线程安全 → 划除（见 §2）。
- DB 迁移层：本 change 无迁移 → 不适用（无 testcontainer 用例）。

### 5.4 层选择结论（五问句 ③）

最便宜层优先：投影与 `excerpt` → Go handler 测；刷新编排与导语回退 → Vitest（composable/组件）；**完整交互故事（节拍 5）→ opencli 端到端（静态 `:5100`）**，无自动化部分按「人工：验证方式」留痕。
