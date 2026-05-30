Status: ready-for-agent

## Parent

fix-tags-page-fractures

## What to build

修复 SectorApprovalPanel 执行反馈断裂。当前 panel 声明了 `execResult`/`execProgress`/`execError` refs 但从未赋值——confirm 事件发送到 TagsPage 执行 API 调用，结果不回传。

修复策略：将 API 调用移入 SectorApprovalPanel 内部。Panel 新增 `category` prop，confirm 时自行调用 `confirmRegenerateSectors` API 并更新进度/结果状态。TagsPage 简化为只监听 `done` 事件重新加载板块列表。

## Acceptance criteria

- [ ] SectorApprovalPanel 新增 `category` prop
- [ ] Panel 内部直接调用 `confirmRegenerateSectors` API
- [ ] `execResult`/`execProgress`/`execError` 在执行过程中正确更新
- [ ] 用户点击"全部批准"后看到进度反馈和执行结果
- [ ] TagsPage `handleConfirmRegenerate` 简化为 `handleConfirmDone`
- [ ] `pnpm lint` + `pnpm exec nuxi typecheck` 通过

## Blocked by

- `04-sector-confirm-api-structure` — API 数据结构必须先修好，否则确认仍无效

## Reference

- Plan: `docs/plans/2026-05-17-fix-phase14-16-bugs.md` Task 2
- `SectorApprovalPanel.vue:1-32, 128-138`, `TagsPage.vue:138-158`
