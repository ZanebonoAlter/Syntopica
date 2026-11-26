# 设计 · fix-lane-row-gap-zoom

## Context

`unify-lane-workbench` 归档后，用户在真机上反馈：滚轮放大画布后，节点之间的间隔"卡死"。

排查结论（实测，非推测）：

1. **滚轮缩放的机制是对的**——临时探针测试 mount `LaneCorrectionCanvas` 后点 `lane-zoom-in`，读到：

   ```
   BEFORE: width="900"  height="360"  viewBox="0 0 900 360"
   AFTER : width="990"  height="396"  viewBox="0 0 900 360"   （110%）
   ```

   `viewBox` 不变、SVG 元素尺寸 ×1.1 → 浏览器等比拉伸，**坐标间距与半径同比放大**。全局 CSS 也没有 `svg { width: ... }` 去压制这个属性。

2. **真正被写死的是"行距 / 直径"这个比值**。成员区行距是常量 `slot * 16`，而成员半径由列宽推导：

   ```
   cap        = min(16, columnWidth × 0.46)
   memberBase = max(min(cap, 10), cap × 0.85)
   ```

   **本画布的成员半径被 `memberRadius: MEMBER_BASE_RADIUS`（5.5）覆盖，恒定不随列宽变**。所以：

   | 项 | 值 |
   | --- | --- |
   | 成员圆直径 | **11** |
   | 标题起笔间距 | 5 |
   | 标题最多两行 | **24** |
   | **单节点纵向总占高（圆顶 → 标签底）** | **40** |
   | 行距 | **16** |
   | 判定 | **圆本身不重叠（16 > 11），标签压下一节点 24px** |

   等比放大 3 倍后：行距 48、单节点占高 120，比值仍是 0.4 → 仍然压。这就是"放大不缓解"的来源。

3. **新建泳道画布无此问题**：`LaneComposeCanvas` 的 `rowGap = max(36, min(110, 360 / (maxSlots − 1)))`，下限 36 > 直径 27.2，比值 > 1。故不纳入本次改动。

## Decisions

### D1 行距由**节点纵向总占高**推导

```
NODE_BLOCK_H  = MEMBER_BASE_RADIUS * 2 + LABEL_TOP_GAP + LABEL_MAX_LINES * LABEL_LINE_H
              = 11 + 5 + 24 = 40        // 圆顶 → 标签底（行距基准）
memberRowGap  = max(16, NODE_BLOCK_H + 4)   // = 44
```

- **为什么不是"按半径"**：本画布 `appearanceOf` 传 `memberRadius: MEMBER_BASE_RADIUS`（5.5）覆盖，成员半径**恒定**。若按半径推导会得出 `max(16, 15) = 16` —— 和改动前一样，缺陷原封不动保留。真正的占高来自**挂在圆下方的两行标题**（24px）。
- **为什么不把半径压小**：半径 5.5 已经是刻意的固定小尺寸；且横向重叠（标题框宽度）另有「宁缺勿叠」跳过机制兜底，压半径不解决纵向压盖。
- **保留 `max(16, …)` 下限**：防御性兜底，避免未来常量调整后行距反而小于改动前。

- **落点**：`front/app/features/tags/components/console/LaneCorrectionCanvas.vue` 常量 `NODE_BLOCK_H` 与 `memberRowGap`；验收落到 spec Scenario「同一天两个成员不重叠（含标签）」。
- **取舍**：被否方案——按半径推导（`2 × defaultMemberRadius + 4`）。否决理由：本画布半径被覆盖为常量 5.5，按半径得 16px，缺陷原样保留；且它随列宽变化会让 30 天档更密。

### D2 半径公式抽成纯函数，避免两处漂移

`pointAppearance` 内部算 `memberBase` 的公式抽成 `defaultMemberRadius(columnWidth: number): number` 到 `laneComposeLogic.ts`：

```
cap        = max(3, min(16, columnWidth × 0.46))
memberBase = max(min(cap, 10), cap × 0.85)
```

`pointAppearance` 改为调用它（`flags.memberRadius` 覆盖优先，行为逐一等价）。

