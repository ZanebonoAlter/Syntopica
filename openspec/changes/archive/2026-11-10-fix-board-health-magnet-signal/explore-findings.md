# Explore Findings：板块体检把核心主体（伊朗）判为污染的根因

> 探查方式：只读子线程（explore 档）代码链路梳理 + 主线程真库只读 SELECT 核对。快照日期：2026-11-09。
> 本文件是结论存档，实现细节见 `design.md`，用例故事见 `test-cases.md`。

## 1. 现象锚点

| 项 | 值 |
| --- | --- |
| 板块 | `semantic_labels.id=1974`「中东地缘政治与美伊关系」 |
| 快照 | `board_health_snapshots.id=410`，checked_at=2026-11-09 21:23，llm_score=58 |
| 分诊分布 | rewrite 47 / keep 0 / remove 0 / insufficient 0 |
| 磁铁嫌疑成立 | 41 / 47（平均 gap 0.1062，阈值 0.08） |
| 伊朗（aux 262） | all_mean 0.6661 / failed_mean 0.5424 / all_samples 303 / failed_samples 19 / suspicious=true；cosine_mean 0.6661（与 all_mean 恒等） |
| 板块 description | 空（LLM 无「伊朗是核心主体」的文本依据） |

## 2. 根因（三层叠加）

### 2.1 口径层：磁铁信号方向与语义相反

`backend-go/internal/tagmanagement/service/boardhealth/signals.go:65 computeMagnetSignals`：
`suspicious = all_mean − failed_mean ≥ MagnetGap`。

- 「磁铁」语义 = 某标签**相对更吸引跑题挂载**（failed 侧应更高）；代码却判「failed 比全体低就算嫌疑」。
- 失败子集按定义与本板方向不符，与核心主体天然不相似 → **越是核心主体 gap 越大**（伊朗 0.124 全板靠前）。
- `all_mean` 侧含失败行 → 既自我稀释，又与 `cosine_mean` 恒等（一个数字两个含义）。
- 失败子集是**板级共享行集**（代码注释自认「不做单标签吸引归因」），却当标签级证据用。

### 2.2 提示词层：把板级证据当标签级证据

`llm.go:87-91`：
- remove 的必要证据 = `magnet.suspicious=true`（该信号已失效）；
- rewrite 判据含「近邻是实例/机构/**地点**类标签」——对国家/地名这类实体标签，同类近邻恰是健康形态；
- 只禁短实体标签判 remove，**不禁 rewrite**；
- 板级 `magnet_flow`（"失败挂载成因：美国新闻"）被渲染进**每个**标签的证据块。

实证翻转：同一板块 11-06 10:14 快照（13 keep + 34 rewrite，锚点「德黑兰/伊朗 = keep」）→ 代码与提示词一字未改 → 11-06 21:03 与 11-09 变 47/47 rewrite。

### 2.3 展示/闸门层：无保护、无兜底

- 无「核心主体」概念（全库检索 `核心主体/主体词/core_subject` 零命中）；
- 无「待核对占比过高」闸门（47/47 全 actionable 原样落库并全量呈现）；
- 板块 description 为空 → LLM 只有标题可依据。

## 3. 结论

不是伊朗有问题，也不是数据坏了——是**指标方向反了 + 提示词判据缺保护 + 无兜底闸门**三件事叠加。
项目早在 `openspec/changes/archive/2026-11-06-board-health-triage-advice/tasks.md:42` 记下过锚点「德黑兰/伊朗 = keep」，但只以散文形式存在，没变成代码约束或回归测试。

## 4. 关键代码落点

| 落点 | 内容 |
| --- | --- |
| `backend-go/internal/tagmanagement/service/boardhealth/signals.go:65` | `computeMagnetSignals`（口径反转主战场） |
| `backend-go/internal/tagmanagement/service/boardhealth/llm.go:87-91` | 分诊判据段（提示词硬约束） |
| `backend-go/internal/tagmanagement/service/boardhealth/llm.go:349` | `normalizeTriage`（闸门落点：归一之后、持久化之前） |
| `backend-go/internal/tagmanagement/models/board_health.go` | `BoardHealthMagnetSignal` / `BoardHealthLabelMetrics` / `BoardHealthMetrics`（字段扩展） |
| `backend-go/internal/tagmanagement/service/boardhealth/run.go:309` | `collectBoardStatistics`（核心主体标记需 board 标题/描述/别名） |
| `front/app/features/tags/components/console/board-health/boardHealthWorkspace.ts:90` | `splitTriageItems`（remove/rewrite → 待核对；insufficient → 其余标签） |
| `front/app/api/normalizers/boardHealthTriage.ts:235` | 信号归一化（前端不重算口径） |
