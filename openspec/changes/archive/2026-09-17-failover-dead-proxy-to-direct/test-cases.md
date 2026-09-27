# test-cases — failover-dead-proxy-to-direct

> 单元 = Requirement「代理不可达时熔断降级直连 / 熔断恢复试探 / 错误分类 / 即时生效」四故事，串成一个用户故事：**用户配了 Clash 当全局代理；Clash 崩了，国内 RSS 照常刷新；Clash 重启后流量自动回到代理；期间在设置页换代理地址立即生效**。

## 主链路表

| # | 节拍 | 来源 Scenario | 期望 | 层 | 落点 |
| --- | --- | --- | --- | --- | --- |
| 1 | 配置代理 A（无可达监听），请求直连可达目标 | 代理挂掉时直连可达源正常抓取 | 首请求拨 A 失败→直连重试成功；熔断打开 | Go 单测（httptest 目标 + 不可达代理端口） | `failover_test.go` |
| 2 | 熔断窗口内继续请求 | 熔断窗口内零代理拨号 | 全部直连成功；A 的监听位置连接计数不增 | 同上 | `failover_test.go` |
| 3 | 窗口到期前重启代理监听器，到期后请求 | 代理恢复后自动接回 | 试探成功→熔断关闭，请求经代理转发 | 同上 | `failover_test.go` |
| 4 | 窗口到期代理仍不可达，请求 | 试探失败续期 | 请求直连成功不报错；downUntil 续满一个窗口 | 同上 | `failover_test.go` |
| 5 | 窗口到期并发 N 请求、代理不可达 | 试探期间并发请求直连 | 恰 1 次试探拨号；N 个请求全成功 | 同上（goroutine + WaitGroup） | `failover_test.go` |
| 6 | 代理可达、代理对目标回 502 | 代理返回 502 不熔断 | 调用方拿到 502；熔断不开；后续仍走代理 | 同上（httptest 代理回 502） | `failover_test.go` |
| 7 | 代理可达、目标慢（阻塞 handler + 短超时） | 代理可达时的目标超时不熔断 | 超时报错；熔断不开 | 同上 | `failover_test.go` |
| 8 | 经代理请求成功后，SetProxy("") | 运行时清空代理立即直连 | 后续请求直连（代理连接计数冻结） | 同上 | `failover_test.go` |
| 9 | 熔断打开中 SetProxy(可达代理 B) | 运行时换址立即生效且熔断重置 | 请求立即经 B 成功 | 同上 | `failover_test.go` |
| 10 | 启动态 SetProxy 后 New() | 启动注入语义不变 | 新 client 走代理（既有测试回归） | Go 单测 | `httpclient_test.go` |

层选择依据：纯平台网络逻辑，httptest 即最便宜可信层；无 DB（不涉 testcontainer）、无前端（无 opencli 落点，交互故事为后端网络行为本身）。

## 变体走查

- **输入**：代理 URL 三 scheme 合法形态各覆盖拨代理失败路径（任务 1.4 矩阵）；非法 scheme/解析失败 → 既有 `TestSetProxy_RejectsUnsupportedScheme` 既有覆盖；空串=清空 → 主链路 #8；**大小写/超长/特殊字符变体：不适用（URL 解析归 `url.Parse` + scheme 白名单，既有测试管）**——划除留痕
- **前置**：代理可达/不可达/半可达（活着但上游坏）三态 = 主链路 #1-#7 全覆盖；空集（未配置代理）→ 「未配置代理行为不变」
- **时间窗口**：边界两端 = downUntil 到期瞬间（#3/#4 各踩一侧）；归一化不适用（无日期）——划除留痕
- **幂等**：重复执行 = 窗口内重复请求行为稳定（#2）；部分失败重试 = 试探续期（#4）；并发 = #5 单飞
- **可用性（UI 前三必检）**：不适用（纯后端，无 UI）——划除留痕

## 白盒附加（复杂档：3 态状态机 + 错误分类 + 单飞）

### 分支表（RoundTrip 入口 × 状态 × 结果）

| breaker 态 | Proxy 函数决策 | 请求结果 | 分类 | 状态迁移 |
| --- | --- | --- | --- | --- |
| closed，URL 空 | ProxyFromEnvironment | 任意 | 不分类 | 不变 |
| closed，URL 设，loopback 目标 | nil 直连 | 任意 | 不分类 | 不变 |
| closed，URL 设，外部目标 | 代理 | dial 代理失败 | isProxyDialFailure=T | →open（downUntil=now+W）+ 本请求直连重试 |
| closed，URL 设，外部目标 | 代理 | 目标侧失败（502/CONNECT 非 200/超时） | F | 不变（不重试） |
| open（now<downUntil） | nil 直连 | 任意 | 不产生（未走代理） | 不变 |
| open 到期，无试探者 | 代理（试探，置 probing） | dial 代理失败 | T | probing 清、downUntil=now+W（续期）+ 直连重试 |
| open 到期，无试探者 | 代理（试探） | 非 dial 失败 | F | probing 清、→closed（代理路径存活） |
| open 到期，有试探者在飞 | nil 直连（单飞让路） | 任意 | 不产生 | 不变 |

### 边界值

- downUntil 到期瞬间并发：首请求抢 probing 单飞，其余直连（#5 断言拨号计数恰 1）
- 换址与在途请求竞态：分类按请求发起时读到的地址比对，换址瞬间最坏单请求误判——良性，接受（design §Risks）
- dial 失败形态：connection refused（快）与黑洞超时（慢）都属 OpError.Addr 匹配——测试用 refused 形态，超时形态靠分类函数同构覆盖
- 重试与 `Client.Timeout` 共享预算：重试不重置整体超时（实现断言：直连重试后 client.Timeout 语义不变）

### 效果核对

效果不依赖数据覆盖率/LLM 行为，单测矩阵即量化证据；部署后端到端核对见 tasks 3.2（真实 Clash 场景人工留痕）。

## ⓪ 继承与调整

无 MODIFIED/REMOVED Requirements（ai-health-reprobe 回环要求原样保留）——不适用，划除留痕。
