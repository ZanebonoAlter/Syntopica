# test-cases: add-image-proxy

> 这是未执行的交付账本。后端代理纯逻辑/校验用函数与 handler 单测（httptest stub，不打真实图床）；缓存文件系统行为用临时目录单测（t.TempDir，不碰 data/ 生产目录）；前端改写纯函数与组件用 Vitest `--maxWorkers=2`。完整交互故事用「人工：静态 :5100 + 浏览器 Network 断言 + curl」作为 opencli 替代，实施时留证据。curl 单发 ≠ 侦探墙全链路验收。

## 故事S1：外链图片经代理带对 Referer 成功加载

锚：代理转发与 Referer 注入（禁空/白名单两类防盗链）、前端外链图片统一改写、侦探墙贴图不再 403。

### 主链路

| 步 | 动作 | 来源Scenario | 期望 | 层/规划落点 |
| --- | --- | --- | --- | --- |
| 1 | 前端渲染 sspai 封面 URL | 外链封面改写为代理地址 | `<img src>` 变 `/api/image-proxy?url=<enc>`，原始 URL 不变仍存库 | front/app/utils/imageProxy.test.ts + ArticleCardView.test.ts |
| 2 | 代理收到请求，注入 Referer=图片自身 origin + 浏览器 UA | 禁空 Referer 图床经代理成功 | 上游收到 `Referer: https://cdnfile.sspai.com/` 与非 Go UA，200 图片字节与 Content-Type 原样透传 | imageproxy/handler_test.go（httptest 上游断言请求头） |
| 3 | 首次下载后落盘缓存 | 二次请求命中缓存 | 文件落 `data/image-cache/sha256(url)`，响应带 `X-Image-Proxy-Cache: HIT`（第二次） | imageproxy/cache_test.go |
| 4 | 侦探墙 Image 加载同一 URL | 侦探墙贴图不再 403 | 纹理绘制完成；Network 对图床域名零直连、零 `deny by referer access rule` | 人工：静态 :5100 侦探墙 + Network 断言（3.4/5.5） |

## 故事S2补充：外链 favicon 不再直连（FeedIcon，实现期排查新增）

| 步 | 动作 | 来源Scenario | 期望 | 层/规划落点 |
| --- | --- | --- | --- | --- |
| 1 | feed 图标为外链 URL | 前端外链图片统一改写 | src 经代理；`@error` 降级 mdi:rss 保持 | FeedIcon.test.ts |
| 2 | feed 图标为 `/icons/...` 本地路径 | 非外链地址不改写（自有地址特例） | 直连 API origin，不进代理（自指防护） | FeedIcon.test.ts:29 原断言 |

## 故事S2：非法与失败请求被安全拒绝，前端照旧降级

锚：非法 url 被拒绝、上游拒绝时透传状态码、破图降级衔接。

| 步 | 动作 | 来源Scenario | 期望 | 层/规划落点 |
| --- | --- | --- | --- | --- |
| 1 | 缺 url / `ftp://` / 指向代理自身 host:port | 非法 url 被拒绝 | 4xx，上游计数为 0（零上游请求） | imageproxy/handler_test.go |
| 2 | 上游仍 403（IP 封禁/签名失效） | 上游拒绝时透传状态码 | 透传 403，缓存零写入 | imageproxy/handler_test.go |
| 3 | 上游超时 / 连接失败 | 破图降级衔接（外部依赖失败有答案） | 504/502，前端收到错误响应 | imageproxy/handler_test.go（httptest 不响应/Close） |
| 4 | 代理返回透传 403 给列表封面 | 代理透传 403 时封面降级 | `@error` 触发，FeedIcon 呈现，无浏览器破图 | ArticleCardView.test.ts 既有降级用例 |

## 故事S3：缓存吃图但不撑爆 SD 卡

锚：磁盘缓存与大小上限、非图片响应不缓存、缓存超限滚动淘汰。

| 步 | 动作 | 来源Scenario | 期望 | 层/规划落点 |
| --- | --- | --- | --- | --- |
| 1 | 上游 200 但 `Content-Type: text/html` | 非图片响应不缓存 | 照常透传，缓存目录零新增文件 | imageproxy/cache_test.go |
| 2 | 写入后总量超上限 | 缓存超限滚动淘汰 | 按 mtime 旧→新删到 ≤90% 水位，最新访问文件保留 | imageproxy/cache_test.go（t.TempDir + IMAGE_CACHE_MAX_MB 覆盖） |
| 3 | 上游 403 | 上游拒绝时透传状态码 | 不写缓存（负面节拍） | imageproxy/cache_test.go |

