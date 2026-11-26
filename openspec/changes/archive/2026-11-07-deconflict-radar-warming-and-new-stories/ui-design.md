# UI 设计 — deconflict-radar-warming-and-new-stories

<!-- ui-impact: minor -->

## 复用契约

本 change **不新增**任何设计 token、色值、断点或组件；全部沿用既有骨架：

| 用途 | 复用对象 | 位置 |
| --- | --- | --- |
| 分档容器 | `.rd-warm-block` | `BoardRadarReadingRail.vue` |
| 分档小标题 | `.rd-warm-sub` | 同上 |
| 空态/理由句 | `.rd-warm-why` | 同上 |
| 说明脚注 | `.rd-warm-note` | 同上 |
| 条目 meta 行 | `.rd-read-meta` | 同上 |
| 条目容器 | `.rd-warm` / `.rd-warm-new` | 同上 |
| 文本色 | `--color-text-muted` | 既有主题变量 |

## 结构变化（唯一实质改动）

**改前**：「今天可能升温」卡片内部嵌套「今日新起」子块 —— 视觉上是一张卡里两个标题，读者分不清两档。

**改后**：两档成为**同级独立分档**，各自有 `.rd-warm-block` 容器：

```
今日该读 (READING LIST)
├─ 滞后提示（既有）
└─ 5 条事件线

今天可能升温            ← 独立卡片
└─ 空态 / 升温条目

今日新起                ← 独立卡片（原为嵌套子块）
└─ 条目 / 一行摘要
```

## 文案改动

| 位置 | 改前 | 改后 |
| --- | --- | --- |
| 升温空态（无跨天线） | `暂未触发升温条件。` | `当前没有跨天延续的事件线。` + `跨天延续能力尚在建设中。` |
| 升温空态（有跨天线未达 2 倍） | `暂未触发升温条件。` | `暂未触发升温条件。`（保持不变） |
| 升温空态（旧响应无法判定） | `暂未触发升温条件。` | `暂未触发升温条件。`（保守回退） |
| 新起摘要（全被该读覆盖） | 3 条重复条目 | `另有 N 条今日新起事件线（未在今日该读中列出）。` |
| 新起脚注 | `事件线当日热度达到自身近 3 日均值的 2 倍，且当日有报道证据；近 3 日无基线的归「今日新起」。` | 拆两句分放两档：升温档只讲 2 倍条件；新起档讲「近 3 日无基线，且未在今日该读中列出」。 |

## 目标检查

- 1440×900、1920×1080、390×844 三档：两档标题各自可读、无嵌套歧义；摘要行在窄屏正常换行不溢出。
- 空态与摘要素材均来自 `stories` 段已有或新增字段，**不在前端硬编码判断**。
- 不引入新的动画或过渡；reduced-motion 行为不变。

## 验收

- 组件测试：`front/app/features/tags/components/console/BoardRadarView.test.ts`（空态三态、摘要行、新起档结构）；`front/app/features/tags/composables/useBoardRadarOverview.test.ts`（warmingEmptyKind 三态）；`front/app/api/normalizers/boardRadar.test.ts`（cross_day_story_count 归一）。
- agent-browser 验收：`openspec/changes/deconflict-radar-warming-and-new-stories/ui-verification/report.md`（A-1～A-7 机械断言 + 三视口截图 radar-1440.png / radar-390.png，静态 :5100）。

## 原型

无。`minor` 档复用既有契约，不需要 `ui-prototype/`，不需要用户原型审批。
