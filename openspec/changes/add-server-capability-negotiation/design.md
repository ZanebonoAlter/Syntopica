## Context

- **前端当前无形态感知**：`front/app` 与 `backend-go` 全仓 `client_mode|tier1|tier2` 0 命中。前端硬假定「所访问的机器就是干活的机器」，代价是五处硬伤（日报生成点不动、运行状态面板空、文章状态恒「已完成」、`isAIEnabled` 恒 false、发现页无降级位）。
- **档位矩阵已定稿**：`docs/srs/syntopica-client/iter0-findings.md` 0-4 节已定 `client_mode` 三态与档位差异（8 个供给侧 job 不跑、embedding/chat 路由改指向）。本 change 落地该定稿的**服务端代码侧**，不含向量/chat 路由指向（那是配置面，用户自填 vendor URL）。
- **调度注册表已有自动发现能力**：`backend-go/internal/platform/scheduler/registry.go:OrderedNames` 已被调度状态端点用于渲染清单（scheduler 域红线①：不得维护第二份 descriptor 列表）。
- **job 关闭的代价点已知**：iter0-findings 0-4 载明「关一个 job 要动几处代码」——调度器类 1 处（注册点），队列 worker 类另 1 处（`core/workers.go`）。
- **健康快照是进程内既有能力**：`backend-go/internal/platform/aihealth/aihealth.go:GetSnapshot` 已有 60s 心跳维护的路由可达性快照，端点复用即可，不必额外探测。
- **本 change 是客户端形态的地基**：后续的 `/api/capabilities` 消费方（前端）、素材包同步协议（迭代 2）、多客户端 token 鉴权，都依赖此处的契约先定下来。

## Goals / Non-Goals

**Goals:**

- 交付一个**只读、自述、无副作用**的能力协商端点，前端据此决定功能入口的显示与降级。
- 落地 `client_mode` 三态的**注册期过滤**：被关闭的 job / worker 不进入进程（无 goroutine、无注册项）。
- 让「job 清单」「能力分组可用性」「档位决策」三件事各有**唯一真相源**，不产生第二份清单。
- 现有服务器装法**行为零变化**（缺省 `off`，全部注册、全部可用）。

**Non-Goals:**

- 前端消费端点（改 `front/`）——另开 change。
- 素材包同步协议、cursor、embedding 空间版本协商——迭代 2 另开 change（本 change 只自述空间的模型标识与维度）。
- 多客户端 token 鉴权——另开 change（本端点不暴露任何用户数据）。
- 档位「热切换」——按 0-4 定稿，档位切换 = 改配置 + 重启。
- 供给侧 job 的**能力实现**（如发现四件套本身）——本 change 只管开关，不管实现。

## Decisions

### D1. 端点落在 `/api` 组内的 `GET /api/capabilities`

- **结论**：在 `api` 路由组注册 `GET /api/capabilities`，响应为 JSON。
- **落点**：`backend-go/internal/app/router.go`
- **取舍**：不落根级——根级只有 `/health`、`/icons/*`、`/ws`、`/api/tasks/status` 四个公共面，新增根级端点会让公共面扩散；不落 `/api/admin/*`——admin 组是运维视角，且未来多客户端引入 token 鉴权时 admin 需要独立收口，能力自述端点是**客户端契约**，不该混进 admin。

### D2. job 清单从调度注册表派生，不维护第二份清单

- **结论**：自述端点的 job 清单直接取调度注册表的已注册项集合。
- **落点**：`backend-go/internal/platform/scheduler/registry.go:OrderedNames`
- **取舍**：不新建 job descriptor 表。代价是「能力分组」需要知道每个 job 属于哪一组（见 D3），但那是**分类映射**不是**清单**——分类表不重复列举「有哪些 job」以外的信息，且可被漂移测试守住（见 D12 风险项）。

### D3. job→能力分组的映射与档位开关同置一个叶子包

- **结论**：新建 `backend-go/internal/platform/clientmode/` 包，内含：档位枚举与解析、job→能力分组映射表、各档位关闭的分组集合、分组可用性派生函数。
- **落点**：任务 1.2
- **取舍**：不在 `runtime.go` 的 17 个注册点就地写 `if`。就地判断能实现「关 job」，但**派生出不了分组可用性**（前端需要的是「发现四件套这套能力有没有」，不是 17 个布尔量），且 17 处重复条件必然漂移。

### D4. 过滤点在注册期，不引入运行时开关

- **结论**：被档位关闭的 job 直接不调用 `registry.Register`，被关闭的 worker 直接不启动——进程内不存在对应 goroutine。
- **落点**：任务 2.1
- **取舍**：不用「注册了但在 job 内部检查开关跳过」的做法。那样 goroutine 仍在、仍会 tick、仍可能 lease/写状态，与 iter0-findings 0-4 定的「注册期过滤（不注册=进程无此 goroutine）」相悖，且客户端形态下这些 job 本就不该产生任何状态行。