## 故事S4：改写收敛到一个入口，非外链不被误伤

锚：前端外链图片统一改写（不改写矩阵 + 全调用点覆盖）。

| 步 | 动作 | 来源Scenario | 期望 | 层/规划落点 |
| --- | --- | --- | --- | --- |
| 1 | `proxiedImageUrl` 过五类不改写输入 | 非外链地址不改写 | 空/相对/`data:`/`blob:`/已代理地址原样返回，零二次嵌套 | imageProxy.test.ts |
| 2 | 头图/正文 `<img>`/侦探墙贴图接入 | 外链封面改写为代理地址（同机制） | 各调用点均经同一工具改写 | ArticleContentPreviewPanel / markdown·useArticleContentView 单测 / CardGroup grep |
| 3 | `grep no-referrer` | 侦探墙贴图不再 403（前置清理） | front/app 零命中 | 任务 2.5 验证命令 |

> 上表后端省略共同前缀 `backend-go/internal/platform/imageproxy/`；前端省略 `front/app/`。

## 继承与调整（已运行 test-assets 反查）

已运行 `bash scripts/harness/test-assets.sh image-proxy` →「capability 无匹配：主 specs 与 archive 均不存在」——本 capability 全新，无旧 Scenario/旧测试需要继承。相关既有测试按 MODIFIED-less 处理：`ArticleCardView.test.ts`「封面加载失败降级」用例断言的契约不变，应原样通过（若失败=实现破坏了既有降级契约，修实现不修改用例）。

**实现期契约更新（2026-09-22，已同步实现与断言）**：

| 旧断言 | 处置 | 旧测试 | 动作 |
| --- | --- | --- | --- |
| `ArticleCardView.test.ts:168` 断言封面 src == 原始外链 | 本 change 显式改契约（spec：外链封面改写为代理地址） | 同文件 | 断言更新为 `/api/image-proxy?url=<enc>`；降级相关用例不动 |
| `FeedIcon.test.ts:19/:114` 断言外链 favicon src == 原始 URL | 同上（FeedIcon 外链接线为实现期排查新增点位，任务 2.6） | 同文件 | 断言更新为代理地址；`:29` 本地路径断言**不动**，反向锁定「后端自有地址不进代理」 |
| F6 同源分支 | 实现揭示（dev 页面与 API 不同源时同源判断拦不住自家地址，需调用方约束 + 后端自指校验双保险） | imageProxy.test.ts | 白盒表补 F6 行，测试补同源用例 |

## 变体走查（五组固定清单）

| 组 | 条目与答案 | 层/落点 |
| --- | --- | --- |
| 输入 | url 空串→400；纯空白（含全角/tab）→400；缺 scheme（`//host/x.png`）→400；`ftp:`/`file:`/`javascript:`→400 且零上游；大小写 `HTTPS://`→接受（url.Parse 归一）；特殊字符（`&`、`#`、中文、`?imageView2/...` 查询串）→解码后原样转发不吞参；超长 url（8KB）→有界处理不 panic；前端空/纯空白 imageUrl→不渲染 img 不发请求 | handler_test.go、imageProxy.test.ts |
| 前置 | 缓存目录不存在→自动创建；上游 302 重定向→跟随（Go 默认 10 次上限内）且只缓存最终 200 图；上游超时→504；缓存目录无权限写→透传不受影响（缓存失败不拖垮响应）；并发同图首载→不声称线程安全，允许重复下载（D2 有意取舍），rename 原子保证无半文件 | handler_test.go、cache_test.go |
| 时间窗口 | 不适用业务时间窗（无日期语义）。缓存 LRU 边界：mtime 恰好等于水位线的文件保留策略由「删到 <90% 上限」覆盖；mtime 相同（同一秒写入）→任一淘汰皆可接受，断言总量达标而非具体文件 | cache_test.go；本组其余划除：无「当天算不算/空窗口/跨窗口/归一化」场景 |
| 幂等 | 重复 GET 同 URL→第二次 HIT 且零上游（幂等）；淘汰中途失败→下次写入重试，无残留 .tmp；部分失败（磁盘满写一半）→临时文件 rename 失败即弃，透传链路不受影响；并发写同 hash→单写者目录 rename 原子，不声称并发去重 | cache_test.go；「仅当声称线程安全的并发」——不声称，划除并发互斥断言 |
| 可用性(UI必检前三) | 误输入反馈：URL 由系统渲染非用户输入，无误输入面→划除；空态：无封面文章→FeedIcon（既有，回归）；错误态：代理 403/502→既有降级视觉，无新错误文案（负向断言：测试不出现新文案节点）；加载态：不变（`loading="lazy"` 保留）；超长文本：超长 URL 编码后 src 不截断；重复提交：GET 无提交按钮→划除 | ArticleCardView.test.ts、ui-design 受影响状态节 |

