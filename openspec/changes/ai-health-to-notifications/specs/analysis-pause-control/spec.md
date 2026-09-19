## MODIFIED Requirements

### Requirement: 前端健康未就绪提示

前端 SHALL 在「用户意图为运行（analysis_paused=false）但 AI 模型未就绪（NOT 健康）」时，通过通知中心展示健康未就绪提示：通知铃铛进入警示态（警示图标/配色，与未读数角标正交叠加），且通知面板列表顶部展示置顶系统状态条，告知用户 LLM/Embedding 未连通、分析暂停运行，并提供「重新检测」与跳转至模型配置页的入口。该提示 SHALL NOT 修改或禁用既有的暂停/启动按钮，SHALL NOT 在页面顶部悬浮 banner 形式展示。设置页 SHALL 提供「AI 健康状态」面板，展示各路由主 provider 的可达性明细、是否被后端拉起、上次检测时间，以及 `auto_start_models` 总开关。

#### Scenario: 意图运行但不健康时展示 banner

- **GIVEN** analysis_paused=false，ai_healthy=false
- **WHEN** 用户打开任意页面
- **THEN** 通知铃铛 SHALL 进入警示态，通知面板列表顶部 SHALL 显示「AI 模型未就绪（LLM/Embedding 未连通），分析暂停运行」置顶条（提示条随通知中心展示，不再悬浮页面顶部），含「重新检测」按钮与跳转设置页入口，且暂停/启动按钮 SHALL 保持可用不被禁用

#### Scenario: 健康恢复后警示消失

- **GIVEN** 通知中心处于 AI 未就绪警示态
- **WHEN** ai_healthy 变为 true（探活自愈或手动重探通过）
- **THEN** 铃铛 SHALL 回归普通态，面板置顶条 SHALL 消失（状态驱动，无需用户交互清除）

#### Scenario: 用户主动暂停时不展示健康 banner

- **GIVEN** analysis_paused=true（用户主动暂停）
- **WHEN** 模型亦 NOT 健康
- **THEN** 铃铛 SHALL NOT 进入健康警示态，面板 SHALL NOT 显示该健康未就绪置顶条（用户已主动暂停，无需再提示健康）

#### Scenario: 设置页展示健康面板与总开关

- **WHEN** 用户打开设置页
- **THEN** SHALL 见「AI 健康状态」面板（各路由主 provider 通断 + 是否后端拉起 + 上次检测时间）与 auto_start_models 开关

#### Scenario: 顶部栏常驻健康指示

- **WHEN** 顶部栏渲染
- **THEN** SHALL 常驻显示当前 AI 健康状态（健康/不健康二态，如 mdi:heart-pulse 绿/红），点击跳 AI 健康设置 section；与通知中心的健康未就绪警示（铃铛警示 + 面板置顶条）并存，二者 SHALL NOT 互斥

#### Scenario: 面板空列表时置顶条仍可见

- **GIVEN** analysis_paused=false，ai_healthy=false，通知列表为空
- **WHEN** 用户打开通知面板
- **THEN** 置顶系统状态条 SHALL 显示在空列表占位之上（系统状态与通知列表内容正交）
