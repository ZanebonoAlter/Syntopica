# ai-model-health Specification

## Purpose
TBD - created by archiving change ai-model-health-gate. Update Purpose after archive.

## Requirements

### Requirement: 本地模型自动拉起总开关

系统 SHALL 提供 `auto_start_models` 配置（持久化于 ai_settings，默认 false）作为本地模型自动拉起的总开关。当某被探测的主 provider 不可达 且 `auto_start_models=true` 且该 provider 配置了 `start_command` 时，系统 SHALL 以脱离父进程的方式执行该启动命令（Windows 经 `cmd /c`、*nix 经 `sh -c`），并在执行后轮询探测其可达性（上限约 45 秒）。`auto_start_models=false` 时系统 SHALL 仅检测可达性、SHALL NOT 执行任何启动命令。系统 SHALL NOT 托管被拉起进程的生命周期（不记录 PID、不在后端退出时终止、不崩溃自动重启）。慢启动模型（加载超出 45 秒轮询窗口）会使「探活成功则不拉」的抑制失效，因此系统 SHALL 额外满足以下防重复拉起约束：

- 同一次探测内，同一 provider 被多条路由共享时 SHALL 只探测/拉起一次，各路由复用该结果；
- 成功执行 start_command 后 SHALL 记录拉起时间（进程内存态）；冷却窗口（默认 10 分钟）内的后续探测发现该 provider 仍不可达时 SHALL NOT 再次执行 start_command，SHALL 仅继续轮询已拉起进程；冷却过期后仍不可达的，下次探测 MAY 再次拉起；
- 探测 SHALL 全局互斥：已有探测在跑时新触发的探测（如恢复分析触发的 reprobe）SHALL 直接跳过，不并发探测。

#### Scenario: 总开关关时不拉起

- **WHEN** auto_start_models=false，启动探测某 provider 不可达且配了 start_command
- **THEN** 系统 SHALL NOT 执行 start_command，快照记录该 provider reachable=false、launched_by_backend=false

#### Scenario: 总开关开且不通时拉起并复探

- **WHEN** auto_start_models=true，启动探测本地 llama.cpp provider 不可达且配了 start_command
- **THEN** 系统 SHALL 脱离执行 start_command，随后轮询探测；若在 ~45s 内变可达，快照记录 reachable=true、launched_by_backend=true

#### Scenario: 无 start_command 的 provider 不被拉起

- **WHEN** auto_start_models=true，某不可达 provider 未配 start_command
- **THEN** 系统 SHALL NOT attempts 执行任何命令，记录 reachable=false、launched_by_backend=false

#### Scenario: 已可达则不重复拉起

- **WHEN** 启动探测某 provider 已可达（如旧进程仍占端口）
- **THEN** 系统 SHALL NOT 执行其 start_command，记录 launched_by_backend=false

#### Scenario: 进程生命周期不被托管

- **WHEN** 系统通过 start_command 拉起了一个本地进程
- **THEN** 系统 SHALL NOT 记录其 PID、SHALL NOT 在后端退出时终止它、SHALL NOT 在其崩溃后自动重启

#### Scenario: 同 provider 多路由共享时只拉起一次

- **WHEN** auto_start_models=true，embedding 与 llm 两条路由的主 provider 为同一个配置了 start_command 的本地 provider，且其在 45 秒轮询窗口内始终不可达（模型仍在加载）
- **THEN** 系统 SHALL 只执行一次 start_command（而非每条路由各一次），两条路由共享该探测结果

#### Scenario: 冷却窗口内 reprobe 不再拉起

- **WHEN** 某 provider 的 start_command 已被成功执行且仍在冷却窗口（默认 10 分钟）内，一次新探测（如用户点击恢复分析）发现其仍不可达
- **THEN** 系统 SHALL NOT 再次执行 start_command，SHALL 继续轮询等待已拉起进程就绪

#### Scenario: 冷却过期后可再次拉起

- **WHEN** 某 provider 距上次成功拉起已超过冷却窗口且仍不可达
- **THEN** 下次探测 MAY 再次执行其 start_command

#### Scenario: 探测互斥，进行中时不重复探测

- **WHEN** 一次探测仍在进行中（如启动探测仍在 45 秒轮询窗口内），用户点击恢复分析触发 reprobe
- **THEN** 新触发的探测 SHALL 被跳过（不并发探测、不重复拉起），快照由进行中的探测完成更新

### Requirement: 健康就绪判定（宽松）

系统 SHALL 以宽松口径判定整体「健康」：embedding 类路由中至少有一条的主 provider 可达，且 llm 类（capability 非 embedding）路由中至少有一条的主 provider 可达。两条任一不满足 SHALL 判定 NOT 健康。单条非关键路由不可达 SHALL NOT 单独导致 NOT 健康（只要仍满足上述两类各至少一条可达）。

#### Scenario: 两类各通一条即健康

- **GIVEN** embedding 路由主 provider 可达，summary 路由主 provider 可达，digest_polish 路由主 provider 不可达
- **WHEN** 健康判定计算
- **THEN** 系统 SHALL 判定 healthy=true（digest_polish 不通不推翻整体，因其明细仍展示在快照）

#### Scenario: 缺 embedding 则不健康

- **WHEN** 所有 embedding 路由主 provider 均不可达，但存在可达的 llm 路由
- **THEN** 系统 SHALL 判定 healthy=false

#### Scenario: 缺 llm 则不健康

- **WHEN** embedding 路由主 provider 可达，但所有 llm 路由主 provider 均不可达
- **THEN** 系统 SHALL 判定 healthy=false

#### Scenario: 未配置任何路由不健康

