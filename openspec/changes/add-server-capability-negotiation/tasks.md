## 1. clientmode 包（档位 + job 分组映射 + 派生）

- [ ] 1.1 新建 `backend-go/internal/platform/clientmode/clientmode.go`：档位枚举（`off` / `tier1` / `tier2`）+ 解析（环境变量 `SYNTOPICA_CLIENT_MODE` > 本地配置文件 > 缺省 `off`）+ 非法值回退 `off` 并记警告日志。包置于 `platform` 下作叶子包，只依赖标准库与 `internal/platform/logging`，不 import `internal/app` / `internal/admin`（防成环），验证：`cd backend-go && go build ./internal/platform/clientmode/...` 退出码 0，且 `go list -deps ./internal/platform/clientmode` 输出不含 `syntopica-backend/internal/app`
- [ ] 1.2 同包内建 job→能力分组映射表与各档位的关闭分组集合：`tier1` 关闭 4 组共 8 个 job（发现四件套 4 个 + 周期汇总 lifeline 3 个 + 版块升级建议 1 个），逐项对齐 `docs/srs/syntopica-client/iter0-findings.md` 0-4 节定稿矩阵；`relation_expire` 归维护类、tier1 保留开启，验证：单测断言 `tier1` 关闭集合展开后**恰为**那 8 个 job 名（不多不少）
- [ ] 1.3 同包内实现能力分组可用性派生函数（输入已注册 job 集合 → 输出分组可用性；分组可用 = 该组全部 job 均已注册），验证：单测覆盖「组内缺一个 → 不可用」「全部注册 → 可用」
- [ ] 1.4 档位解析与派生逻辑单测（优先级 / 非法值回退 / 分组派生 / `relation_expire` 归类），验证：`cd backend-go && go test ./internal/platform/clientmode/...` 退出码 0

## 2. 注册期过滤接入

- [ ] 2.1 `backend-go/internal/app/runtime.go`：**17 个实际注册点**接入分组过滤（`StartRuntime` 内 14 处直接 `registry.Register` + `registerDiscoveryV2Jobs` 内 3 处；`lifeline_weekly` 已在源码中注释停用，不计入）；`tagging.StartAllWorkers()` 调用点同样按分组过滤，验证：单测断言 `tier1` 下注册表**不含**那 8 个被关 job、**含**其余 job
- [ ] 2.2 `backend-go/internal/tagmanagement/service/core/workers.go`：3 个 worker（TagQueue / Embedding / MergeReembedding）启动点接入同一档位判断，验证：单测断言被关闭分组的 worker 未被启动
- [ ] 2.3 启动期留痕：打印生效档位与被关闭分组列表（一条日志，含 `client_mode=` 与分组名），验证：启动日志含 `client_mode=` 且含被关闭分组名
- [ ] 2.4 漂移测试：`runtime.go` 中每个 `registry.Register` 的 job 名都必须在 clientmode 分组映射表中可查（防止新增 job 忘归组导致各档位行为失真），验证：`cd backend-go && go test ./internal/app/...` 退出码 0，且漂移用例存在于 `backend-go/internal/app/runtime_test.go`

## 3. capabilities 自述端点

- [ ] 3.1 新建 `backend-go/internal/app/capabilities_handler.go`：handler 实现（档位字段 + job 清单取调度注册表 `OrderedNames` + 能力分组可用性派生 + AI 层自述 + embedding 空间标识）；进程内零注册 job 时返回 200 + 空数组（不得 5xx），验证：handler 单测覆盖「`off` 全量」「`tier1` 过滤后少 8 项且对应分组不可用」「零 job → 200 + `[]`」
- [ ] 3.2 `backend-go/internal/app/router.go`：在 `api` 组内注册 `GET /api/capabilities`；不加 token 鉴权、不为 demo 只读模式开特例，验证：`curl -s -o /dev/null -w "%{http_code}" http://127.0.0.1:5100/api/capabilities` 输出 `200`
- [ ] 3.3 AI 层自述接 `aihealth.GetSnapshot()` 的进程内快照，端点**不触发额外健康探测**（不现场打 vendor），验证：单测断言首次探测未完成时报告不可用且探测调用次数为 0
- [ ] 3.4 embedding 空间标识取 `embedding_config` 表的 `embedding_model` / `embedding_dimension` 两个 key（不硬编码模型名与维度），验证：响应体含 `embedding_space.model` 与 `embedding_space.dimension`，且值与表中一致
- [ ] 3.5 端点 HTTP 契约单测（200 状态、字段齐、无 5xx 分支），验证：`cd backend-go && go test ./internal/app/...` 退出码 0

## 4. 测试

