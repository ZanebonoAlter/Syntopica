# Design: 标签族 spec 合并

## D1 分层模型（代码依赖验证过，不拍脑袋）

按 `backend-go/internal/` 的真实 import 关系定层，不是按名字猜：

```
models ← platform / datasources        （谁都不依赖它们以外的业务逻辑）
tagmanagement ← models                  （被四层 import：reader / topicgraph / discovery / dataenrichment）
reader ← models, tagmanagement
topicgraph ← models, tagmanagement      （→ 话题建立在标签之上）
discovery ← models, reader
dataenrichment ← models, reader, tagmanagement, topicgraph
admin / app ← 以上全部                  （编排层）
```

业务概念分层：

| 层 | 内容 | 判据 |
|---|---|---|
| **L0 原子概念** | 文章/feed/抓取、标签体系 | 不依赖其他业务概念；被三层以上 import |
| **L1 语义结构** | 话题/事件/图谱、版块 | 由 L0 组装而成；`topicgraph` import `tagmanagement` 是硬证据 |
| **L2 加工能力** | 标签→版块匹配、数据增强/embedding/雷达、订阅发现、阅读 | 输入是 L1 的结构，输出是判断/分数/列表 |
| **L3 产品形态** | 版块工作台/两支画布、日报 | 输出给人看的东西 |
| **L4 工程支撑** | harness / dev / db / api / ui 规范 | 横切，不属于业务链路 |

**关键推论**：标签在 L0，往上每一层都可能用到它。所以标签族的 spec 必须自己能说清规矩，不能指望上层文档替它解释——这也是它碎片化代价最大的原因（21 个文件都在底下，每层都要翻）。

## D2 三族划分

23 个 capability 按「改一个功能要读什么」切三刀：

| 族 | 边界问题 | 吸收成员 | 行数 |
|---|---|---|---:|
| `tagging` | 标签**怎么产生**、数据模型是什么 | tagging-domain（22 条）、aggregate-tagging、semantic-label-model、composite-label、article-tag-watch-toggle、tag-queue-scheduling、tag-queue-progress-chip | 1042 |
| `auxiliary-label` | 辅助标签**怎么维护**：合并、清理、质量 | auxiliary-label、aux-label-gc、auxiliary-label-description-repair、tag-hierarchy-quality、tag-merge-suggestions、tag-merge-ui、tag-match-identity-protection | 678 |
| `tag-matching` | 标签**怎么匹配到版块**、分数怎么展示 | tag-to-board-matching、tag-embedding-management、board-affinity-metadata、board-match-auto-trigger、board-match-cache、match-score-visualization、match-detail-ondemand、source-board-hit-rate、cross-board-relation-discovery | 1051 |

切分依据是**改动连带关系**，不是名字相似：

- 改「提取 prompt」→ 只碰 `tagging`
- 改「合并建议算法」→ 只碰 `auxiliary-label`
- 改「匹配阈值」→ 只碰 `tag-matching`
- 跨族改动（如「新增一种标签类别，要能匹配」）→ 读两族，这本来就是两件事

**boundary 反例**（说明为什么没这么切）：

- 不按「提取 / 辅助 / 匹配」三分之后又拆 `tag-embedding`：embedding 是匹配的手段（`tag-to-board-matching` 用它算相似度），拆出去会让「改阈值」变成读两个文件
- 不把 `article-tag-watch-toggle` 放匹配族：它是「用户订阅某个标签」，属标签本身的状态，不是匹配

## D3 来源注释格式

每条 Requirement 标题下加一行 HTML 注释：

```markdown
### Requirement: 泛词标签黑名单拦截

<!-- from: tagging-domain (2026-05-13 backend-package-restructure) -->

系统 SHALL ...
```

- 格式：`<!-- from: <旧capability> (<归档日期> <归档change名>) -->`
- **来源只认 `## ADDED Requirements` 的 change**（= 真正创建该 capability 的那次）。MODIFIED 的 change 只说明 capability 早已存在，把它当来源会让读者误以为那几条 Requirement 是它引入的（R-4 纠错：`board-match-cache` 6 条、`match-detail-ondemand` 3 条曾因此指错）
- **8 个 capability 的创建 change 不可考**，只写 `<!-- from: <capability> -->`，不编造来源： aux-label-gc、tag-merge-suggestions、tag-merge-ui、tag-match-identity-protection、board-affinity-metadata、board-match-auto-trigger（archive 零引用）+ board-match-cache、match-detail-ondemand（首个 archive change 是 MODIFIED）
- 分类结果随 change 一起入库（`spec-classification.txt`），保证脚本可幂等重跑（R-9）
- 注释是给人查的，不参与任何解析（`domain-history.sh` / `check-standards.sh` 都不读它）

## D4 Requirement 排序规则：business 段 + impl 段

不按来源文件堆，而分两段：

1. **business（行为契约）在前**——保持来源顺序，不额外重排。之所以不再细分「模型/提取/聚合」，是因为自动化的二级分类容易把边界划错，而本族的目地只是「能一次读完」，不是「一次就找到」。
2. **impl（施工记录）在后**——同样保持来源顺序，每条加 `<!-- kind: impl -->` 标记。

分类判据（由 glm-5.4 批量判定 + 人工校验 3 个边界例）：

| 类型 | 判据 | 例 |
|---|---|---|
| business | 约束「系统对外产出什么」，改坏了功能就错 | 泛词标签黑名单拦截、单主题 4000 runes / 6 上限 |
| impl | 约束「代码怎么组织/怎么写」或记录一次已完成的施工 | 包结构、域目录命名、测试迁移、存量一次性清理 |

实测分布：tagging 41 business / 14 impl，auxiliary-label 37 / 1，tag-matching 38 / 17。