### D5. worker 过滤是独立的第二处接入点

- **结论**：除调度注册点外，队列 worker 池（3 个 worker）另有一处启动点需接入同一档位判断。
- **落点**：`backend-go/internal/tagmanagement/service/core/workers.go`
- **取舍**：不把 worker 塞进调度注册表。worker 不是调度 job（无 cron、无状态端点条目），注册表语义是「调度器」；硬塞会污染调度状态端点。

### D6. 分组可用性 = 该组全部 job 均已注册

- **结论**：某能力分组可用，当且仅当该组下的全部 job 都在注册表中。
- **落点**：任务 1.3
- **取舍**：用「任一注册即视为可用」被否——分组要表达的是整条能力链是否完整（例如发现四件套缺一半比全关更糟）。但若某组本身只有一个 job（如版块升级建议），两种规则等价，无歧义。

### D7. 档位配置源：环境变量 > 本地配置文件 > 缺省 `off`

- **结论**：档位从环境变量读（服务器装法），其次本地配置文件（桌面客户端），都没有则 `off`。
- **落点**：任务 1.1
- **取舍**：不存 `ai_settings` 表。档位是**启动期开关**——存库需要迁移，且语义上属于部署形态而非运行时设置；桌面端本地库与服务器库对同一档位的取值本就应当独立。

### D8. AI 层自述取进程内健康快照，端点不触发探测

- **结论**：各路由可达性取 `aihealth.GetSnapshot()` 的既有快照；快照未就绪时保守报告不可用。
- **落点**：`backend-go/internal/platform/aihealth/aihealth.go:GetSnapshot`
- **取舍**：不在端点里现场探 vendor。端点会被前端轮询调用，现场探测会把自述端点变成放大器（每次调用打一轮 vendor），而 `aihealth` 已有 60s 心跳维护这份快照。代价是可达性有最长 60s 的滞后——对「能力有无」这一量级完全够用。

### D9. embedding 空间标识读既有配置存储，不硬编码

- **结论**：embedding 空间的模型标识与维度取 `embedding_config` 表的 `embedding_model` / `embedding_dimension` 两个 key。
- **落点**：`backend-go/internal/tagmanagement/models/embedding_config.go`
- **取舍**：不硬编码 `qwen3-embedding-4b` / `2560`。真库实际值与 seed 默认值不同（seed 默认维度是 1024，生产是 2560），硬编码会自述出一个假空间；空间**版本号**（v1/v2）随素材包协议走，不在本 change。

### D10. 自述端点不引入鉴权

- **结论**：`/api/capabilities` 走既有 `/api` 组中间件，不加 token 校验。
- **落点**：任务 3.2
- **取舍**：多客户端 token 鉴权另开 change（届时统一收口）。本端点只回答「本进程具备什么」，不暴露任何用户数据（无私有源列表、无阅读行为、无版块声明），与 SRS 的「服务端用户状态最小化」不冲突。

### D11. `DEMO_READ_ONLY` 语义不变，不为 capabilities 开特例

- **结论**：demo 只读模式下 capabilities 仍可访问（GET 本就被允许）。
- **落点**：`backend-go/internal/platform/middleware/readonly.go`
- **取舍**：不为该端点加白名单或特例分支。demo 模式本就是「只允许浏览」，自述端点属于浏览面。

### D12. 无调度器时返回 200 + 空集合

- **结论**：进程内零注册 job 时，端点返回 200，job 清单为空数组。
- **落点**：任务 3.1
- **取舍**：不用 404/5xx 表达「无调度器」。scheduler 域红线⑧明确要求状态端点在无调度器时返回 200 + 空集合。

## Risks / Trade-offs

- **分组映射表与注册点漂移**：新增 job 忘记归组 → 该 job 永远在所有档位开启，且分组可用性派生失真。对策：加漂移测试——`runtime.go` 中每个 `registry.Register` 的名字都必须能在分组映射表中找到（任务 2.4）。
- **`clientmode` 包位置约束**：它被 `runtime.go`、`core/workers.go`、capabilities handler 三处引用，若它 import 了 `app`/`admin` 会成环。对策：约束为 `platform` 下的叶子包，只依赖标准库与日志包（任务 1.1）。
- **误关生产 job**：档位解析出错或配置写错可能关掉本该跑的 job。对策：①非法值回退 `off`（最保守 = 全开）而非回退 `tier1`；②启动日志打印生效档位与被关闭分组；③现有部署全部缺省 `off`，行为零变化。
- **档位过滤与 `relation_expire` 归组**：0-4 的定稿矩阵未点名 `relation_expire`（关系过期清理）。本 change 归它入维护类、tier1 保留开启（与数据层同性质：无模型依赖、纯本地清理），并在 flow 文档同步该归类，避免下次有人重新拍板。
- **能力分组粒度可能后续增补**：分组是本 change 新概念，前端消费时若发现粒度不对需要增补分组。分组表是纯数据，增补不改代码结构；变更走显式 change。
