# design — 代理不可达熔断直连回退 + 配置即时生效

## Context

现状（`backend-go/internal/platform/httpclient/httpclient.go`）：

- `SetProxy(rawURL)` 校验 scheme（http/https/socks5）后 `Clone` 一份 `http.DefaultTransport`、装上 `Proxy=proxyWithLoopbackBypass(u)` 函数，存入包级 `proxyTransport`（RWMutex 保护）。
- `New(opts...)` 在**构造时**快照：调用方未显式 `WithTransport` 时取 `currentProxyTransport()`，否则 `http.DefaultTransport`。注释明说 "clients already built keep their transport"——`RSSParser`/`FirecrawlService`/airouter 等启动单例因此吃不到运行时代理变更。
- 回环目标绕过代理（ai-health-reprobe）在 Proxy 函数内实现。
- 调用方（`cmd/server/main.go` 启动注入、`SaveProxySettings` 运行时保存）对 `SetProxy` 的用法不变。

动机与边界见 proposal.md。

## Goals / Non-Goals

**Goals**

- 代理拨号失败时单请求零感知降级直连；故障期后续请求零代理拨号延迟；代理恢复自动接回
- `SetProxy` 变更对**所有**已构造 client 即时生效（含启动单例）
- 三种 scheme（http/https/socks5）行为一致；回环绕过与环境变量兜底语义不变

**Non-Goals**

- 不做代理存活的后台主动探活线程（惰性试探已够，少一个常驻 goroutine）
- 不做按域名分流规则（proxy/bypass 列表）——YAGNI
- 不改 `GET/POST /api/settings/proxy` 契约与设置页 UI
- 不处理「代理活着但上游全坏」（502 型）的降级——见决策 2

## Decisions

### 1. 熔断与重试做在包级单例 failover RoundTripper，而非 Proxy 函数内

Proxy 函数只能「选路」（返回代理 URL 或 nil），无法对已失败请求重试。结构：

```
httpclient.New() ──► otelhttp 包裹 ──► failoverTransport（包级单例）
failoverTransport:
  inner  *http.Transport   // Proxy 函数每请求动态决策（见决策 3）
  breaker proxyBreaker     // mutex + downUntil + probing
RoundTrip(req):
  resp, err := inner.RoundTrip(req)
  if err != nil && isProxyDialFailure(err) { breaker.markDown(); return inner.RoundTrip(req) }
  // 重试时 breaker 已打开 → inner 的 Proxy 函数返回 nil → 直连
  return resp, err
```

重试安全性：仅当 dial 代理失败（请求从未发出、连接未建立）才重试，对 POST 等非幂等请求同样无副作用；重试共享 `http.Client.Timeout` 预算（整体超时语义不变）。

替代方案（已否）：A) Proxy 函数内判熔断返回 nil、不重试——首个请求仍失败一次，调用方可见错误，体验差；B) 每个 client 包装独立 failover 层——client 无数、状态分散，包级单例一处管够（与现状 `proxyTransport` 全局唯一一致）。

### 2. 错误分类：仅「dial 目标地址 = 代理地址」的网络层失败触发（实现采用拨号层打标）

**主判定 = 自定义 `DialContext` 在拨号层打标**：拨号失败且拨号地址（解析前字符串）匹配当前代理地址时，包一层 `proxyDialError` 哨兵；`isProxyDialFailure` 用 `errors.As` 识别。

> 实现期留痕（apply 实测对调主备）：原拟的「错误链找 `*net.OpError` 比对 Addr」方案实测不可行——①三 scheme（含 plain http）的代理拨号失败都被 net/http 包装为 `proxyconnect tcp: dial tcp ...`，`errors.As` 首个命中的 OpError `Op=="proxyconnect"`，内层 dial OpError 被遮蔽；②`OpError.Addr` 是 DNS 解析后的 IP，域名型代理（生产常态）按主机名比对必失败。拨号层比对解析前地址字符串对三 scheme 一致生效、不受包装形态影响，且 scheme 无关（http/CONNECT/socks5 拨代理失败同形）。本方案即原设计的兜底方案，实测后转正。