**边界裁定**（review 会重点看这三处）：

- `Thread Safety`（`sync.RWMutex`）、`L1 索引不原地写 map` → **impl**：数据竞争由 `-race` 把关，不必占契约条目
- `存量病态时间锚标签可修复性`、`存量示例词泄漏一次性清理` → **impl**：跑完一次契约就失效
- 缓存族拆两类：**失效契约（composition/label 任期变更必须重算）→ business；纯 TTL（AI config 5min）→ impl**——前者漏了就用旧数据出结果，后者只是 slale 容忍度

## D8 族名冲突：就地扩容而非新建

`auxiliary-label` 这个族名与现有 capability 重名。两种做法：

| 做法 | 结果 | 评价 |
|---|---|---|
| 改名新建（如 `auxiliary-tags`） | 旧全量 REMOVED + 新全量 ADDED | 多一个只为改名的 capability，与「减少碎片」的初衷相闷 |
| **就地扩容（采用）** | `auxiliary-label` 保留，只把其他 6 个成员的 Requirement 以 ADDED 追加 | 名字本来就该是这个，不改不必要的边界 |

代价：`auxiliary-label` 自己原有的 15 条留在原位，新增 23 条追加到末尾，排序不如另两族整齐。**接受**，因为它的 impl 只有 1 条，错位成本低。

## D5 巨型阈值与后续拆分

三个族合并后 678–1051 行，仍然偏大。本次接受，理由：现状是 23 个文件共 2771 行，先收敛到 3 个可读的整体，再做第二轮拆分风险更低。

**后续拆分阈值（观察用，不阻断）**：

- 单族 > 1500 行 → 必须拆
- 单族 > 1200 行且包含 ≥3 个不相干功能 → 考虑拆
- 单族内某个 Requirement 家庭（同前缀 3 条以上）自成体系 → 可提为独立族

这条阈值不进门禁，写在这里供后续 change 参考——呼应「数值可参考不绝对」。

## D6 历史追溯与孤儿 delta

删掉 23 个旧 capability 后：

- **实测口径**（R-7 更正）：指向 22 个被删 capability 的 delta 文件 47 个、涉及 36 个 archived change（archive 共 284 个）
  - 仍能经其它 capability 命中 `semantic-board` → 15 个 change 进第 6 段
  - **只碰过被删 capability → 21 个 change 彻底不可见**（时间线和孤儿段都不出现），如 `2026-05-13-backend-package-restructure`、`2026-08-22-analysis-remediation`、`2026-09-24-night-window-alignment`、`2026-10-28-guard-tag-exemplar-leakage`、`2026-11-25-retire-person-extraction`
  - 这是本次合并的**已知损失**：`domain-history.sh` 的 `orphans` 只在 `matched=1` 分支自增，匹配不到任何域的 change 无从显示。根治需给脚本加「已退役 capability → 族」别名表（登记为后续 change）
- 第 2 段时间线里，命中 `semantic-board` 且含孤儿 delta 的 change 会带 `[含孤儿 delta]` 标记
- 来源注释是读 spec 时的追溯手段；「哪个 change 改过这条」仍需 `grep openspec/changes/archive/*/specs/<旧cap>` 反查

**不做**：不改 archive 里的任何 delta（归档即永久家），不重建历史链。

## D7 为什么走 openspec 而不是直接改文件

本次改动是结构重构，不产生新行为，但它**改变了 capability 边界**——这是契约层面的变化，必须留变更记录：

- 3 个新 capability 的 `## ADDED Requirements` 记录「这些规矩现在归谁」
- 23 个旧 capability 的 `## REMOVED Requirements` 记录「这些规矩原来归谁」，归档后仍能搜到

直接 `rm -r` + 新写文件会让「某条规矩什么时候消失的」彻底断链。

**退役授权（R-1）**：22 个 capability 的 Requirement 被 REMOVED 清空后，openspec 2.1.1 默认「写入重建稿 → 校验」，空 spec 校验失败会整次中止归档。必须在 change 目录写 `.openspec.yaml` 声明 `retire_capabilities: true`，归档才会直接删除这 22 个 spec 文件并修剪空目录——**不会留下空壳**，因此不存在「归档后手删空壳」这一步。

## D9 族级 domain marker 的取舍

合并后一族只有一个 marker，而 J 段上限 3 个域，装不下成员并集（tagging 7 域、tag-matching 6 域、auxiliary-label 新增成员 4 域）。取舍判据是**「改这族的人必须读哪些 flow 红线」**，不是「成员碰过哪些域」：

| 族 | 成员并集（域数） | 最终 marker | 理由 |
|---|---|---|---|
| `tagging` | semantic-board, topic-graph, ui, scheduler, db, ai-summary, reading（7） | `semantic-board, ui` | 主域不可省；`ui` 保留因队列芯片是用户可见组件，改它要读前端布局契约。scheduler/db/ai-summary/reading 的是实现手段，改这族时读业务域红线即可 |
| `auxiliary-label` | semantic-board + ui, scheduler（新增 6 成员带来） | `semantic-board`（不变） | 合并建议/层级质量是版块域内部治理，聚合执行时机不改变契约。新增的 scheduler 只影响「何时跑」，读业务域红线够 |
| `tag-matching` | semantic-board, topic-graph, api, db, ai-summary, ui（6） | `semantic-board, topic-graph` | 匹配结果同时喂版块和话题两条线，两个业务域都不可省。api/db/ui 是实现面 |

**已知代价**：原本挂 `scheduler` / `ai-summary` / `reading` 的 4 个 capability 合并后不再出现在这些域的反查结果里。改那些域的人不会被告知「标签族有相关条款」。这是「一族一 marker」的固有限制，替代方案（在 Purpose 里列成员域）不参与机器查询，故不采用。
