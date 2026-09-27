## REMOVED Requirements

### Requirement: 启动时模型健康检测

（移除理由：其中「快照健康后不再周期性复检」场景被本 change 反转为持续心跳复检，且正文对重探行为的引用同步变更；场景名无法在 MODIFIED 下退役，故整体重立。由下方 ADDED 的「启动时模型健康检测与心跳复检」接替，六个场景中五个语义不变随新块重建。）

## ADDED Requirements

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