行为边界：

- 三 scheme 的「拨代理」失败（连接拒绝/拨号超时）→ 触发熔断 + 本请求直连重试
- 代理活着但对目标 502 / CONNECT 非 200 / socks5 拒绝目标 / 经代理目标超时 → 非 dial-to-proxy 失败 → 不触发（目标侧失败，直连不会更好），且视为代理路径存活的证据（清熔断）

否决「error string 包含代理 host」——字符串匹配脆弱。

### 3. 即时生效：URL 进 atomic，Proxy 函数每请求动态决策

`SetProxy` 不再换 transport 实例，改为原子写包级 `atomic.Value`（解析后的 `*url.URL` 或 nil）；`inner` 的 Proxy 函数每次请求按序决策：

1. 目标为回环（沿用 `isLoopbackHost`）→ nil（直连，既有语义）
2. 熔断打开（`now < downUntil`）或试探单飞占用中 → nil（直连）
3. URL 为空 → `http.ProxyFromEnvironment(req)`（保持「环境变量兜底」语义，替代现状的 DefaultTransport 分支）
4. 否则 → 当前代理 URL

`New()` 未显式 `WithTransport` 时一律取包级 failover 单例——已构造 client 因共享单例而即时生效；`WithTransport` 显式覆盖语义不变（调用方自管代理）。换址/清空时 `breaker.reset()`（新地址按健康对待）。SetProxy 的校验逻辑与错误返回不变。

否决「保留快照语义 + 文档改为重启生效」——与设置页「即时生效」的用户预期相悖，且单例改造后成本极低。

### 4. 熔断器状态机（3 态）与单飞试探

```
正常(closed) ──拨代理失败──► 熔断(open, downUntil=now+60s) ──到期放行试探──► 试探(probing)
   ▲                            │失败续期(试探请求本身降级直连)              │
   └──────────试探成功(代理路径通：dial 成功即视为通)──────────────────────────┘
```

- `markDown()`：`downUntil = now + proxyFailoverWindow`（包级 var，默认 60s，测试注入短窗口）
- 试探请求任何「非拨代理失败」结果（含目标 404/502/超时）→ 代理路径存活 → 清熔断（回到正常）
- 单飞：mutex 下 `probing` 标志——到期后首个请求置位并走代理（试探者），期间其他请求看 `probing` 直连；试探结束清位
- 并发成本：每请求一次 mutex，纳秒级；不用 atomic CAS 状态机（复杂度不成比例）

## Risks / Trade-offs

- [标准库对代理拨号错误的包装形态随 Go 版本漂移] → 决策 2 的单测矩阵锁定行为；兜底自定义 DialContext
- [熔断打开期间，确需代理才能访问的站点（如被墙源）直连失败] → 相比现状（全部失败）仍是纯改善；窗口仅 60s，试探自动接回；不区分域名（Non-Goals）
- [代理地址黑洞（丢包非拒绝）时首个请求 = 拨号超时 + 直连耗时，延迟翻倍] → 常见故障形态是本机代理进程退出（立即 refused，毫秒级）；`Client.Timeout` 兜底整体上限
- [URL 原子换址与在途请求的分类比较存在良性竞态] → 最坏单个请求按旧地址分类，无害；不引入额外同步
- [otelhttp 包裹层次变化] → failover 单例在 otelhttp 之下，span/traceparent 语义不变；既有 `httpclient_test.go` 断言（新 client 拿到代理 transport）继续成立

## Migration Plan

无数据迁移、无 API 变更；部署即生效，回滚 = revert。用户可见行为变化见 proposal「Impact」。

## Open Questions

（无——错误包装形态的实现细节由单测矩阵在 apply 阶段钉死，不影响规格与任务拆分。）
