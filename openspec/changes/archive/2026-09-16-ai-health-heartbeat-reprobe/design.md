# Design: AI 健康心跳重探（可降级）

## Context

现状（见 proposal.md - Why）：健康重探器单向自愈——not healthy 时 60s 重探，healthy 后停止（`aihealth.go:226-234`），spec `ai-health-reprobe` 明文"永不主动打回"。常驻拓扑（树莓派常驻采集 + PC 按需启动 AI）下，PC 中途关机 → 快照假健康 → 暂停门禁失效 → 批次持续失败烧 `completion_attempts`。

探测链现状（本 change 复用、不改动）：`airouter.TestConnection`（GET `{base_url}/models`，零 token）的 HTTP 超时**已经取 provider.timeout_seconds**（`test_connection.go:23-27`，未配置兜底 15s）；全局探测互斥 + 拉起冷却（10min）由 `RunStartupProbe` 全流程提供。

**数据依据**（开发库 ai_call_logs，2026-09-13 只读核查）：

- 成功调用延迟（近 30 天，本地 provider）：embedding p99 ≤1.2s / max 6.6s；LLM 提取 p95 13~27s / max 63s；内容补全 p50 33s / p95 78s / max 101s；版块升级建议 max 217s。
- 失败调用延迟：全部 ≈ 0s——实际"端点死亡"（PC 关机）表现为 TCP 秒拒（RST），不存在超时等待型失败。

## Goals / Non-Goals

**Goals**

- healthy 状态下持续心跳，健康可被降级；降级证据分级（秒拒 vs 超时），忙服务器零误判。
- 心跳失败复用现有拉起链路（auto_start_models / start_command / 冷却）。
- 单机随用随开用户行为零回退：启动探测、快速自愈路径不变。

**Non-Goals**

- 不改探测口径（GET /models、仅主 provider、同 provider 去重）、`Healthy()` 宽松判定、手动重探 API、AiHealthBanner。
- 不改 PauseAware 暂停覆盖面（firecrawl 维持在暂停名单）。
- 不处理在途批次的 attempts 烧毁（见 Risks 残留）。
- 不新增 ai_settings 配置面——心跳参数为代码常量。

## Decisions

### D1 · 无条件心跳，周期 60s（healthy / not healthy 同节奏）

healthy 与 not healthy 用同一 60s 循环，不搞分档。理由：探测是 LAN + 云端各一次零 token GET，分档节省的是不存在的成本，却把降级检测延迟拉长一个量级。替代方案（healthy 时 5min）已否决。

### D2 · 降级证据分级：秒拒 N=2 连败，超时由 provider 超时天然把关

失败按证据强度分两类：

- **秒拒（connection refused / RST）**：无歧义死亡证据（数据：失败延迟 ≈0s）。规则：连续 2 次失败 → 降级，最坏 ≈120s 判停。
- **探测超时（挂死/无响应）**：有歧义（忙 vs 死）。探测 HTTP 超时 = provider.timeout_seconds（现状，qwen=3000s ≫ 最长真实调用 217s）→ 慢而活着的服务器总会在超时内应答 /models、不计失败；只有真挂死才计一次失败，且第一判就等满 timeout_seconds。**provider 超时即"忙容忍窗口"，无需为忙场景引入额外去抖参数。**

实现口径：不解析错误类型做分类分支——统一"连续 2 次失败降级"即可。因为超时型失败的单次判定周期已被 provider 超时拉长（等满 3000s 才算第 1 次失败），两次连败自然覆盖挂死场景；秒拒型则快速累计两次。分类只存在于推理层面，简化实现。

替代方案否决：① flat 时间窗去抖（如"120s 内失败才降级"）——忙服务器真实调用可达 217s，时间窗要么误判要么迟钝；② 失败类型分支（RST 立即降级 / 超时等 N×timeout）——实现复杂度不值得，统一 N=2 已足够。

### D3 · 升级（恢复）保持现状：单次成功即 healthy

现状自愈路径（单次探测成功 → healthy → 门禁解除）不变。心跳循环下该语义自动覆盖"PC 开机后 ≤60s 恢复"。

### D4 · 心跳失败触发拉起：纯复用，零新逻辑

心跳直接复用 RunStartupProbe 全流程：失败 + `auto_start_models=true` + 配了 `start_command` → 拉起（10min 冷却防刷）。Pi 部署清空 start_command 后自然 no-op；单机用户语义与现状完全一致。不新增"远端唤醒"语义（ssh/wol 属部署配置，不进代码）。

### D5 · 互斥与节奏：串行心跳，天然受探测时长约束

全局探测互斥（现状）保留：心跳与手动重探/启动探测共用一把锁。心跳实际节奏 = max(60s, 上次探测耗时)。挂死端点下心跳被在途探测阻塞属预期行为（D2 的忙容忍的一部分）；PC 关机（秒拒）下探测毫秒级返回，节奏就是 60s。

## Risks / Trade-offs

- **[在途批次烧 attempts（残留，不修）]** 门禁仅批次入口检查：心跳降级后，已在跑的 content_completion 批（≤50 篇）继续失败、烧 `completion_attempts`、部分标 `failed`。心跳把假健康窗口从 ∞ 压到 ≤120s + 1 批，残留损失有界；`failed` 有整 feed 补处理入口兜底（`content_completion_handler.go:111-126`）→ 接受，不纳入本 change。
- **[挂死端点降级慢]** 无 RST 的挂死（防火墙丢包/服务器 wedged）需 2×provider.timeout_seconds（qwen 最坏 ~100min）才降级。此类故障模式在实测数据中不存在（失败全是秒拒）；且误判代价（暂停分析）优于漏判代价（烧 attempts）。→ 接受。
- **[云 provider 心跳流量]** glm 每 60s 一次 GET /models（1440 次/天）。正常 API 限额下无压力；若未来受限，调大周期是 design 参数级变更。→ 接受。
- **[降级期间用户观感]** 降级瞬间在途批次照常失败，AiHealthBanner 随 `/schedulers/status.ai_healthy` 轮询（8~30s）刷新。用户可见延迟 ≤ 轮询周期。→ 接受。

## Migration Plan

1. 实现合入后随正常发布部署；无 schema 变更、无配置迁移、无数据回填。
2. 回滚 = 回退二进制；健康快照是内存态，重启即重建，无状态残留。
3. 部署提示（常驻拓扑）：base_url 改 PC 局域网地址；start_command 清空（或配 ssh 唤醒）；均属部署配置，不在本 change。

## Open Questions

（无——参数均为代码常量：`heartbeatInterval=60s`、`degradeFailures=2`，落在 aihealth 包内，改动即设计变更。）
