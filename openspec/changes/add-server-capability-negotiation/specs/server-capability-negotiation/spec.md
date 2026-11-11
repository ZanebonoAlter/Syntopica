## Purpose

定义服务端「能力自述」契约：`GET /api/capabilities` 报告本进程实际具备的能力，以及客户端档位（`client_mode`）三态的注册期过滤行为。它是客户端形态（瘦客户端 / 本地 sidecar / 远程壳）与服务端之间唯一的协商入口——前端据此决定功能入口的显示与降级，替代当前「硬假定所访问的机器就是干活的机器」的猜测式行为。

## ADDED Requirements

### Requirement: 服务端提供能力自述端点

系统 SHALL 提供只读端点 `GET /api/capabilities`，返回本进程的能力自述 JSON。响应 SHALL 至少包含：档位字段、已注册 job 清单、能力分组可用性、AI 层可用性。

#### Scenario: 完整服务器形态自述全量能力

- **WHEN** 进程以缺省档位（`off`）启动
- **THEN** 端点 SHALL 返回 200
- **AND** 档位字段 SHALL 为 `off`
- **AND** job 清单 SHALL 包含进程内全部已注册 job
- **AND** 全部能力分组 SHALL 报告为可用

#### Scenario: 档位过滤后只自述实际存在的能力

- **WHEN** 进程以 `tier1` 档位启动
- **THEN** 返回的 job 清单 SHALL NOT 包含被该档位关闭分组的任何 job
- **AND** 被关闭分组的能力可用性 SHALL 报告为不可用
- **AND** 未被关闭分组的能力可用性 SHALL 报告为可用

#### Scenario: 无调度器时以空集合表达而非错误码

- **WHEN** 进程内没有任何已注册调度 job
- **THEN** 端点 SHALL 返回 200
- **AND** job 清单 SHALL 为空数组
- **AND** SHALL NOT 返回 5xx

#### Scenario: job 清单由注册表自动派生

- **WHEN** 自述端点构造 job 清单
- **THEN** 清单内容 SHALL 等于进程内调度注册表的已注册项集合
- **AND** 实现 SHALL NOT 维护与注册表并列的第二份 job 清单

#### Scenario: 能力分组可用性由实际注册情况派生

- **WHEN** 某能力分组下的全部 job 均未注册
- **THEN** 该分组 SHALL 报告为不可用
- **AND** 当该分组下至少一个 job 已注册时 SHALL 报告为可用

### Requirement: 客户端档位三态与注册期过滤

系统 SHALL 支持 `off` / `tier1` / `tier2` 三种档位。档位 SHALL 在**注册期**决定哪些调度 job 与哪些后台 worker 进入进程——被关闭的对象 SHALL NOT 在进程内存在其 goroutine。

#### Scenario: 档位来源优先级为环境变量优先

- **WHEN** 环境变量与本地配置文件同时声明了档位
- **THEN** 环境变量的值 SHALL 生效

#### Scenario: 非法档位值回退缺省且不阻断启动

- **WHEN** 配置的档位值不属于三态之一
- **THEN** 进程 SHALL 以 `off` 档位启动
- **AND** SHALL 记录一条警告日志
- **AND** SHALL NOT 因档位非法而启动失败

#### Scenario: 被关闭分组的调度 job 不进入进程

- **WHEN** 档位关闭某能力分组
- **THEN** 该分组下的 job SHALL NOT 被注册到调度注册表
- **AND** 进程内 SHALL NOT 存在该 job 的调度 goroutine
- **AND** 调度状态端点 SHALL NOT 列出该 job

#### Scenario: 被关闭分组的后台 worker 不启动

- **WHEN** 档位关闭某能力分组
- **THEN** 该分组下的后台 worker SHALL NOT 启动
- **AND** 进程内 SHALL NOT 存在该 worker 的 goroutine

#### Scenario: 档位切换需要重启才生效

- **WHEN** 运行中进程所属的档位配置被外部修改
- **THEN** 进程 SHALL 保持原档位对应的行为直至重启

#### Scenario: 启动时留下档位决策痕迹

- **WHEN** 进程启动并完成档位解析
- **THEN** SHALL 记录一条包含生效档位与被关闭分组的日志

### Requirement: AI 层与 embedding 空间自述

自述端点 SHALL 报告 AI 层的可用性与当前 embedding 空间标识。

#### Scenario: 路由可达性取自进程内健康快照

- **WHEN** 端点构造 AI 层自述
- **THEN** 各路由的可达性 SHALL 取自进程内健康快照的既有数据
- **AND** 端点 SHALL NOT 触发额外的健康探测

#### Scenario: 首次探测未完成时保守报告不可用

- **WHEN** 健康快照尚未完成首次探测
- **THEN** 端点 SHALL 报告相关 AI 能力不可用
- **AND** SHALL NOT 因此返回错误码

#### Scenario: 携带 embedding 空间标识

- **WHEN** 端点返回
- **THEN** 响应 SHALL 携带当前 embedding 空间的模型标识与维度
- **AND** 该标识 SHALL 取自既有 embedding 配置存储，而非硬编码常量