**但画布行距刻意不依赖它**：本画布走的是 `memberRadius` 覆盖路径（5.5），与 `defaultMemberRadius(列宽)` 无关；让行距依赖它会算错。抽函数的价值仅在于消除 `pointAppearance` 内部重复，不是为了让行距使用。

- **落点**：`front/app/features/tags/composables/laneComposeLogic.ts:defaultMemberRadius`；`pointAppearance` 默认路径改为调用它。
- **取舍**：被否方案——让画布行距也调用本函数（看似「单一公式源」）。否决理由：两条路径的参数语义不同（画布走覆盖、默认路径走列宽），共用只会把错误引入画布。

### D3 成员区底部驱动分区位置，`canvasHeight` 自适应

现状 `zones.top` 硬编码 `156`，成员从 `MEMBER_Y = 86` 起按行距堆叠：

- 改动前：行距 16 → 成员区可用约 `186 − 86 = 100px` → 放得下 6 个 slot
- 改动后：行距 44 → 只放得下 2 个，**必须让分区跟着下移**

实现：

```
membersBottom = MEMBER_Y + max(memberSlotsPerDay − 1, 0) × memberRowGap
zoneTop       = max(156, membersBottom + 48)
h             = max(60, (bottom − zoneTop − 92) / 2)
canvasHeight  = max(360, containerHeight, membersBottom + 2 × 最小分区高 + 上下留白)
```

`maxSlotsPerDay` 取当天成员数的最大值（与 `points` 里的 slot 计数同源）。

- **落点**：`LaneCorrectionCanvas.vue` 的 `maxMemberSlotsPerDay` / `membersBottom` / `canvasHeight` / `zones`；验收落到 Scenario「成员很多时整体滚动而非压缩」。
- **取舍**：被否方案——保持 `zones.top` 硬编码 156、让成员溢出到候选区。否决理由：那会把「重叠」换成「压到别的区」，且候选区本身也会被挤。

### D4 缩放语义不变，只修布局比值

不改 `:width="vbWidth * zoom"` 的等比拉伸，不改 `scaledPointRadius` 的"缩小时补下限"行为。spec 里显式写清「缩放不承担解拥挤职责」，避免后续再有人试图用放大来掩盖重叠。

- **落点**：无代码落点；约束落在 `specs/lane-correction-surface` 的 Requirement「画布缩放不承担解拥挤职责」与 Scenario「放大不改变间距/占高比值」（对应测试 4.4）。
- **取舍**：被否方案——让 zoom 顺带放大行距以「拉开」拥挤。否决理由：等比拉伸下行距与占高同比变化，比值不变，放大无法解拥挤；真要改密度得换一套语义，超出本 change。

### D5 副作用取舍：纵向变高 → 滚动

成员多的日期会让画布整体变高（容器已 `overflow: auto`）。取舍：**宁可滚动，不可重叠**。已在 `ui-design.md` Layout Contract 记录，`specs` 也写了「SHALL NOT 压缩行距来强行容纳」。

- **落点**：`canvasHeight` 的 `max(baseCanvasHeight, membersBottom + 330)` 下限；容器 `.lcc-surface { overflow: auto }`。
- **取舍**：被否方案——压缩行距以维持画布高度不变。否决理由：压缩即回到缺陷本身。

## Risks

| 风险 | 缓解 |
| --- | --- |
| 行距变大后同屏可见成员变少 | 可滚动查看；`max(16, …)` 下限保证不比改动前更密 |
| `defaultMemberRadius` 抽函数后与 `pointAppearance` 行为不一致 | 抽函数时 `pointAppearance` **改为调用它**，并保留既有 `pointAppearance` 全部测试（成员尺寸不编码接近度、候选恒小于成员）作为回归锚 |
| `zones` 位置变化影响只读参照区与折叠带 | 折叠带用 `zones.zone1Line` 算高度，改动后自动跟随；新增用例断言"候选区不压成员" |
| 366 天档列宽极小、半径极小 | `max(16, …)` 下限兜底，行为与改动前一致 |

## Migration Plan

无数据迁移、无接口变更、无配置项。纯前端布局计算，部署即生效；旧前端不读任何新字段，可自由回退。
