# 轮次 4 Review 处置结论（supervisor 复核，2026-09-17）

Review 报告见同目录 [review-report-round4.md](review-report-round4.md)。verdict: BLOCK → **经主线程实测复现后推翻，维持合并（已归档）**。

## P1-1（parentSessionId 可选链必崩）→ 假阳性，实测推翻

reviewer 静态推演 `ctx?.sessionManager?.getHeader?.()?.parentSession` 在 `getHeader` 存在但返回 `null` 时抛 `TypeError`（可选链 `?.()` 只护 callee 不护调用返回值）。**推演方向错了**，两条独立实测：

1. **语义微测**（node 26，本机）：
   `node -e "const o={getHeader:()=>null}; console.log(o?.getHeader?.()?.parentSession)"` → `undefined`，**不抛**。V8 对 `?.()` 之后的成员访问仍受同一条可选链短路保护（整条链一旦短路即整体返回 undefined）。
2. **smoke 实跑**：19.11/19.12 的桩正是 `getHeader: () => null`（smoke.cjs:884/:904），`run-smoke.sh` 全绿（SMOKE OK, exit=0），supervisor 独立复跑一致。

reviewer 自述无 shell 工具、纯静态审查——「TS strict 拒绝该表达式（TS18047）」的断言同样未实跑（且 TS 的可选链模型与运行时语义一致，也不会报）。

## P1-2（apply-report 结果与源码不可调和）→ 随 P1-1 解体

源码从未处于「必崩」状态；apply-report-round4.md 的绿结果真实（supervisor 复现）。

## P2-1（ExtCtx 类型缺 getSessionFile 声明）→ 采纳，fixup 补上

`sessionManager` 类型块补 `getSessionFile?: () => string | undefined | null;`（纯类型声明，零行为变更；extensions 无 typecheck 门禁，属契约诚实性修正）。

## P2-2（getHeader/getSessionFile 回调本身抛错无防护）→ 记录不处理（reviewer 同结论）

理论性；round 1-2 已存在同一面，非本轮恶化。

## 非空性判别力（reviewer「必须复跑」项，supervisor 实跑闭环）

- 探针：`inheritFromParentHistory` 首行短路（`return false`）→ **恰 4 红**（19.10×3 + 19.11×1），19.12（负向用例）保持绿——与 apply-report §3 自述完全一致。
- 恢复后 sha256 与修复前一致；`run-smoke.sh` / `run-harness-smoke.sh` 双绿。

## 最终状态

轮次 4 设计与实现全部成立（回退语义/短路顺序/记账归属/失败方向安全/测试判别力），一行类型补声明已 fixup，无遗留阻塞。
