<!-- complexity: medium -->
<!-- ui-impact: none -->
<!-- constraint-domains: semantic-board, data-enrichment -->

# Proposal: 版块诊断/研究两族 spec 合并（4 → 2）

## Why

`semantic-board` 域里 5 个「都带 board 字样」的 spec 共 1579 行，但读完内容会发现它们**不是同一件事**——是「版块」这个对象把它们凑到了一起：

| spec | 实际职责 | 消费者 |
|---|---|---|
| board-health-checkup（405） | 看版块**内部**构成质量：贴合度归因、事实标签画像、LLM 评分、快照趋势、分诊信号 | 体检任务 + 前端体检页 |
| board-direction-check（210） | 匹配结果的 **cosine 方向校验**：低于阈值标 `direction_mismatch` | 匹配管线 + 日报排除 + 匹配详情 API |
| board-signal-reports（387） | **外部新闻信号** → 人工触发 → 四十轮研究 → 可核查短报告 | 研究工作台 |
| board-level-analysis（382） | 版块**语义**简报 → 用户选问题 → 深度调查（多假设/正反证） | 版块分析工作台 |
| radar-pipeline-status（195） | 雷达页**状态块 UI**：队列积压 → 日报窗口 → 当日是否已出 | L0 `/tags` 页面 |

硬把 5 个合成 1 个会同时犯两个错：① 1579 行超过 D5 的 1500 红线；② 把「页面状态块」这种 UI 展示契约混进「分析能力」族，读的人要在一堆研究逻辑里找前端字段。

## What Changes

**2 族 + 1 保留**：

| 族 | 吸收 | 行数 | Requirement |
|---|---|---|---:|
| `board-diagnostics` | board-health-checkup + board-direction-check | 615 | 22 |
| `board-research` | board-signal-reports + board-level-analysis | 769 | 36 |
| `radar-pipeline-status` | **保留独立** | 195 | 5 |

切分依据是「改一个功能要读什么」，不是名字相似：

- 调**体检阈值 / 方向校验阈值 / 分诊闸门** → 只碰 `board-diagnostics`（都是「判定质量合不合格」）
- 改**研究报告产出 / 调查假设链** → 只碰 `board-research`（都是「人工触发的研究产出」，共享「候选 → 触发 → 异步 → 可核查」结构）
- 改**雷达页状态块展示 / 手动补生成入口** → 只碰 `radar-pipeline-status`（前端契约，与后端分析无改动连带）

合并规则沿用 `merge-tag-spec-family`（已归档）验证过的做法：

1. Requirement 原样搬运，不改语义
2. 每条带 `<!-- from: <旧capability> (<归档change>) -->`
3. 施工记录排末尾并标 `<!-- kind: impl -->`
4. 归档需 `.openspec.yaml` 的 `retire_capabilities: true`
5. 归档前摘掉旧 spec 的 `<!-- domain: -->`（否则 openspec 拒绝删除）

## Impact

- **spec 数量**：158 → 156（退役 **4** 个 + 新建 2 个；`radar-pipeline-status` 保留）
- **历史追溯**：指向 4 个被删 capability 的 archived delta 落 `domain-history.sh` 第 6 段；`tag-matching` 里「详见 board-direction-check」的引用会变死引用（设计 D2 记录，后续 change 修）
- **代码**：零改动
- **用户可见行为**：无
- **回退**：`git revert` 单/双 commit

## 不做的事

- 不合并 `radar-pipeline-status`（见上）
- 不合并泳道/画布族（9 个 spec / 3392 行，含 4 个巨型需先拆，单独 change）
- 不修正 spec 内容问题（重复/过时条款），发现记入 `review.md`
- 不把 `board-direction-check` 迁去 `tag-matching`（那族已归档，且方向校验在语义上属「诊断」：判断匹配质量合不合格，不参与匹配决策）
