## Context

动机见 proposal.md（2026-09-28 实测：单开 embedding + 云端 deepseek 可达时，本地打标链路全死但健康门仍开 → tag worker 照跑 → heuristic 降级标签入库）。本 change 只改健康判定口径，不改探测机制。

现状（`backend-go/internal/platform/aihealth/aihealth.go`）：

- `runProbeLocked` 为每条 enabled 且有启用 provider 链路的路由产出一条 `RouteHealth` 快照条目（含 `Capability` 与主 provider 可达性），随后以 `hasReachableCapability(entries, "embedding") && hasReachableCapabilityOtherThan(entries, "embedding")` 计算 healthy——即「双类各一条」宽松或运算；
- 快照条目天然承载 fail-closed 所需信息：路由未配置 / 全链路禁用 → 探测循环跳过 → `entries` 中**不存在**该 capability 的条目；
- 判定下游：`analysispause.IsPaused() = UserPaused() || !aihealth.Healthy()`，健康 API（`GET /api/ai/health`）与 60s 心跳复检共用同一快照。

## Goals / Non-Goals

**Goals:**

- 判定口径改为「核心 capability 集合每者 ≥1 条主 provider 可达」：`{embedding, topic_tagging}`；
- 打标链路不可用（路由缺失 / 未绑定 / 主不可达）→ 一律 NOT 健康（fail-closed）；
- 快照明细、健康 API 返回结构、探测/拉起/心跳/去抖机制零变化。

**Non-Goals:**

- 不做按 capability 分门调度（哪类 job 由哪条路由决定停不停）——全局二值语义保留；
- 不改 `analysispause` 组合公式与前端通知警示逻辑；
- 不为「是否把 topic_tagging 主当分析类 job 的门」做细粒度区分——分析 job 一并停是已知取舍（proposal Impact 已记录）。

## Decisions

**D1：判定实现 = 核心 capability 常量集合 + 逐者查快照，替换 or 运算。**
`healthy = 对 coreHealthCapabilities（`[]string{embedding, topic_tagging}`）中每个 capability，entries 中至少有一条该 capability 且 Reachable`。天然覆盖三种 fail-closed 输入（路由不存在 / 链路全禁用 / 探测失败），无需独立分支。
备选 a：保留原 or 语义再 AND 一条 topic_tagging —— 拒绝，两套口径叠加难解释、难测；备选 b：只硬编码 topic_tagging 单路由 ID —— 拒绝，按 capability 查与现有 capability 路由体系一致，且未来扩核心集合只改常量。
注：现实配置为每 capability 单路由，「≥1 条」与 spec 中 topic_tagging 单数表述等价；若未来出现同 capability 多路由，「≥1 主通即该 capability 通」是合理的聚合语义。

**D2：快照明细全量保留，仅 healthy 布尔变。**
所有路由（含 digest_polish / data_enrichment_* 等退出判定的）继续进 `entries`，健康 API 与设置页卡片展示不变。备选：只探测核心路由、非核心不再探 —— 拒绝，明细展示依赖全量探测，且探测成本可忽略（每 provider 一次 /models）。

**D3：判定函数保持纯函数形状，测试注入沿用现有 seams。**
`hasReachableCapability` 系辅助函数风格延续（纯 entries 输入），`probeFn` / `SetSnapshotForTest` 注入点不动；`health_gate_compose_test.go` 的 3 个 compose 场景按新口径改写，新增「云端可达不救命」回归场景对齐 spec 锁定的 2026-09-28 实测。

**D4：常量而非配置。**
核心 capability 集合写死在代码常量。单用户系统，做成 `ai_settings` 配置项是过度设计；若未来需要，扩常量为配置是纯增量改动，不影响本 spec。

## Risks / Trade-offs

- [本地 LLM 长期不开 → 门长期关，分析与打标全停（含 daily_report 等本可走云端的任务）] → 有意取舍（proposal Impact）；缓解已有：`auto_start_models` + provider `start_command` 自动拉起、60s 心跳自愈、通知中心「意图运行但 !健康」警示。
- [用户实验性删/禁 topic_tagging 路由会让门直接关] → fail-closed 是设计意图，spec「打标路由未配置不健康」场景显式锁定；设置页健康明细可看到该 capability 条目缺失。
- [心跳去抖窗口内（连续 2 次失败）判定滞后] → 既有机制，语义不变，不新增风险。
- [spec 中 topic_tagging 单数 vs 实现按 capability 聚合] → 现实单路由等价；design 记录，若未来多路由以本节 D1 语义为准，届时同步 spec 措辞。

## Migration Plan

无数据迁移、无 API 变更。部署即生效；回滚 = revert 该 commit（判定逻辑单点，无状态残留——快照为内存态，重启即重建）。

## Open Questions

（无——口径、fail-closed 边界、常量 vs 配置均已在上文决策；厂商接入类问题属于 response-format-adaptation 的 design 范畴，与本 change 无关。）
