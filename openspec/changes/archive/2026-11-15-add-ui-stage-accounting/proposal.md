<!-- complexity: medium -->
<!-- ui-impact: none -->

## Why

`add-ui-contract-ladder` 在 2026-11-15 归档，把 L0–L3 制品阶梯、R0–R3 评审阶梯、四条机械判据写成了规范文本。但它**一行代码也没有**——所以这套规矩目前没有任何执行点，只能靠 `change-review` 子线程人工核。

更关键的是：即使现在想加门禁，**照抄现有 `ui-approval` 机制也会失效**。

`ui-approval` 的检查点写在 `.pi/extensions/ui-design-gate.ts` 的第 132 行：

```ts
if (!modeInfo || modeInfo.mode !== "implementation" || !modeInfo.boundChange) return;
```

实测（事实库 30 天窗口）：34 次 `ui-design` 档派发 **100% 落在 `requirements` 或无档位**，`ui-dispatch-allowed` 留痕 0 条。原型生产不破坏产品代码，因此它天然不经过 `implementation` 入口——门禁根本没看到过它。

所以本次要解决的不是「判据写没写清楚」，而是**触发条件选错了维度**：`ui-approval` 拦的是「会破坏代码的实现入口」，用 mode 当触发条件是合理的；而 L1–L3 拦的是「原型怎么被生产」，`ui-prototype/` 里的每一次写入都该被看见，与 mode 无关。

## What Changes

1. **新增 stage 标记**：`ui-design.md` 头部机器可读标记 `<!-- ui-stage: l0|skeleton|interaction|data|approved -->`，与既有三条 marker 同层同形。缺标记视为 legacy。
2. **触发条件从 mode 改为路径**：写入目标落在 `openspec/changes/<name>/` 下即触发检查，**change 身份直接由路径解析**，不依赖 `mode.set` 的 `boundChange` 绑定——这正是绕开现有盲区的地方。
3. **stage → 允许写的路径域**：`l0` 只许写规划制品；`skeleton` 只许写 `ui-prototype/skeleton/`；`interaction` 只许写 `ui-prototype/`；`data` 只许写 `fixture.json`；`approved` 全放行。
4. **本阶段只记账、不阻断**：越域与缺失一律记 `policy.decision` 的 `warn`，不中断会话。理由是「按路径触发」这个改造本身有误伤风险（存量 22 个 change 的 `ui-prototype/` 都会被扫到），必须先证明不漏拦、不误拦，再上锁。

## Non-Goals

- **不 block**。真正的阻断、路径域硬限制、跳过 stage 的判定，留到观察期之后（见 design D5 的分阶段口径）。
- 不改既有 `UI_REASON_CODES` 的九个阻断码白名单——本次新增的三个码全部落在 **warn 侧**（对齐 `UI_PASS_REASON_CODES` 的既有先例：非阻断码属 harness 事实库词汇表，不进 spec 阻断码契约）。
- 不迁移存量 change：缺 marker 的 22 个 change 只 warn 一次，不要求补。
- 不动产品代码（`.pi/extensions/` 之外零 diff）。

## Impact

- 受影响文件：`.pi/extensions/lib/ui-design-gate.ts`（纯函数）、`.pi/extensions/ui-design-gate.ts`（触发点）、`.pi/extensions/tests/ui-design-gate.smoke.cjs`（用例）。
- 行为变化：会话在 `requirements` 档写 `openspec/changes/<x>/ui-prototype/**` 时，事实库会新增 `policy=ui-design-gate`、`action=warn` 的记录。**不改任何既有 block/pass 裁决**。
- 产品运行时行为：零变化。

## 回检口径

两周后用 `bash scripts/harness/harness-retro.sh` 看 `ui-design-gate` 的 warn 分布：若 `ui-stage-missing` 占比接近 100%（说明没人写 marker，机制空转），应先补 marker 的模板默认值而不是加 block；若 `ui-stage-scope-out` 命中集中在少数 change，再决定是否对那几个 change 单独上锁。
