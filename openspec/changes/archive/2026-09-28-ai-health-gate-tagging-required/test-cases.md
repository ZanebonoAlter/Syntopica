# test-cases（simple 档精简：specs Scenario 即用例，此处只记契约继承、主链路落点与变体走查）

## 契约继承与调整（⓪ 本 change 改契约：ai-model-health 判定 Requirement）

反查命令：`bash scripts/harness/test-assets.sh ai-model-health`（2026-09-28 执行）

| 旧 Scenario（宽松口径） | 处置 | 旧测试 | 动作 |
| --- | --- | --- | --- |
| 两类各通一条即健康 | 保留标题、改写内容（「两类」重定义为核心两类） | `TestRunStartupProbe_LenientHealth_NonCriticalDown`、`TestRunStartupProbe_AllReachable_Healthy` | fixture 改为 embedding + topic_tagging 可达（可含非核心断路），断言 healthy=true；带 Lenient 旧名者按新口径改名（已落地为 `TestRunStartupProbe_CorePairUp_NonCriticalDown_Healthy`） |
| 缺 embedding 则不健康 | 保留（补前提） | `TestRunStartupProbe_MissingEmbedding_NotHealthy` | fixture 补「topic_tagging 可达」前提，断言不变 |
| 缺 llm 则不健康 | 保留标题、改写内容（收紧为打标不可达即不健康） | `TestRunStartupProbe_MissingLLM_NotHealthy` | 断言改为「打标主不可达即 false，且非核心（含云端）可达不救命」 |
| 未配置任何路由不健康 | 保留 | `TestRunStartupProbe_NoRoutes_NotHealthy` | 不动（回归） |
| （compose 侧 pins 旧语义） | 改写 | `TestHealthGate_EmbeddingUp_LLMDown_NotHealthy`（health_gate_compose_test.go 注释明说 pins lenient-health） | 按「打标决定生死」改名为 `TestHealthGate_EmbeddingUp_TaggingDown_NotHealthy` + 新增云端可达不救命回归 `TestHealthGate_CloudLLMReachable_NoRescue` |
| （compose 侧健康态前提） | 补前提 | `TestHealthGate_SlowProviderWithinTimeout_Healthy`、`TestHealthGate_ProbeComposesWithPauseGate`、`TestHealthGate_HeartbeatDegrade_PauseAwareSkips` | 健康 fixture 补 topic_tagging 可达 |
| （handler 侧 resume 自愈前提） | 补前提 | `TestSetAnalysisPause_Resume_TriggersHealthReprobe`（internal/admin/handler/analysis_pause_resume_probe_test.go） | 健康 fixture 由 embedding+summary 改为核心对（断 Healthy()=true 必须双核心可达） |
| （机制类 Requirement，26 场景中其余 22 个） | 不受影响 | reprobe_test / heartbeat_degrade_test / aihealth_launch_guard_test（心跳/自愈 fixture 补核心对前提） | 机制不改，整包回归 |

## 主链路表

| # | 步/动作 | 来源 Scenario | 期望 | 层 | 落点 |
| --- | --- | --- | --- | --- | --- |
| 1 | 探测后判定：embedding 通 + topic_tagging 通（非核心如 digest_polish 断路） | 两类各通一条即健康 | healthy=true | 函数单测 | backend-go/internal/platform/aihealth/aihealth_test.go（`TestRunStartupProbe_HealthVerdictMatrix` 行 1、`TestRunStartupProbe_AllReachable_Healthy`、`TestRunStartupProbe_CorePairUp_NonCriticalDown_Healthy`） |
| 2 | embedding 全不可达、打标通 | 缺 embedding 则不健康 | healthy=false | 函数单测 | backend-go/internal/platform/aihealth/aihealth_test.go（矩阵行 2、`TestRunStartupProbe_MissingEmbedding_NotHealthy`） |
| 3 | 打标主不可达、embedding 通 | 缺 llm 则不健康 | healthy=false（其他 LLM 可达不救命） | 函数单测 | backend-go/internal/platform/aihealth/aihealth_test.go（矩阵行 3、`TestRunStartupProbe_MissingLLM_NotHealthy`） |
| 4 | 打标主不可达 + digest_polish/data_enrichment_analysis 云端主可达 | 云端 LLM 可达不救命（2026-09-28 回归） | healthy=false，且 `analysispause.IsPaused()`=true（有效暂停） | 函数单测 + 组合 | backend-go/internal/platform/aihealth/aihealth_test.go（矩阵行 4）, backend-go/internal/platform/analysispause/health_gate_compose_test.go（`TestHealthGate_CloudLLMReachable_NoRescue`） |
| 5 | topic_tagging 路由缺失，或其链路 provider 全禁用 / provider 被禁用 | 打标路由未配置不健康 | healthy=false（fail-closed） | 函数单测 | backend-go/internal/platform/aihealth/aihealth_test.go（矩阵行 5-7） |
| 6 | 系统无任何启用路由（空 entries） | 未配置任何路由不健康 | healthy=false | 函数单测 | backend-go/internal/platform/aihealth/aihealth_test.go（矩阵行 8、`TestRunStartupProbe_NoRoutes_NotHealthy`） |
| 7 | 心跳去抖降级 / 不健康自愈 / 拉起冷却在新口径下走一遍 | 既有机制场景（不属本 delta） | 机制行为零变化 | 组合回归 | backend-go/internal/platform/aihealth/heartbeat_degrade_test.go, backend-go/internal/platform/aihealth/reprobe_test.go, backend-go/internal/admin/handler/analysis_pause_resume_probe_test.go |

