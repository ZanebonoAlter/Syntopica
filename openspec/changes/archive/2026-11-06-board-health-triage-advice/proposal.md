<!-- complexity: complex -->
<!-- ui-impact: major -->
<!-- constraint-domains: semantic-board -->

## Why

板块体检的 LLM 输出目前只有一句笼统评语（如「板块挂载规模健康，但部分国产厂商及通用概念标签存在相对低分现象」）——用户知道有问题，但不知道该摘哪个标签、该留哪个。2026-10-28 调研（docs/research/board-matching-evaluation/explore-findings.md）实证了「低分标签」背后至少有三种病因，不鉴别就无法开药方：

- **磁铁污染**（真该摘）：标签把跨域 tag 吸进版块（「全球科技巨头动态」max_sim 挂载方向失败率 46.9%）；
- **短名天花板**（不该杀）：短实体名 × 版块长名的 embedding cosine 天然低（OpenAI 0.57、MySQL 0.45，挂载实际全对），现行 LLM 判据已部分防误读，但只在评分层面，未落到标签级动作；
- **description 污染**（治数据）：L3 入库时 storage embedding = embed("label: description")，文章式 description 把短标签语义带偏（「上海」desc="上海知名商业综合体"→最近邻「举办地点」0.796；「中国媒体」desc="《环球时报》"→窄化成具体媒体机构簇）。

## What Changes

- 体检新增三类确定性信号计算（纯读，不重算匹配）：
  - **磁铁信号**：每个构成标签对「方向失败挂载子集」与「全部挂载」的贴合度均值差（失败子集贴合显著更低 = 污染源嫌疑）；
  - **最近邻信号**：构成标签的 pgvector top-3 最近邻 + 自身 label/description（短概念标签最近邻是实例标签 = 窄化/漂移嫌疑证据）；
  - **description 事件性预筛**：正则命中「此次/本次/该事件/报道/针对/期间/近日」等文章式词根（当前全库 1.8%，板块 composition 内 33 个）。
- 交互按 2026-10-28 FB-2 重做为**紧凑问题清单 + 单项核对与处理**，不再纵向堆四类建议；保留/不足项默认收起，统计作为按需依据。此信息架构变更升级为 major，原型未获用户明确批准前不实施。
- LLM 综合裁决升级为**标签级分诊归因**：输入从统计快照扩展为「统计快照 + 三病信号证据」，输出从「评分+一句评语」扩展为「评分+评语+构成标签四类分诊清单」：
  - 🔴 建议摘除（附证据：失败挂载数、流向对手版块）；
  - 🟡 建议重写 description（附可编辑的新文案，用户确认后进入独立修复执行链）；
  - 🟢 保留澄清（短名天花板/绝对健康，防误杀）；
  - ⚪ 证据不足（样本 < 最小样本量，下轮再判）。
- 分诊清单随快照持久化（metrics 结构扩展），查询端点返回清单，前端右轨体检卡片展示清单与动作。
- 动作边界维持体检纯读原则不变：**摘除**走既有手动摘除接口；**描述修复**由用户另行确认提交，生成新 storage embedding 后原子保存 description+embedding，再失效匹配缓存并更新全部活跃话题标签的版块匹配。体检采集/查询不会执行该写链。
- 描述修复提供持久任务、阶段进度、失败恢复与版本冲突保护。确认前声明辅助标签全局共享与匹配更新范围；仅在匹配全部更新后显示「更新完成，待下一轮体检验证」，不得把复制、提交成功或部分完成说成已修好。

### Out of Scope

- 提取提示词的百科式 description 源头治理与全库自动批量清洗（仍留后续 change）；
- 方向校验按 label 长度分层阈值（后续 change，依赖本 change 攒「确认保留的短名标签」数据）。

## Capabilities

### New Capabilities

- `auxiliary-label-description-repair`：人工核对并确认共享标签描述、storage 向量重建、原子保存、持久任务、匹配更新、失败重试与效果验证边界。

### Modified Capabilities

- `board-health-checkup`: 「LLM 综合评分与降级」Requirement 从单一评分+评语升级为标签级三病分诊与动作建议（新增磁铁/最近邻/description 三类信号输入与分诊清单输出）；「体检数据查询端点只读」Requirement 扩展返回分诊清单；快照持久化结构扩展。防三类系统性误读判据保留并下沉到标签级分诊依据。

## Impact

- **后端**：`backend-go/internal/tagmanagement/service/boardhealth/`（metrics.go 信号计算、llm.go 提示词与结构化输出、run.go 快照组装）、handler 侧体检查询端点响应结构扩展。
- **前端**：右轨版块体检卡片（`front/app/features/tags/components/console/` BoardHealth 相关组件）展示分诊清单与摘除/确认动作。
- **数据**：`board_health_snapshots.metrics` jsonb 扩展，历史快照不重写；修复任务需持久记录阶段、请求版本及匹配进度/失败，涉及新增任务存储与迁移，不再承诺零 DDL。现有 label/aliases/merge_embedding、文章与日报正文保持不变。
- **新增执行面**：auxlabel 手动修复服务与 handler/API、独立任务存储、board 匹配缓存失效及既有匹配引擎。不得复用只补 NULL 的 backfill-embeddings 去改已有向量，也不得依赖仅内存 job 假装可恢复。
- **LLM 成本**：体检仍每版块每轮一次裁决；每次用户确认的描述修复额外生成一次该标签 storage embedding，不重嵌全部文章，不自动发起额外体检。
- **文档**：`docs/reference/flow/semantic-board.md`、相关 API/数据库文档按最终执行契约同步。

## 部署后影响与操作

原型阶段不部署、不写真库。正式交付后，新一轮体检给出待处理问题；用户确认修复会修改共享辅助标签的描述及 storage 向量，并更新活跃话题标签的版块匹配，可能改变多个版块的挂载。确认前展示该范围，旧体检快照/既有日报不改写；效果待下一轮体检验证。新增任务表由迁移创建，无全库自动清洗、无部署即批量修复；用户按项核对后才执行。
