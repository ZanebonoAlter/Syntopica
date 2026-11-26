# Design: 日报 spec 拆分与归族

## D1 为什么拆 3 族

`daily-report-system` 1417 行 / 41 req 是全仓最肥的单 spec，超过 D5 的 1200 预警线。41 条按功能分布：

| 组 | Requirement | 行数 | 改动时的现实场景 |
|---|---:|---:|---|
| A 生成管线核心 | 15 | 470 | 调聚类数上限 / 改 prompt 事实锚 / 改去重规则 |
| B 接口层 | 4 | 217 | 加端点参数 / 改 WebSocket 消息 |
| C 前端展示 | 7 | 285 | 改报纸布局 / 加空态 / 调全屏层 z-index |
| D 调度与补档 | 9 | 260 | 改补档窗口 / 改缺口口径 / 改告警文案 |
| E 重点选题 | 5 | 159 | 改候选排序 / 改 highlights 降级 |
| F 泳道分桶 | 1 | 58 | 改主题分桶规则 |

切分依据同既往四族：**改 A 时是否几乎必然要动 B**。

- A + E + F 都属「日报怎么生成出来」：聚类 → 分组 → 裁决 → 选题 → 分桶 → 存储，共享同一份 prompt 与同一批模型。**同族**
- B + C 都属「日报怎么对外暴露」：端点、WebSocket、报纸布局、浏览器展示。**同族**
- D 属「日报出不来时怎么办」：补档、守卫、缺口、告警。**同族**

三组之间几乎没有连带：改补档窗口不会影响聚类裁决；改报纸布局不会影响去重规则。

**R-3 修正**：最初把「日报重建窗口守卫」「日报终态通知的计数口径与 WS 终态消息一致」放在 ops，但它们对应的两条红线都是**跨端点口径**——红线 20「超窗日期必须拒绝重建，`generate API` 与 `TriggerNowWithDate` 同口径」、红线 35「终态消息与终态通知共用同一三态计数口径」。放在 ops 会让每改一次口径就要开两族。**已移入 `interface`**。

移后 ops 仍含一个端点（`POST /api/daily-reports/backfill-gaps`），但那是**运维动作的入口**而非查询/生成接口——切分线是「动作性质」（生成/查询 vs 运维），不是「有没有端点」。

## D2 radar-pipeline-status 归 interface（推翻上轮判断）

上一族（`merge-board-diagnostic-specs`）判定它「独立保留」，理由是「雷达页状态块 UI 契约，与后端分析零改动连带」。那个判断在**当时的对照组**（诊断/研究）里是对的。

这一轮它的兄弟变成「日报前端展示」，情况不同：`radar-pipeline-status` 描述的是「标签队列积压 → 日报生成窗口 → 当日是否已出」这条链的可视化，与 C 组的「日报生成进度前端」「TagsPage 内容 Tab」是同一条链的不同环节。

**判据不变、对照变了**：判断归族不能脱离「跟谁比」。同一份契约放在不同批次里，归属可能不同——这不是判据漂移，是「改动连带」的参照系变了。

## D3 不新建 `daily-report`，在 `daily-report-system` 上就地扩容（R-2 更正）

**最初设计错了**：我把族名叫 `daily-report`，却只把 match-fallback 的 2 条写进它的 ADDED——那 21 条核心条目还在 `daily-report-system` 里。结果「改生成算法只开 `daily-report`」是假的：族名不是 capability，族跨 2 个文件。

**正确做法**：族名就用 `daily-report-system`，它的 delta 内同时写

- `## ADDED Requirements`：match-fallback 的 2 条
- `## REMOVED Requirements`：搬走的 20 条（B4 + C7 + D9）

21 条 A+E+F 原位不动。openspec 2.1.1 实测预演（`buildUpdatedSpec`）：归档后 `daily-report-system` 重建为 21 + 2 = 23 条 / 711 行，**纯删除 + 追加、顺序原样、marker 保留、新增 0 行**。

这条也修正了 doc-authoring 的「就地扩容」用法：**族名必须等于既有 capability 名**，否则就是新建。

**代价**（同既往）：原 21 条不重排、不带 impl 标记、Purpose 需手改。

## D4 拆分机制：split-plan 而非 cap 级合并

既往用 `merge-spec-family.sh`，粒度是「整个 capability」。这次要**按 Requirement 子集**切分一个 spec，cap 级粒度不够。

新增 `split-plan.tsv`（存 change 目录，可复现）：

```
族名	源 capability	Requirement 标题（* = 全取）
daily-report	daily-report-match-fallback	*
daily-report-interface	daily-report-system	日报生成 API — 异步触发
...
```

生成器按 plan 抽取 → 写各族的 `## ADDED Requirements`，并把**被取走的条目**写成源 cap 的 `## REMOVED Requirements`（未取走的留在原位，不出现在 REMOVED 里）。

这样 `daily-report-system` 的 REMOVED 只有 20 条，不是全部 41 条——**部分搬运与整体退役用同一套机制表达**。

## D5 排序与分类

只对**搬移的 27 条**做 business/impl 分类（保留在原位的 21 条不写 delta、不加标记，符合就地扩容的固有约束）。

预判 impl 候选：
- `定时任务复用`——若正文只是「复用既有 cron 注册」，偏 impl
- `日报存储`——若只讲表结构与写入，偏 impl
- `日报数据模型`——混合（模型定义 vs 字段语义契约）

## D6 归档机制

1. `.openspec.yaml` 声明 `retire_capabilities: true`
2. 归档前摘掉 **2 个**被退役 spec 的 `<!-- domain: -->`（daily-report-match-fallback、radar-pipeline-status）
   - **`daily-report-system` 不摘**——它不退役（保留 21 条），继续持有 marker
3. 归档后 marker：
   - `daily-report-system` 保留 `daily-report, semantic-board`
   - `daily-report-interface` → `daily-report, ui`（前端契约占 C 组 7 条）
   - `daily-report-ops` → `daily-report`（纯后端调度，无 UI/api 面）
4. 归档后手改 `daily-report-system` 的 Purpose（就地扩容）

## D8 域级未收敛（R-12，观测项）

拆分本身是减法，但 `daily-report` 域拆完仍有 17 个 capability / 5234 行——**域内碎片没减少，只是把最肥的那个切开了**。这与 ISS-04「按功能族收敛」的长期目标仍有距离。

拆分不是终点：`daily-report-interface` 787 行里 12 条是前端契约，若继续增长会成为下一个分段对象（拆分点是「端点」vs「前端展示」）。

## D7 已知损失

- 指向 2 个被退役 capability 的 delta 落 `domain-history.sh` 第 6 段
- `daily-report-system` 被删的 20 条，其历史 delta 会指向一个「还存在但条目已搬走」的 capability——`domain-history` 按 capability 匹配，仍能命中，不会变孤儿，但读者按标题 grep 会找不到
- 跨族引用：`daily-report-match-fallback` 的「排除 direction_mismatch」与 `tag-matching`/`board-diagnostics` 的方向校验条款有交叉，登记到「跨族引用清扫」批次