## 变体走查（五组）

- **输入**（空串/纯空白/纯分隔符/单 token/大小写/特殊字符/超长）：**不适用留痕**——判定输入为结构化 `RouteHealth` 条目集合，无文本输入面；等价结构变体以「前置组」覆盖。
- **前置**：空集 = 场景 6 ✓｜单元素（仅 embedding 或仅打标）= 场景 2/3 ✓｜重复（同 capability 多路由）→ design D1「≥1 主通即该 capability 通」聚合语义，纳入 1.2 表驱动一条：同 capability 双路由一通一断 → healthy=true ✓｜越界引用（路由 enabled 但 provider 全禁用 / 路由不存在）= 场景 5 ✓｜部分满足 = 场景 2/3/4 ✓
- **时间窗口**：**不适用留痕**——判定为无时间参量的纯计算；心跳去抖（连续 2 次失败才降级）属既有机制不改，由第 7 行回归覆盖；启动竞态（CheckedAt=nil → false）属既有 fail-closed，`TestHealthy_NotReadyThenReady` 回归 ✓
- **幂等**：重复探测（60s 心跳反复写快照）同输入同输出 ✓（reprobe_test 回归）｜部分失败重试（ListRoutes 重试后判定）既有 ✓｜并发：`probeMu` 互斥既有，本 change 不新增并发面 ✓（不声称新线程安全）
- **可用性（UI 前三）**：**不适用留痕**——后端判定无界面；健康 API 响应字段集不变（仅 healthy 取值语义随口径），由设置页卡片既有组件测试回归，无新增交互 ✓

## 效果核对（判定结果依赖真实 provider 存活 → 真环境量化）

- **触发原因**：单测只证伪规则本身，证不了现实门态（healthy 还受真实端点、心跳时序影响）。
- **方法**：apply 部署后复刻 2026-09-28 原始场景——停本地 qwen（topic_tagging 主）、保持 qwen3-embedding 起、deepseek 云端可达，`curl --noproxy '*' localhost:5100/api/ai/health` → `healthy=false`；随后启动 qwen → 心跳一个周期内 `healthy=true` 且 tag_jobs 恢复消费。
- **量化结果/结论（2026-09-28 13:31 部署后实测，新口径半程已验证）**：
  - 环境：`10.11.12.111:8080` 已停（直连 `http=000`，经代理 `127.0.0.1:7890` 得 502——探测判不可达正确）；`qwen3-embedding`（`…:8081`）仍活；`deepseek-flash` 云端主力路由可达。
  - `GET /api/ai/health` → **`healthy=false`**，routes 明细：`embedding` reachable=true、`topic_tagging` reachable=false、`digest_polish` / `data_enrichment_analysis`（云端）reachable=true（首轮探活含 45s 拉起轮询，约 45s 后落地）。
  - `GET /api/schedulers/status` → `ai_healthy=false` **且** `analysis_paused=false`（用户意图开关不被健康度污染，符合 spec）；有效暂停 = `UserPaused() \|\| !aihealth.Healthy()` = **true** → 分析类 job 与 tag worker/embedding/merge 池停租约。
  - 结论：**规则在真环境生效**——“单开 embedding + 云端 deepseek 活着”不再开门，锁死原始事故路径；旧口径的 `healthy=true` 由矩阵/组合用例（矩阵行 4、`TestHealthGate_CloudLLMReachable_NoRescue`）锁定。
  - 恢复半程（2026-09-28 13:35，8080 拉回、直连 `http=200`）：**≤1 个心跳周期**（60s；探通即恢复、不走去抖）`healthy=true`（`checked_at=13:35:22`，全部 8 条路由 reachable=true）；`GET /api/schedulers/status` → `ai_healthy=true` / `analysis_paused=false` → 有效暂停解除；tag 队列恢复消费——`tag_jobs` leased=13 / pending=3、completed 915→922（实测期间补完 7 个），`embedding_queues` pending=10 重新被 lease。
  - 结论：**双向闭环实测通过（关→开、开→关）**，规则达标交付。
  - 旁证（非本 change 引入、机制未变）：`auto_start_models=true` 下 qwen 的 `start_command` 是 Windows 路径（`D:\tool\llama-server …`），后端跑在 Linux（树莓派）→ `sh -c` 无法执行，拉起链路对这台远端 AI 主机实际无效（日志仅见 `launched within 10m0s; skip re-launch, keep polling`，8080 重启后仍停）；模型需在 `10.11.12.111` 那台机器上拉起。用户决定关闭 `auto_start_models`（副作用：不可达 provider 不再走 45s 拉起轮询，心跳判定落地更快，判定口径不变）。

## 展示字段盘点（⑤ 改数据结构）

- 本 change 不改字段集：`healthy` / `routes[]` 字段名与结构原样，仅 healthy 计算口径变；来源语义有 spec 锚（MODIFIED「健康就绪判定（核心路由硬条件）」）✓；前端展示语义跟随、无新增用户可见字段，无需补 Requirement ✓
