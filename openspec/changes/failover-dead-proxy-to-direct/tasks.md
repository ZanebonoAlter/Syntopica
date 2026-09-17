# Tasks — failover-dead-proxy-to-direct

## 1. 核心实现：failover transport + 即时生效（httpclient 包内，零调用方改动）

- [x] 1.1 包级 `atomic.Value` 存当前代理 `*url.URL`；`SetProxy` 改为原子写 + `breaker.reset()`（校验逻辑/错误返回不变；空串=清空）——单测：`TestSetProxy_AppliesToNewClients` / `TestSetProxy_EmptyClearsProxy` / `TestSetProxy_RejectsUnsupportedScheme` 继续绿——落点 `internal/platform/httpclient/httpclient.go` + `httpclient_test.go`
- [x] 1.2 包级单例 failover transport：`inner *http.Transport` 的 Proxy 函数每请求动态决策（回环→nil → 熔断/试探占用→nil → 空 URL→`ProxyFromEnvironment` → 代理 URL）；`New()` 未显式 `WithTransport` 时一律取该单例——单测：既有 `TestProxyBypass_*` 三条继续绿——落点同上
- [x] 1.3 熔断器（mutex + `downUntil` + `probing`）与 `RoundTrip` 包装：拨代理失败→ `markDown()` + 同 transport 重试一次；窗口内/试探占用期 Proxy 返回 nil 直连；到期放行单请求试探，试探结果按设计 §4 归类（非 dial 代理失败=代理路径存活→清熔断）——单测见任务 2——落点 `internal/platform/httpclient/failover.go`（新文件）※实现偏离：错误分类改用 design 兜底方案 `proxyDialContext` 拨号层打标（实测 OpError 方案被 `proxyconnect` 包装遮蔽 + Addr 为解析后 IP），已回写 design.md 决策 2 留痕
- [x] 1.4 错误分类函数 `isProxyDialFailure`：errors 链找 `proxyDialError` 哨兵（拨号层打标）；以 httptest 矩阵单测验证 http/https(CONNECT)/socks5 三 scheme 的拨代理失败形态——落点 `internal/platform/httpclient/failover_test.go`

## 2. 测试矩阵（复杂档：状态机 3 态 + 错误分类 + 单飞，用例清单见 test-cases.md）

- [x] 2.1 降级链路：代理拨号失败→本请求直连重试成功、熔断窗口内后续请求零代理拨号（代理监听器连接计数断言）——落点 `failover_test.go`
- [x] 2.2 恢复链路：窗口到期试探成功自动接回、试探失败续期且请求不报错、试探期并发请求直连不惊群——落点 `failover_test.go`
- [x] 2.3 负向分类：代理活着但 502 / CONNECT 后目标错误 / 目标经代理超时 → 不熔断、不重试、行为与现状一致——落点 `failover_test.go`
- [x] 2.4 即时生效：运行时清空立即直连（代理连接计数不再增长）、熔断中换址立即生效且熔断重置——落点 `failover_test.go`

## 3. 收尾验证

- [x] 3.1 影响包测试全绿：`go test ./internal/platform/httpclient/...`（经 `scripts/change-scope.sh` 判定）
- [x] 3.2 本地端到端（真实栈实证，2026-09-17）：`start-dev.sh --restart back` 载新代码 → 运行时配必挂代理 `http://127.0.0.1:59999`（无重启）→ 刷新 feed：直连可达源 feed 9（博客园）**success**；自建 RSSHub 宕机的 feed 3/16 报目标侧 connection refused（回退链路正确，错误从代理层变目标层）；feed 20 HTTP 抓取成功仅解析失败（源内容问题）；需代理的 feed 28（v2ex）直连超时（预期，直连救不了被墙站点）→ 恢复代理配置为空
- [x] 3.3 完工汇报含「部署后影响 + 需要的操作」（见下节完工汇报）

## 4. 测试

- [x] `cd backend-go && go test ./internal/platform/httpclient/...` → ok（26 测试全 PASS，含 12 个新 failover 测试）
- [x] `cd backend-go && golangci-lint run ./internal/platform/httpclient/... && go vet ./internal/platform/httpclient/... && go build ./...` → 0 issues / 无输出 / 成功

## 5. 文档

<!-- doc-impact: flow, configuration -->

- [x] `docs/reference/configuration.md` 出站代理节：修正「保存即时生效」表述（改为真实语义：atomic URL 所有已构造 client 即时生效）+ 补熔断降级与恢复语义、错误分类边界
- [x] `docs/reference/flow/scheduler.md`「代理污染防线」节：补充代理不可达熔断直连回退（60s 窗口 + 单飞试探）一段
- [ ] 归档后按 §12.2 在对应 flow 文档补「变更溯源」链接（archive 后补，属归档收尾 commit）

## 6. 验证

归档门禁命令（2026-09-17 实测）：

- `cd backend-go && go test -count=1 ./internal/platform/httpclient/...` → `ok syntopica-backend/internal/platform/httpclient 0.646s`（26 测试全 PASS）
- `cd backend-go && golangci-lint run ./internal/platform/httpclient/...` → `0 issues`；`go vet ./internal/platform/httpclient/...` → 无输出；`go build ./...` → 成功
- `bash scripts/scenario-trace.sh openspec/changes/failover-dead-proxy-to-direct` → ✓ 11 个 Scenario 映射齐全，退出码 0
- `bash scripts/doc-impact.sh verify openspec/changes/failover-dead-proxy-to-direct` → 通过（声明 flow, configuration，文件 2 个）
- `bash scripts/check-standards.sh --change failover-dead-proxy-to-direct` → 通过 155 / 失败 0（A-I 全段零失败）
- 注：`go test -race` 本机不适用（arm64 工具链 race runtime ABI 不匹配，测试未执行即 FATAL），并发正确性由 mutex 单写者设计 + `TestProbeSingleFlightConcurrentDirect` 行为断言覆盖

### Scenario → 测试文件映射（scenario-trace）

| Scenario | 测试文件 |
| --- | --- |
| 代理挂掉时直连可达源正常抓取 | backend-go/internal/platform/httpclient/failover_test.go |
| 熔断窗口内零代理拨号 | backend-go/internal/platform/httpclient/failover_test.go |
| 未配置代理行为不变 | backend-go/internal/platform/httpclient/proxy_loopback_test.go |
| 代理恢复后自动接回 | backend-go/internal/platform/httpclient/failover_test.go |
| 试探失败续期 | backend-go/internal/platform/httpclient/failover_test.go |
| 试探期间并发请求直连 | backend-go/internal/platform/httpclient/failover_test.go |
| 代理返回 502 不熔断 | backend-go/internal/platform/httpclient/failover_test.go |
| 代理可达时的目标超时不熔断 | backend-go/internal/platform/httpclient/failover_test.go |
| 运行时清空代理立即直连 | backend-go/internal/platform/httpclient/failover_test.go |
| 运行时换址立即生效且熔断重置 | backend-go/internal/platform/httpclient/failover_test.go |
| 启动注入语义不变 | backend-go/internal/platform/httpclient/httpclient_test.go |

对应测试函数（apply 后回填实际函数名）：

- `TestProxyDialFailureFallsBackToDirect`、`TestCircuitOpenSkipsProxyDial`、`TestProbeSuccessReopensCircuit`、`TestProbeFailureExtendsWindow`、`TestProbeSingleFlightConcurrentDirect`、`TestProxyAliveTargetErrorsNoBreaker`（502/CONNECT/超时三形态）、`TestRuntimeClearProxyTakesEffect`、`TestRuntimeSwapProxyResetsBreaker`