## 效果核对

效果目标=「外链 403 图裂消失」，依赖断言外因素（真实图床规则、浏览器行为），做前后量化对照：

| 项 | 方法 | 量化结果 | 结论 |
| --- | --- | --- | --- |
| 直连基线 | `curl`（无 Referer）直取 sspai 图 | 实测 403 + `x-exception-info: deny by referer access rule`（2026-09-22 探索期已记录） | 证明问题真实存在 |
| 代理修复 | `curl --noproxy '*'` 打 `:5100/api/image-proxy?url=<同图>` | 实现后填：期望 `200 image/*` 且 `X-Image-Proxy-Cache` 依次 MISS→HIT | 代理根治禁空 Referer 类 |
| 全链路 | 静态 :5100 打开侦探墙含 sspai 图文章，Network 面板 | 实现后填：图床域名直连请求数=0、代理请求 200 | 前端改写收敛生效 |

## 白盒附加（分支表 + 边界值 + 划除留痕）

**分支表 `proxiedImageUrl(url)`**：

| # | 条件 | 输出 | 断言落点 |
| --- | --- | --- | --- |
| F1 | falsy（null/undefined/''/纯空白） | 原样 | imageProxy.test.ts |
| F2 | 含 `/api/image-proxy` 前缀 | 原样（防嵌套） | 同上 |
| F3 | `data:` / `blob:` 开头 | 原样 | 同上 |
| F4 | 非 `http(s)://` 开头（相对路径、`//`、其它 scheme） | 原样 | 同上 |
| F5 | `http://` / `https://` | `/api/image-proxy?url=`+encodeURIComponent | 同上 |
| F6 | 与 `window.location.origin` 同源的绝对地址（dev 下指向后端的地址由调用方约束不进本函数） | 原样（避免喂给代理被自指校验 400） | imageProxy.test.ts（实现揭示后补入） |

**分支表 handler 校验**：

| # | 条件 | 输出 | 断言落点 |
| --- | --- | --- | --- |
| H1 | 缺 url / 空白 | 400，上游 0 请求 | handler_test.go |
| H2 | scheme ∉ {http, https} | 400，上游 0 请求 | 同上 |
| H3 | host:port == 代理自身 | 4xx，上游 0 请求 | 同上 |
| H4 | 合法 + per-host 覆盖表无该 host | Referer=`scheme://host/` | 同上（断言上游收到的头） |
| H5 | 合法 + 覆盖表命中 | Referer=覆盖值 | 同上（表初始空，单测直接喂表） |
| H6 | 上游 200+image/* | 透传 + 落缓存 | cache/handler_test.go |
| H7 | 上流 200+非图 | 透传 + 不落缓存 | cache_test.go |
| H8 | 上游 4xx/5xx | 状态透传 + 不落缓存 | handler_test.go |
| H9 | 上游超时/断连 | 504/502 | handler_test.go |

**分支表缓存淘汰**：

| # | 条件 | 输出 | 断言落点 |
| --- | --- | --- | --- |
| C1 | 目录不存在 | 自动建目录 | cache_test.go |
| C2 | 命中文件存在 | 读文件 + touch mtime + HIT 头，零上游 | 同上 |
| C3 | 写入后总量 ≤ 上限 | 不扫描 | 同上 |
| C4 | 写入后总量 > 上限 | 按 mtime 升序删到 ≤90% 上限 | 同上（断言总量与最新文件存活） |
| C5 | `IMAGE_CACHE_MAX_MB=0` | 不缓存（或等价禁用语义，实现时定其一并断言） | 同上 |

**边界值**：url 恰为图片 origin 根路径；host 大小写混合（URL host 归一）；显式 `:443` 与缺省端口的 Referer 拼接；上限恰好等于当前总量（不触发）；单文件 > 上限（写入即触发淘汰删到只剩新文件或空，断言有界不卡死）。

**划除留痕**：业务时间窗口组整体不适用（无日期语义）；「误输入反馈」「重复提交」UI 条目不适用（URL 系统渲染、GET 无提交）；并发互斥断言不适用（D2 明示不声称线程安全，只断言 rename 原子的最终一致）；SQLite 全程不出现（文件系统缓存无 SQL，repository 层不适用）。
