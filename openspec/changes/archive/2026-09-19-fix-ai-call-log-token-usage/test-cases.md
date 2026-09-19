<!-- 遵循 docs/reference/standard/shared/test-design.md：复杂档，白盒附加必备 -->

# test-cases — fix-ai-call-log-token-usage

**测试单元（用户故事）**：作为运维者，我发起一次 AI 调用（chat 或 embedding）后，能在 `ai_call_logs.token_usage` 看到真实的 prompt/completion/total 用量，并在 session 聚合里看到两类调用的总和；provider 没给用量或调用失败时，看到 NULL 而不是假零。

## 0. 继承与调整（⓪ 契约 MODIFIED，test-assets.sh ai-logging 反查）

| 旧 Scenario | 处置 | 旧测试 | 动作 |
| --- | --- | --- | --- |
| token 用量记录（本 change 拆为「token 用量记录」+「无 usage 块不落全零」） | 修改 | `router_test.go: TestEncodeTokenUsage`（存储形状 `{"prompt":10,...}` 断言） | **保留不动**——正是本 change 要求不变的存储形状回归锚 |
| 同上 | 修改 | `router_test.go: TestTokenUsageInChatResult`（ChatResult.Usage 透传，走 fake client 绕过 HTTP 解析） | **保留不动**；bug 在 wire 解析层、fake 路径测不到，新增 client 层 HTTP 用例补盲区 |
| 完整 prompt 落库 / 历史行回填 / operation 列约束 | 不变 | 无（test-assets 无历史映射） | 不动 |

## 1. 主链路表（节拍）

| 步 | 动作 | 来源 Scenario | 期望 | 层 | 落点 |
| --- | --- | --- | --- | --- | --- |
| 1 | chat 调用成功，provider 返回标准 usage 键 | token 用量记录（MODIFIED） | `token_usage` = `{"prompt":N,"completion":N,"total":N}` 且 total>0 | client HTTP 单测（httptest）+ router 单测 | `openai_compatible_test.go` 新增；`router_test.go` 新增 |
| 2 | 同一调用查库验证存储形状 | 同上（形状不变） | jsonb 键仍为 `prompt`/`completion`/`total` | router 单测（查 ai_call_logs 行） | `router_test.go` 新增 |
| 3 | chat 成功但响应无 usage 块 | 无 usage 块不落全零（MODIFIED） | `token_usage` IS NULL，非全零 jsonb | router 单测（fake client usage=nil） | `router_test.go` 新增 |
| 4 | chat 调用失败（HTTP 500） | 同上（失败分支） | `token_usage` IS NULL | router 单测（复用 seedChatFailover 失败路径） | `router_test.go` 新增断言 |
| 5 | embed 调用成功，provider 返回 usage | embedding 成功调用记录用量（ADDED） | `token_usage` 非零、形状同上 | client HTTP 单测 + router 单测 | `openai_compatible_test.go` / `router_test.go` 或 `embed_cache_router_test.go` 新增 |
| 6 | embed 请求命中本地缓存 | embedding 缓存命中不记用量（ADDED） | 对应日志行 `token_usage` IS NULL | router 单测（扩展既有缓存命中用例） | `embed_cache_router_test.go` |
| 7 | 同 session 下 chat+embed 混合调用聚合 | session 聚合涵盖 embedding 用量（ADDED） | `summary.total_tokens` = 两类行之和 | handler 单测（既有 fixture 字符串形状未变，聚合逻辑零改动） | `session_handler_test.go` 保留；如缺口则补混合用例 |

## 2. 变体走查（五组清单）

| 组 | 变体 | 答案 |
| --- | --- | --- |
| 输入 | usage 键族：标准键 / 历史键 / 两者皆无 | 步1 测标准键；白盒附测历史键兼容（防御）；皆无 → 全零值结构 |
| 输入 | usage 块整体缺失 / `usage: null` | `parsed.Usage == nil` → NULL 落库（步3） |
| 输入 | 空串/纯空白/分隔符/大小写/特殊字符/超长 | ~~不适用~~——usage 是结构化 JSON 块，非自由文本输入 |
| 前置 | 空集（无 providers）/ 越界引用 | 既有降级链行为，本 change 未触及，划除 |
| 前置 | 单元素/重复 provider | 同上，划除 |
| 时间窗口 | 边界/空窗口/跨窗口/归一化 | ~~不适用~~——无时间语义 |
| 幂等 | 重复执行/部分失败重试/并发 | 重复调用各记一行（现状保持）；并发：`LogCall` 无新增共享可变状态，白盒确认后划除 |
| 可用性 UI | 误输入/空态/错误态/加载态/长文本/重复提交 | ~~不适用~~——ui-impact: none，无界面改动 |

## 3. 效果核对（问句④）

效果依赖「真实 provider 返回标准 usage 块」这一断言外因素 → 真库量化核对：tasks 6.5 触发真实调用后 psql 抽查最近 5 分钟行（触发原因=修复上线；方法=psql 按 created_at 窗口过滤；量化=chat 行 total>0、embed 非缓存行非 NULL；结论记录在任务勾选时）。

## 4. 白盒附加（复杂档）

### UnmarshalJSON 分支表

| 分支 | 输入 | 期望 |
| --- | --- | --- |
| 标准键 | `{"prompt_tokens":9,"completion_tokens":12,"total_tokens":21}` | 9/12/21 |
| 历史键（防御） | `{"prompt":9,"completion":12,"total":21}` | 9/12/21（兼容存储形状反序列化） |
| 两者皆无 | `{}` | 零值结构，不报错 |
| 非法 JSON | `not-json` | 返回 error（encoding/json 语义） |
| `usage` 键缺失 / `usage: null` | 响应顶层 | `parsed.Usage == nil` → encodeTokenUsage 返回 "" → LogCall Omit → NULL |

### 边界值

- `total_tokens: 0` 且块存在：按块值落零。理论上仅空响应会产出，而 chat 空响应已被 `empty_response` 归一为失败、embed 空输入被拒——识别为不可达边界，**不加特判**（避免为不可达路径增加分支）。
- 极大值（>2^31）：`*int` 平台字长足够，无截断风险。

### 测试盲区备忘

`fakeProviderClient` 绕过 HTTP 解析层——本次 bug 正是从这里溜掉的。wire 解析断言必须落在 httptest 真 HTTP 路径（`provider.BaseURL` 指向测试 server），不得只用 fake client。
