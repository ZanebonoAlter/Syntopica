<!-- complexity: complex -->
<!-- ui-impact: none -->
<!-- constraint-domains: scheduler, ai-summary -->

## Why

Syntopica 即将转向常驻部署拓扑：树莓派 5 常驻采集（auto_refresh 7×24），AI 推理跑在按需手动启动的局域网 PC 上。现有健康重探器的设计是**单向自愈**：快照 not healthy 时每 60s 重探、一旦 healthy 即停止（`ai-health-reprobe` spec 明文 "重探器 SHALL 仅在快照 not healthy 时活动，SHALL 永不主动把健康快照打回 not healthy"；实现见 `internal/platform/aihealth/aihealth.go:226-234`）。该设计只覆盖"开机自愈"场景；在常驻拓扑下，**PC 中途关机后快照停留假健康**：暂停门禁（`analysispause/gate.go:56 IsPaused() = UserPaused() || !aihealth.Healthy()`）不再拦截，分析批次持续对不可达端点发起调用并失败，烧毁 `completion_attempts`、文章被标 `failed` 且不进入自动重试候选，形成需人工 force 的永久缺口。

## What Changes

- **定时重探器改为无条件心跳**：健康快照无论 healthy 与否均按周期探测（复用 RunStartupProbe 全流程，含全局探测互斥、拉起冷却），健康状态可被心跳**降级**为 not healthy——修订现有 spec 中"健康即停、永不主动打回"的约定。
- **降级去抖**：避免单次瞬时网络抖动把 healthy 打成 not healthy 暂停全部分析；连续 N 次探测失败才降级（N 默认值在 design 阶段定），降级后立即进入现有的 60s 快速重探节奏，恢复路径（连续成功即升回 healthy）保持现状。
- **零新增通道**：不新增探测端点、不改探测口径（GET /models 零 token）、不改 `Healthy()` 宽松判定、不改手动重探 API 与前端横幅——降级态自动被现有 AiHealthBanner / `ai_healthy` 字段呈现。
- **明确不改动**：PauseAware 暂停覆盖面（firecrawl 等维持现状）、`analysispause` 公式、auto_refresh 与维护类任务不受健康门禁影响的现状、拉起冷却与进程不托管的约定。
- 已知残留风险（design 阶段评估是否纳入）：心跳把"假健康"窗口压缩到 ≤1 个心跳周期 + 1 个在途批次，但在途批次中途断电仍可能烧 `completion_attempts`（门禁仅在批次入口检查）；是否加批内快速失败/中止作为本 change 范围，待 design 决策。

## Capabilities

### New Capabilities

（无）

### Modified Capabilities

- `ai-health-reprobe`: 「健康快照定时重探（自动自愈）」requirement 整体重立——原契约"仅在 not healthy 时活动、healthy 后停止、永不主动打回"被双向心跳取代（REMOVED + ADDED「健康快照定时心跳（自愈与降级）」，healthy 态持续探测 + 连续 2 次失败去抖降级 + 忙容忍由 provider 超时天然提供）；手动重探 API、回环直连 requirement 不变。
- `ai-model-health`: 「启动时模型健康检测」requirement 因"快照健康后不再周期性复检"场景被反转而整体重立（REMOVED + ADDED「启动时模型健康检测与心跳复检」，六个场景中五个语义不变随新块重建）；「健康快照内存态与启动竞态」「健康就绪判定（宽松）」requirement 维持不变（降级复用同一快照与口径）。

## Impact

- **代码**：`backend-go/internal/platform/aihealth/aihealth.go`（`StartPeriodicReprobe` 循环逻辑、快照降级写入口径）；`analysispause`、`admin/scheduler`、`airouter` 预计零代码改动（行为随快照自动生效）。
- **接口/UI**：无新端点、无 UI 结构变化；现有 `GET /api/ai/health`、`/schedulers/status.ai_healthy`、AiHealthBanner 自动反映降级态。
- **行为**：常驻部署下 PC 关机 → ≤（心跳周期 × 去抖 N）时间内分析自动暂停，PC 开机 → ≤60s 自动恢复；单机用户（后端随用随开）行为不回退——启动探测与自愈路径不变。
- **测试**：aihealth 单元测试（心跳循环、去抖、互斥、冷却交互）+ `analysispause/health_gate_compose_test.go` 扩展降级场景。
- **部署面提示**：本 change 是"树莓派常驻 + 局域网 AI"拓扑的代码侧前置；部署侧配置（base_url 改 LAN 地址、start_command 清空、Cloudflare Tunnel 只读公开实例）不在本 change 范围。