- [ ] 4.1 影响包测试（只跑受影响包，不跑全量），验证：`cd backend-go && go test ./internal/platform/clientmode/... ./internal/app/... ./internal/tagmanagement/service/core/...` 退出码 0
- [ ] 4.2 后端质量门禁，验证：`cd backend-go && golangci-lint run ./...` 退出码 0、`go vet ./...` 退出码 0、`go build ./...` 退出码 0

## 5. 文档

<!-- doc-impact: flow, api, configuration -->

- [ ] 5.1 `docs/reference/api/system.md`：新增 `GET /api/capabilities` 段（响应字段表 + 示例响应）
- [ ] 5.2 `docs/reference/api/_index.md`：`system.md` 行的端点清单补 `/api/capabilities`
- [ ] 5.3 `docs/reference/configuration.md`：新增 `SYNTOPICA_CLIENT_MODE` 配置项（三态取值、缺省 `off`、非法值回退、切换需重启、两种装法的配置位置差异）
- [ ] 5.4 `docs/reference/flow/scheduler.md`：补「客户端档位与注册期过滤」节（关闭分组清单 + `relation_expire` 归类 + 无调度器时状态端点返回 200 空集合）
- [ ] 5.5 变更溯源：archive 后按 §12.2 补 flow 变更溯源链接

## 6. 验证

- [ ] 6.1 `cd backend-go && go test ./internal/platform/clientmode/... ./internal/app/... ./internal/tagmanagement/service/core/...` → 退出码 0，输出 `ok` 各包
- [ ] 6.2 `cd backend-go && golangci-lint run ./...` → 退出码 0，无告警输出
- [ ] 6.3 `cd backend-go && go vet ./...` → 退出码 0
- [ ] 6.4 `cd backend-go && go build ./...` → 退出码 0
- [ ] 6.5 `openspec validate add-server-capability-negotiation --strict` → 退出码 0
- [ ] 6.6 `bash scripts/harness/scenario-trace.sh openspec/changes/add-server-capability-negotiation` → 退出码 0
- [ ] 6.7 `bash scripts/harness/doc-impact.sh verify openspec/changes/add-server-capability-negotiation` → 退出码 0
- [ ] 6.8 `bash scripts/harness/check-standards.sh --change add-server-capability-negotiation` → 退出码 0
- [ ] 6.9 手工冒烟（缺省档位）：起后端 → `curl -s http://127.0.0.1:5100/api/capabilities` → HTTP 200，响应含 `client_mode: "off"`，job 清单非空
- [ ] 6.10 手工冒烟（tier1 档位）：`SYNTOPICA_CLIENT_MODE=tier1` 起后端 → 同一端点 → `client_mode: "tier1"`，job 清单比 6.9 少 8 项，对应能力分组可用性为 false
- [ ] 6.11 `bash scripts/harness/review-trace.sh openspec/changes/add-server-capability-negotiation` → 退出码 0

### Scenario → 测试文件

| Scenario | 测试文件 |
| --- | --- |
| 完整服务器形态自述全量能力 | `backend-go/internal/app/capabilities_handler_test.go` |
| 档位过滤后只自述实际存在的能力 | `backend-go/internal/app/capabilities_handler_test.go` |
| 无调度器时以空集合表达而非错误码 | `backend-go/internal/app/capabilities_handler_test.go` |
| job 清单由注册表自动派生 | `backend-go/internal/app/capabilities_handler_test.go` |
| 能力分组可用性由实际注册情况派生 | `backend-go/internal/platform/clientmode/clientmode_test.go` |
| 档位来源优先级为环境变量优先 | `backend-go/internal/platform/clientmode/clientmode_test.go` |
| 非法档位值回退缺省且不阻断启动 | `backend-go/internal/platform/clientmode/clientmode_test.go` |
| 被关闭分组的调度 job 不进入进程 | `backend-go/internal/app/runtime_test.go` |
| 被关闭分组的后台 worker 不启动 | `backend-go/internal/tagmanagement/service/core/workers_test.go` |
| 档位切换需要重启才生效 | `backend-go/internal/app/runtime_test.go` |
| 启动时留下档位决策痕迹 | `backend-go/internal/app/runtime_test.go` |
| 路由可达性取自进程内健康快照 | `backend-go/internal/app/capabilities_handler_test.go` |
| 首次探测未完成时保守报告不可用 | `backend-go/internal/app/capabilities_handler_test.go` |
| 携带 embedding 空间标识 | `backend-go/internal/app/capabilities_handler_test.go` |

- [ ] 6.12 Review：派 `change-review` 子线程只读审查全量制品与 diff，意见按 High/Medium/Low 处置后在 `review.md` 建账（格式见 `openspec/specs/review-trace-gate`），验证：`bash scripts/harness/review-trace.sh openspec/changes/add-server-capability-negotiation` 退出码 0
