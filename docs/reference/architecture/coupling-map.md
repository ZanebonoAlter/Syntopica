# 跨功能耦合地图（Coupling Map）

> **活文档 · 增量维护**。本文件登记已知的"跨功能传导耦合"——即改 A 功能会通过隐式链路影响 B 功能行为的那些点。它不是全量架构审计，而是**踩到/分析出的耦合点逐条登记**，避免再次因"改了 X 没想到把 Y 打散"而踩坑。
>
> 与 [map.md](map.md) 的分工：map.md 回答"业务域怎么导航"（域→流程→代码入口）；本文件回答"**改这个会影响那个**"（跨域传导契约）。

## 为什么需要这份地图

单测是孤立的，看不到跨功能传导。最典型的教训：**quality 排序改动 → 通过聚类输入把 topic 持久化血缘打散**（见 §1）。`codegraph impact <符号>` 只查直接调用面，跨域/跨层传导链需要人工顺延深查。本地图把已知的传导链沉淀下来，配合 [开发执行规范 §7](../开发执行规范.md) 的"架构体检顺传导链深查"一起使用。

## 维护规则

- **发现即登记**：每次 §7 架构体检发现自己改动能跨功能传导、或线上踩到"改 X 坏 Y"的坑，就往本文件补一条（§N）。
- **每条必含**：耦合名、源功能→目标功能、传导链、守卫测试、触发条件/注意事项。
- **登记 ≠ 写死**：若后续重构切断了某条耦合，在该条标注"已解除（commit/PR）"但保留记录。
- **归档归属**：本文件属于 `architecture/`（架构级耦合关系），里程碑收尾时随 architecture 一起 review。

---

## §1 quality 排序截断 → 持久话题锚定血缘

| 项 | 内容 |
|----|------|
| **源功能** | 日报质量排序截断（`filterTagsByQuality`，topicgraph/service） |
| **目标功能** | 持久话题锚定 / 生命周期（`assignAndUpdateTopics` → `planTopicAssignments`，topicgraph/repository） |
| **耦合性质** | 跨层（service→repository）、跨子域（quality 排序→topic 持久化），单测不可见 |
| **守卫测试** | 纯逻辑：`TestPlanTopicAssignments_AnchorHit_MatchedWithinThresholdNotNearest`；DB 集成（断链）：`TestTopicLineageSurvivesClusterDrift` |

**传导链**：
```
改 filterTagsByQuality 的截断排序（如 MatchTier 调用、截断阈值）
  → 进 LLM 聚类(ClusterTags)的 tag 集合变化
  → LLM 返回的 matched_topic_id 漂移到 embedding 第 2 近的 topic
  → planTopicAssignments 的双重确认判定
  → [修复前] matched_id != 最近邻 → 判定失败 → section 误开 candidate
       → 持久话题血缘断裂 → 前端"全是突发话题"
  → [修复后] matched_id 在 embedding 阈值内任一 → 仍锚定 → 血缘保持
```

**触发条件**：任何改"进 LLM 聚类的输入"（截断、去重、tag 收集 SQL、cluster 阈值）的 change，都要意识到它会传导到 topic 锚定。

**注意事项**：
- 双重确认已放宽为"embedding 阈值内 AND LLM 同指"（见 `quality-scoring-observability` design D6）。若有人想收紧回"必须最近邻"，**必先看本条 + 守卫测试**——收紧会重新引入脆弱性。
- 评估 scope 时，截断/聚类输入改动的 Risks 评估必须把"→ topic 持久化"这条传导链纳入，不能只写"截断边界变化"。


## §2 dataenrichment→admin 调度器反向依赖（已解除：decouple-backend-domains）

| 项 | 内容 |
|----|------|
| **源功能** | dataenrichment 生命线三 job（lifeline_weekly/monthly/yearly JobFunc） |
| **目标功能** | 调度器框架（原 `internal/admin/scheduler`） |
| **耦合性质** | 基础设施反向依赖业务域：业务域 import admin 的框架类型（JobFunc/JobResult），改框架牵连所有 job 域 |
| **守卫测试** | depguard boundary 规则 + tasks 8.5 grep（`dataenrichment/platform` 对 `admin/scheduler` 生产 import = 空） |

**传导链**：~~dataenrichment/scheduler_jobs.go → admin/scheduler（框架）~~ → **已解除（decouple-backend-domains，2026-09-26）**：框架迁 `internal/platform/scheduler`，dataenrichment/platform 直接 import 平台层。

**触发条件**：不再适用（结构性解除）。

## §3 域间深路径耦合现状（decouple-backend-domains 收敛后基线）

| 项 | 内容 |
|----|------|
| **源功能** | 全部业务域（admin/reader/tagmanagement/topicgraph/dataenrichment/datasources/discovery） |
| **目标功能** | 彼此的 handler/service/repository/models 子包 |
| **耦合性质** | 跨域深路径 import（编译耦合 + 心智负担）；已由 depguard 编译期拦截 |
| **守卫测试** | `.golangci.yml` 7 条 boundary 规则（`golangci-lint run ./...` 退出码 0 = 合规） |

**传导链**：收敛前基线（2026-09-26 复测）：生产深路径域间边 9 条/14 文件（admin→tagmanagement 3、admin→reader 2、admin→topicgraph 1、admin→datasources 1、reader→tagmanagement 2、dataenrichment→{admin,reader,datasources} 各 1、datasources→dataenrichment 2）。收敛后：**业务域间生产深路径 = 0**（跨域一律走 root 门面：tagmanagement/reader/topicgraph/discovery 各 wire.go re-export）。

**剩余耦合边（豁免层，depguard 不管）**：app→任意域（装配层）；platform/testutil→tagmodels（golden schema hook）；datasources↔dataenrichment 互嵌（唯一带注释豁免的边界违规，改门面会成 root 环，待后续 change 拆解）。

**触发条件**：新增跨域调用时——只 import 对方 root 门面包，需要新符号就在对方 wire.go 加 re-export；模型引用同样经 root（对方 models/ 是深路径）。

**注意事项**：
- 白名单共享模型（internal/models 18 项+FeedStats 例外）唯一权威登记处 = `.golangci.yml` 白名单注释；单域独占新模型必须定义在所属域 models/ 子包并 init() 调 `database.RegisterModels` 自注册（静态清单 import 域包会成环，见 platform/scheduler/task.go 注释）。

---

<!-- 后续耦合点按 §4、§5... 增量登记。模板：
## §N <耦合名>

| 项 | 内容 |
|----|------|
| **源功能** | ... |
| **目标功能** | ... |
| **耦合性质** | ... |
| **守卫测试** | ... |

**传导链**：...
**触发条件**：...
**注意事项**：...
-->