- **WHEN** 系统无任何 enabled 且绑定了 provider 的路由
- **THEN** 系统 SHALL 判定 healthy=false

### Requirement: 健康快照与查询 API

系统 SHALL 在内存维护一份健康快照（各路由主 provider 的可达性明细 + 整体 healthy + 上次检测时间），并提供 `GET /api/ai/health` 返回该快照与 `auto_start_models` 当前值。系统 SHALL 提供 `PUT /api/ai/health/auto-start-models`（body `{enabled:bool}`）更新 `auto_start_models` 并返回新值。系统 SHALL 提供 `POST /api/ai/health/reprobe` 手动触发异步重探（幂等，探测 in-flight 时跳过，不改变分析暂停状态）。`/schedulers/status` 顶层 SHALL 增量返回 `ai_healthy` 布尔与各路由主 provider 的简明通断明细，供前端无需额外请求即可渲染提示。

#### Scenario: 查询健康快照

- **WHEN** 调用 GET /api/ai/health
- **THEN** 返回 { healthy, checked_at, auto_start_models, routes:[{route_name, capability, primary_provider, model_kind, reachable, launched_by_backend, last_checked, error}] }

#### Scenario: 设置自动拉起总开关

- **WHEN** 调用 PUT /api/ai/health/auto-start-models，body { enabled: true }
- **THEN** 系统 SHALL 持久化 auto_start_models=true 并返回 { enabled: true }

#### Scenario: 手动触发异步重探

- **WHEN** 调用 POST /api/ai/health/reprobe，无探测 in-flight
- **THEN** 系统 SHALL 异步启动一次 RunStartupProbe 并立即返回（triggered=true），快照由探测完成时更新

#### Scenario: 探测 in-flight 时手动重探跳过

- **WHEN** 调用 POST /api/ai/health/reprobe，恰有一次探测 in-flight
- **THEN** 系统 SHALL 返回 skipped=true，SHALL NOT 并发探测

#### Scenario: 调度器状态携带健康摘要

- **WHEN** 调用 GET /schedulers/status
- **THEN** 响应顶层 SHALL 包含 ai_healthy 布尔与各路由主 provider 的简明通断明细

### Requirement: 健康快照内存态与启动竞态

健康快照 SHALL 为进程内存态（不持久化，重启后重建）。后端启动后、首次健康检测完成前，快照 SHALL 处于未就绪态（healthy 视为 false、checked_at 为空），以使分析类任务在模型就绪度未知时不运行。该未就绪态 SHALL 在首次检测完成后被实际结果取代。

#### Scenario: 启动竞态期分析不跑

- **WHEN** 后端刚启动、首次健康检测尚未完成
- **THEN** healthy SHALL 视为 false，分析类任务 SHALL 不 lease（由健康门硬执行保证）

#### Scenario: 检测完成后快照就绪

- **WHEN** 首次 RunStartupProbe 完成
- **THEN** 快照 SHALL 写入各路由明细、checked_at=now、healthy 按宽松口径计算

### Requirement: 启动时模型健康检测与心跳复检

系统 SHALL 在后端启动时执行一次模型健康检测：遍历每条 enabled 且已绑定 provider 的 AI 路由，取其 priority 最高（第一个）provider，复用 `airouter.TestConnection`（GET `{base_url}/models`，零推理 token，HTTP 超时取 provider 自身 `timeout_seconds`，未配置兜底 15 秒）探测可达性，结果写入内存健康快照。检测 SHALL 只针对每条路由的第一个 provider，SHALL NOT 穷举该路由的 fallback provider。启动检测之后，系统 SHALL 由后台定时心跳器（见 ai-health-reprobe 能力）无论快照 healthy 与否按固定间隔周期性复检——快照可被连续 2 次探测失败降级为 not healthy，也可被单次成功恢复 healthy，期间 SHALL 复用同一全局探测互斥与拉起冷却约束。

#### Scenario: 启动时探测每条路由主 provider

- **WHEN** 后端启动，存在 enabled 的 summary 路由（主 provider A）与 embedding 路由（主 provider B）
- **THEN** 系统 SHALL 探测 A 与 B 的可达性，并将两条结果写入健康快照

#### Scenario: 仅探主 provider 不探 fallback

- **WHEN** 某 embedding 路由绑定了主 provider A 与 fallback provider B
- **THEN** 启动健康检测 SHALL 只探测 A，SHALL NOT 探测 B

#### Scenario: 无 provider 的路由跳过

- **WHEN** 某 enabled 路由未绑定任何 provider
- **THEN** 启动健康检测 SHALL 跳过该路由（不产生快照条目，也不视为「down」条目）

#### Scenario: ListRoutes 瞬态失败时重试

- **WHEN** 启动健康检测查询路由列表（store.ListRoutes）失败（如瞬态 DB 连接错误、socket 耗尽、端口冲突）
- **THEN** 系统 SHALL 重试若干次（默认 3 次、~2s 退避），仅当反复失败才记 NOT 健康；避免单次瞬态错误永久焊死健康门，使用户点「恢复」亦无法自愈

#### Scenario: 快照健康后仍按心跳周期复检且可降级

- **WHEN** 健康快照判定 healthy
- **THEN** 系统 SHALL 由定时心跳器继续周期性复检（不再停止），复检连续 2 次失败时快照 SHALL 降级为 not healthy

#### Scenario: 快照不健康时定时复检直至自愈

- **WHEN** 启动探测后快照判定 not healthy（如本地模型加载超时），模型在启动后数分钟才加载完成
- **THEN** 系统 SHALL 由定时心跳器周期性复检，模型可达后快照 SHALL 自动更新为 healthy，无需用户干预
