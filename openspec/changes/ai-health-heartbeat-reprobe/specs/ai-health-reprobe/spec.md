## REMOVED Requirements

### Requirement: 健康快照定时重探（自动自愈）

（移除理由：核心保证反转——原契约"仅在 not healthy 时活动、healthy 后停止、永不主动打回"被双向心跳取代：healthy 态持续探测、连续失败可降级。由下方 ADDED 的「健康快照定时心跳（自愈与降级）」整体接替，场景集随之重建。）

## ADDED Requirements

### Requirement: 健康快照定时心跳（自愈与降级）

系统 SHALL 在后端启动时启动一个后台定时心跳器：**无论健康快照判定 healthy 与否**，均按固定间隔（默认 60 秒）周期性探测（复用 RunStartupProbe 全流程，含自动拉起、45 秒轮询窗口、10 分钟拉起冷却、全局探测互斥）。健康状态 SHALL 双向流转：

- **降级**：连续 2 次探测失败 SHALL 将快照从 healthy 降级为 not healthy（探测 HTTP 超时沿用 provider 自身 `timeout_seconds`、未配置兜底 15 秒——慢而活着的服务器在超时内应答即不计失败，"忙容忍"由 provider 超时天然提供，不设额外时间窗）；
- **升级**：单次探测成功即恢复 healthy。

心跳器 SHALL NOT 改变健康判定口径、SHALL NOT 在探测 in-flight 时叠加探测（仍由全局互斥保证）。心跳发现的不可达 provider 在满足拉起条件（`auto_start_models=true` 且配置了 `start_command`）时 SHALL 走既有拉起链路（受 10 分钟冷却约束）。

#### Scenario: 健康态心跳持续探测

- **WHEN** 健康快照判定 healthy
- **THEN** 心跳器 SHALL 继续按固定间隔探测，SHALL NOT 停止（不再"健康即停"）

#### Scenario: 端点秒拒连续两次即降级

- **WHEN** 快照 healthy，主 provider 所在端点失联且表现为连接拒绝（如 AI 宿主关机，失败延迟接近 0）
- **THEN** 连续 2 次心跳探测失败后快照 SHALL 降级为 not healthy，分析类任务 SHALL 随门禁停止租约（最坏延迟 ≈ 2×心跳间隔）

#### Scenario: 忙服务器不被误降级

- **WHEN** 主 provider 正在处理长耗时推理调用（实测成功调用最长可超 200 秒），心跳探测期间服务器响应 /models 变慢
- **THEN** 探测 SHALL 等待至 provider `timeout_seconds` 才判定失败；慢而活着的服务器在超时内应答 SHALL 计为成功，SHALL NOT 触发降级

#### Scenario: 降级后自动恢复

- **WHEN** 快照因连续失败降级为 not healthy，随后端点恢复（如 AI 宿主开机）
- **THEN** 心跳器 SHALL 在后续探测中探通并在单次成功后恢复快照 healthy，全程无需用户干预

#### Scenario: 慢加载模型加载完成后自动自愈

- **WHEN** 后端启动探测时本地 llama-server 仍在加载（45 秒轮询窗口内始终返回 502），快照判定 not healthy，模型在启动后数分钟加载完成
- **THEN** 心跳器 SHALL 在后续间隔探测中探测成功，快照 SHALL 自动更新为 healthy

#### Scenario: 探测 in-flight 时定时触发被跳过

- **WHEN** 心跳间隔到达，但一次探测（启动探测或手动重探）仍在进行中
- **THEN** 定时触发的探测 SHALL 被跳过（不并发、不重复拉起），由进行中的探测完成后更新快照

#### Scenario: 心跳失败触发拉起且遵守冷却

- **WHEN** 心跳探测发现某 provider 不可达，`auto_start_models=true` 且该 provider 配置了 `start_command`
- **THEN** 心跳 SHALL 走既有拉起链路执行 start_command；该 provider 在 10 分钟冷却窗口内已被成功拉起时 SHALL NOT 再次执行，仅继续轮询已拉起进程
